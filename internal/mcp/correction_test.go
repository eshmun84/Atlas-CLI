package mcp_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

func TestNativeKeyCollisionBlocked(t *testing.T) {
	t.Parallel()
	_, err := mcp.BuildDesiredState(mcp.Selection{
		BuiltinEnabled: map[string]bool{mcp.BuiltinGitHub: true},
		Custom: []mcp.CustomSpec{{
			ID: "custom-1", Name: "GitHub", Transport: "streamable_http",
			CommandOrURL: "https://example.com/mcp", Enabled: true,
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "native key collision") {
		t.Fatalf("expected collision, got %v", err)
	}

	_, err = mcp.BuildDesiredState(mcp.Selection{
		Custom: []mcp.CustomSpec{
			{ID: "custom-1", Name: "My Tools", Transport: "stdio", CommandOrURL: "npx", Enabled: true},
			{ID: "custom-2", Name: "my-tools", Transport: "stdio", CommandOrURL: "npx", Enabled: true},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "native key collision") {
		t.Fatalf("expected custom collision, got %v", err)
	}
}

func TestSymlinkParentEscapeBlocked(t *testing.T) {
	root := t.TempDir()
	external := t.TempDir()
	cursorDir := filepath.Join(root, ".cursor")
	if err := os.Symlink(external, cursorDir); err != nil {
		t.Fatal(err)
	}
	// mcp.json does not exist under the symlink target.
	_, err := mcp.ContainedJoin(root, cursor.ConfigRelPath)
	if err == nil {
		t.Fatal("expected symlink parent to block ContainedJoin")
	}

	home := t.TempDir()
	def := mustBuiltin(t, mcp.BuiltinFilesystem)
	def.Enabled = true
	_, err = mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "sym-proj",
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{def}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()},
	})
	if err == nil {
		t.Fatal("expected reconcile block on symlink escape")
	}
	// No write outside the project.
	entries, _ := os.ReadDir(external)
	if len(entries) != 0 {
		t.Fatalf("external dir polluted: %#v", entries)
	}
	if _, err := os.Stat(filepath.Join(external, "mcp.json")); !os.IsNotExist(err) {
		t.Fatal("mcp.json written outside project")
	}
}

func TestBackupsOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	def := mustBuiltin(t, mcp.BuiltinFilesystem)
	def.Enabled = true
	projectors := map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()}
	res1, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "bak-proj",
		Desired:  mcp.DesiredState{Definitions: []mcp.Definition{def}},
		Adapters: []mcp.AdapterID{mcp.AdapterCursor}, Projectors: projectors,
		Now: func() time.Time { return time.Date(2026, 10, 8, 18, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	// Drift Atlas-owned entry so the next apply updates and takes a Home backup.
	writeJSON(t, filepath.Join(root, cursor.ConfigRelPath), map[string]any{
		"mcpServers": map[string]any{
			"filesystem": map[string]any{"command": "drifted"},
		},
	})
	res, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "bak-proj",
		Desired:  mcp.DesiredState{Definitions: []mcp.Definition{def}},
		Adapters: []mcp.AdapterID{mcp.AdapterCursor}, Projectors: projectors,
		Ownership: res1.Ownership,
		Now:       func() time.Time { return time.Date(2026, 10, 8, 18, 1, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.BackupStamp == "" {
		t.Fatal("expected backup stamp")
	}
	homeBackup := filepath.Join(home, "projects", "bak-proj", "mcp", "backups", res.BackupStamp, "cursor.json")
	if _, err := os.Stat(homeBackup); err != nil {
		t.Fatalf("home backup missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "mcp.json.atlas-backup")); !os.IsNotExist(err) {
		t.Fatal("workspace atlas-backup must not exist")
	}
	found, rel, err := mcp.WorkspaceHasAtlasMCPBackup(root)
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatalf("workspace polluted with %s", rel)
	}
}

type failOpenCodeAfterCursor struct {
	opencode.Projector
	root              string
	cursorMutatedSeen *bool
}

func (p failOpenCodeAfterCursor) ApplyMCPProjection(root string, entries []mcp.NativeEntry, managedKeys, removeKeys []string) error {
	raw, err := os.ReadFile(filepath.Join(p.root, cursor.ConfigRelPath))
	if err != nil {
		return fmt.Errorf("induced opencode failure (cursor unreadable: %v)", err)
	}
	// Deterministic apply order (cursor < opencode): first adapter must already be mutated.
	if !strings.Contains(string(raw), `"github"`) {
		return fmt.Errorf("induced opencode failure (cursor was not mutated first)")
	}
	if p.cursorMutatedSeen != nil {
		*p.cursorMutatedSeen = true
	}
	return fmt.Errorf("induced opencode failure")
}

func TestMultiAdapterRollback(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true
	gh := mustBuiltin(t, mcp.BuiltinGitHub)
	gh.Enabled = true
	projectorsOK := map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()}

	// Seed user-owned entries and establish ownership via a successful Cursor-only apply.
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(root, cursor.ConfigRelPath), map[string]any{
		"mcpServers": map[string]any{
			"personal-db": map[string]any{"command": "node"},
		},
	})
	writeJSON(t, filepath.Join(root, opencode.ConfigRelPath), map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"mcp": map[string]any{
			"keep-me": map[string]any{"type": "local", "command": []any{"echo"}},
		},
	})
	res1, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "rb",
		Desired:  mcp.DesiredState{Definitions: []mcp.Definition{fs}},
		Adapters: []mcp.AdapterID{mcp.AdapterCursor}, Projectors: projectorsOK,
	})
	if err != nil {
		t.Fatal(err)
	}
	cursorBaseline, err := os.ReadFile(filepath.Join(root, cursor.ConfigRelPath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cursorBaseline), `"filesystem"`) || !strings.Contains(string(cursorBaseline), "personal-db") {
		t.Fatalf("baseline cursor unexpected: %s", cursorBaseline)
	}
	ocBaseline, err := os.ReadFile(filepath.Join(root, opencode.ConfigRelPath))
	if err != nil {
		t.Fatal(err)
	}
	ownBaseline, err := mcp.LoadOwnership(home, "rb")
	if err != nil {
		t.Fatal(err)
	}
	if len(ownBaseline.AdapterEntries(mcp.AdapterCursor)) == 0 {
		t.Fatal("expected cursor ownership after baseline apply")
	}
	if len(ownBaseline.AdapterEntries(mcp.AdapterOpenCode)) != 0 {
		t.Fatal("opencode ownership should be empty before multi-adapter apply")
	}

	var cursorMutatedSeen bool
	_, err = mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "rb",
		Desired:  mcp.DesiredState{Definitions: []mcp.Definition{fs, gh}},
		Adapters: []mcp.AdapterID{mcp.AdapterCursor, mcp.AdapterOpenCode},
		Projectors: map[mcp.AdapterID]mcp.Projector{
			mcp.AdapterCursor: cursor.New(),
			mcp.AdapterOpenCode: failOpenCodeAfterCursor{
				root: root, cursorMutatedSeen: &cursorMutatedSeen,
			},
		},
		Ownership: res1.Ownership,
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback error, got %v", err)
	}
	if !cursorMutatedSeen {
		t.Fatal("expected first adapter (cursor) to mutate successfully before second adapter failed")
	}

	cursorAfter, _ := os.ReadFile(filepath.Join(root, cursor.ConfigRelPath))
	if string(cursorAfter) != string(cursorBaseline) {
		t.Fatalf("cursor not restored to pre-failure baseline:\n%s", cursorAfter)
	}
	if strings.Contains(string(cursorAfter), `"github"`) {
		t.Fatal("github leaked on cursor after rollback")
	}
	if !strings.Contains(string(cursorAfter), "personal-db") {
		t.Fatal("user-owned cursor entry lost after rollback")
	}
	ocAfter, _ := os.ReadFile(filepath.Join(root, opencode.ConfigRelPath))
	if string(ocAfter) != string(ocBaseline) {
		t.Fatalf("opencode not restored:\n%s", ocAfter)
	}
	if strings.Contains(string(ocAfter), `"filesystem"`) || strings.Contains(string(ocAfter), `"github"`) {
		t.Fatal("atlas entries leaked on opencode after rollback")
	}
	ownAfter, err := mcp.LoadOwnership(home, "rb")
	if err != nil {
		t.Fatal(err)
	}
	if !ownershipDocsEqual(ownBaseline, ownAfter) {
		t.Fatalf("ownership not restored:\nbefore=%#v\nafter=%#v", ownBaseline, ownAfter)
	}
}

