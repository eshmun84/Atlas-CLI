package mcp_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

func TestReconcile_AttemptBackupRemovedAfterSuccessfulRollback(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	pid := "bak-clean"
	oldStamp := "20260101T000000.000000000Z-000001"
	oldDir := filepath.Join(homePath, "projects", pid, "mcp", "backups", oldStamp)
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "cursor.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}

	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true
	gh := mustBuiltin(t, mcp.BuiltinGitHub)
	gh.Enabled = true

	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, cursor.ConfigRelPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(root, cursor.ConfigRelPath), map[string]any{
		"mcpServers": map[string]any{"personal-db": map[string]any{"command": "node"}},
	})
	writeJSON(t, filepath.Join(root, opencode.ConfigRelPath), map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"mcp":     map[string]any{"keep-me": map[string]any{"type": "local", "command": []any{"echo"}}},
	})
	res1, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: pid,
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()},
		Now:        func() time.Time { return time.Date(2026, 10, 8, 22, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(homePath, "projects", pid, "mcp", "backups")
	beforeEntries, err := os.ReadDir(backups)
	if err != nil {
		t.Fatal(err)
	}
	before := map[string]struct{}{}
	for _, e := range beforeEntries {
		before[e.Name()] = struct{}{}
	}
	if _, ok := before[oldStamp]; !ok {
		t.Fatal("pre-existing backup missing before failed attempt")
	}

	failStampTime := time.Date(2026, 10, 8, 22, 1, 0, 0, time.UTC)
	var cursorMutatedSeen bool
	_, err = mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: pid,
		Desired:  mcp.DesiredState{Definitions: []mcp.Definition{fs, gh}},
		Adapters: []mcp.AdapterID{mcp.AdapterCursor, mcp.AdapterOpenCode},
		Projectors: map[mcp.AdapterID]mcp.Projector{
			mcp.AdapterCursor: cursor.New(),
			mcp.AdapterOpenCode: failOpenCodeAfterCursor{
				root: root, cursorMutatedSeen: &cursorMutatedSeen,
			},
		},
		Ownership: res1.Ownership,
		Now:       func() time.Time { return failStampTime },
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback, got %v", err)
	}
	if strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("rollback should succeed: %v", err)
	}

	afterEntries, err := os.ReadDir(backups)
	if err != nil {
		t.Fatal(err)
	}
	after := map[string]struct{}{}
	for _, e := range afterEntries {
		after[e.Name()] = struct{}{}
	}
	if len(after) != len(before) {
		t.Fatalf("failed-attempt backup should be removed; before=%v after=%v", keys(before), keys(after))
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			t.Fatalf("pre-existing backup %s was removed", name)
		}
	}
	// Failed attempt stamp must not linger (NewBackupStamp embeds the Now() value).
	for name := range after {
		if strings.HasPrefix(name, failStampTime.UTC().Format("20060102T150405")) {
			t.Fatalf("failed attempt backup retained: %s", name)
		}
	}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestReconcile_CleanupFailureAfterSuccessfulRestore(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	pid := "bak-clean-fail"
	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true
	gh := mustBuiltin(t, mcp.BuiltinGitHub)
	gh.Enabled = true

	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, cursor.ConfigRelPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(root, cursor.ConfigRelPath), map[string]any{
		"mcpServers": map[string]any{"personal-db": map[string]any{"command": "node"}},
	})
	writeJSON(t, filepath.Join(root, opencode.ConfigRelPath), map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"mcp":     map[string]any{"keep-me": map[string]any{"type": "local", "command": []any{"echo"}}},
	})
	res1, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: pid,
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()},
		Now:        func() time.Time { return time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}

	mcp.SetRemoveAttemptBackupForTest(func(homePath, rel string) error {
		return errors.New("injected cleanup failure")
	})
	t.Cleanup(func() { mcp.SetRemoveAttemptBackupForTest(nil) })

	var cursorMutatedSeen bool
	_, err = mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: pid,
		Desired:  mcp.DesiredState{Definitions: []mcp.Definition{fs, gh}},
		Adapters: []mcp.AdapterID{mcp.AdapterCursor, mcp.AdapterOpenCode},
		Projectors: map[mcp.AdapterID]mcp.Projector{
			mcp.AdapterCursor: cursor.New(),
			mcp.AdapterOpenCode: failOpenCodeAfterCursor{
				root: root, cursorMutatedSeen: &cursorMutatedSeen,
			},
		},
		Ownership: res1.Ownership,
		Now:       func() time.Time { return time.Date(2026, 10, 9, 1, 1, 0, 0, time.UTC) },
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "transactional backup cleanup failed") {
		t.Fatalf("expected cleanup failure wording, got %v", err)
	}
	if strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("must not claim rollback failed when baseline restored: %v", err)
	}
	if !strings.Contains(err.Error(), "restored") {
		t.Fatalf("expected restored baseline wording: %v", err)
	}
}

func TestReconcile_AttemptBackupRetainedWhenRollbackFails(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	pid := "bak-retain"
	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true

	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, cursor.ConfigRelPath)), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(root, cursor.ConfigRelPath), map[string]any{
		"mcpServers": map[string]any{},
	})
	res1, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: pid,
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()},
		Now:        func() time.Time { return time.Date(2026, 10, 8, 22, 10, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}

	gh := mustBuiltin(t, mcp.BuiltinGitHub)
	gh.Enabled = true
	_, err = mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: homePath, ProjectID: pid,
		Desired:  mcp.DesiredState{Definitions: []mcp.Definition{fs, gh}},
		Adapters: []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{
			mcp.AdapterCursor: poisonRestoreCursor{root: root},
		},
		Ownership: res1.Ownership,
		Now:       func() time.Time { return time.Date(2026, 10, 8, 22, 11, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("expected rollback failure, got %v", err)
	}
	backups := filepath.Join(homePath, "projects", pid, "mcp", "backups")
	entries, err := os.ReadDir(backups)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("attempt backup must be retained when rollback fails")
	}
}

// poisonRestoreCursor writes successfully then replaces the native file with a
// directory so RestoreSnapshots cannot restore bytes.
type poisonRestoreCursor struct {
	root string
	cursor.Projector
}

func (p poisonRestoreCursor) ApplyMCPProjection(root string, entries []mcp.NativeEntry, managed, remove []string) error {
	if err := p.Projector.ApplyMCPProjection(root, entries, managed, remove); err != nil {
		return err
	}
	path := filepath.Join(p.root, cursor.ConfigRelPath)
	if err := os.Remove(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return fmt.Errorf("induced failure after poison")
}

func TestRemoveContainedRel_SymlinkSafe(t *testing.T) {
	homePath := t.TempDir()
	outside := t.TempDir()
	pid := "sym"
	stamp := "20261008T000000.000000000Z-000001"
	rel := filepath.ToSlash(filepath.Join("projects", pid, "mcp", "backups", stamp))
	if err := os.MkdirAll(filepath.Join(homePath, "projects", pid, "mcp", "backups"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(homePath, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
	if err := home.RemoveContainedRel(homePath, rel); err == nil {
		t.Fatal("expected symlink refuse")
	}
	if _, err := os.Lstat(filepath.Join(homePath, filepath.FromSlash(rel))); err != nil {
		t.Fatal("symlink must remain")
	}
}