func ownershipDocsEqual(a, b mcp.OwnershipDocument) bool {
	if a.SchemaVersion != b.SchemaVersion || a.ProjectID != b.ProjectID {
		return false
	}
	if len(a.Adapters) != len(b.Adapters) {
		return false
	}
	for _, aa := range a.Adapters {
		be := b.AdapterEntries(aa.Adapter)
		ae := a.AdapterEntries(aa.Adapter)
		if len(ae) != len(be) {
			return false
		}
		am := map[string]mcp.OwnedEntry{}
		for _, e := range ae {
			am[e.NativeKey] = e
		}
		for _, e := range be {
			prev, ok := am[e.NativeKey]
			if !ok || prev.DefinitionID != e.DefinitionID {
				return false
			}
		}
	}
	return true
}

func TestOpenCodeFlatAndNestedPreservation(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	def := mustBuiltin(t, mcp.BuiltinGitHub)
	def.Enabled = true

	t.Run("flat", func(t *testing.T) {
		root := t.TempDir()
		writeJSON(t, filepath.Join(root, opencode.ConfigRelPath), map[string]any{
			"$schema": "https://opencode.ai/config.json",
			"mcp": map[string]any{
				"keep-flat": map[string]any{"type": "local", "command": []any{"echo"}},
			},
		})
		_, err := mcp.Reconcile(mcp.ReconcileInput{
			Root: root, HomePath: home, ProjectID: "flat",
			Desired:    mcp.DesiredState{Definitions: []mcp.Definition{def}},
			Adapters:   []mcp.AdapterID{mcp.AdapterOpenCode},
			Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterOpenCode: opencode.New()},
		})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(filepath.Join(root, opencode.ConfigRelPath))
		var doc map[string]any
		_ = json.Unmarshal(raw, &doc)
		mcpObj := doc["mcp"].(map[string]any)
		if _, hasServers := mcpObj["servers"]; hasServers {
			t.Fatalf("flat became nested: %s", raw)
		}
		if _, ok := mcpObj["keep-flat"]; !ok {
			t.Fatal("user entry lost")
		}
		if _, ok := mcpObj["github"]; !ok {
			t.Fatal("github missing")
		}
	})

	t.Run("nested", func(t *testing.T) {
		root := t.TempDir()
		writeJSON(t, filepath.Join(root, opencode.ConfigRelPath), map[string]any{
			"$schema": "https://opencode.ai/config.json",
			"mcp": map[string]any{
				"servers": map[string]any{
					"keep-nested": map[string]any{"type": "local", "command": []any{"echo"}},
				},
			},
		})
		_, err := mcp.Reconcile(mcp.ReconcileInput{
			Root: root, HomePath: home, ProjectID: "nested",
			Desired:    mcp.DesiredState{Definitions: []mcp.Definition{def}},
			Adapters:   []mcp.AdapterID{mcp.AdapterOpenCode},
			Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterOpenCode: opencode.New()},
		})
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := os.ReadFile(filepath.Join(root, opencode.ConfigRelPath))
		var doc map[string]any
		_ = json.Unmarshal(raw, &doc)
		mcpObj := doc["mcp"].(map[string]any)
		servers, ok := mcpObj["servers"].(map[string]any)
		if !ok {
			t.Fatalf("nested not preserved: %s", raw)
		}
		if len(mcpObj) != 1 {
			t.Fatalf("hybrid/mixed mcp object: %s", raw)
		}
		if _, ok := servers["keep-nested"]; !ok {
			t.Fatal("user nested entry lost")
		}
		if _, ok := servers["github"]; !ok {
			t.Fatal("github missing in nested")
		}
	})

	t.Run("ambiguous", func(t *testing.T) {
		root := t.TempDir()
		writeJSON(t, filepath.Join(root, opencode.ConfigRelPath), map[string]any{
			"mcp": map[string]any{
				"servers":   map[string]any{"a": map[string]any{"type": "remote", "url": "https://a.example/mcp"}},
				"flat-also": map[string]any{"type": "remote", "url": "https://b.example/mcp"},
			},
		})
		_, err := mcp.Reconcile(mcp.ReconcileInput{
			Root: root, HomePath: home, ProjectID: "amb",
			Desired:    mcp.DesiredState{Definitions: []mcp.Definition{def}},
			Adapters:   []mcp.AdapterID{mcp.AdapterOpenCode},
			Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterOpenCode: opencode.New()},
		})
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("expected ambiguous block, got %v", err)
		}
		raw, _ := os.ReadFile(filepath.Join(root, opencode.ConfigRelPath))
		if !strings.Contains(string(raw), "flat-also") {
			t.Fatal("ambiguous file was mutated")
		}
	})
}

func TestRepeatedApplyIdempotentNoChurn(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	def := mustBuiltin(t, mcp.BuiltinFilesystem)
	def.Enabled = true
	fixed := time.Date(2026, 10, 8, 19, 0, 0, 0, time.UTC)
	projectors := map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()}
	res1, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "idem",
		Desired:  mcp.DesiredState{Definitions: []mcp.Definition{def}},
		Adapters: []mcp.AdapterID{mcp.AdapterCursor}, Projectors: projectors,
		Now: func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, cursor.ConfigRelPath)
	before, _ := os.ReadFile(path)
	info1, _ := os.Stat(path)
	backupDir := filepath.Join(home, "projects", "idem", "mcp", "backups")
	countBackups := func() int {
		n := 0
		_ = filepath.Walk(backupDir, func(_ string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.HasSuffix(info.Name(), ".json") {
				n++
			}
			return nil
		})
		return n
	}
	backups1 := countBackups()

	res2, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "idem",
		Desired:  mcp.DesiredState{Definitions: []mcp.Definition{def}},
		Adapters: []mcp.AdapterID{mcp.AdapterCursor}, Projectors: projectors,
		Ownership: res1.Ownership,
		Now:       func() time.Time { return fixed.Add(time.Minute) },
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)
	info2, _ := os.Stat(path)
	if string(before) != string(after) {
		t.Fatal("second apply mutated native config")
	}
	if !info2.ModTime().Equal(info1.ModTime()) {
		t.Fatal("second apply changed mtime")
	}
	if res2.Applied {
		t.Fatal("second apply should be no-op")
	}
	if countBackups() != backups1 {
		t.Fatal("second apply created extra backups")
	}
	found, rel, _ := mcp.WorkspaceHasAtlasMCPBackup(root)
	if found {
		t.Fatalf("workspace backup pollution: %s", rel)
	}
}
