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

func TestCursorNativeConfig_PreservesMode0600(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, cursor.ConfigRelPath)
	writeJSONMode(t, path, map[string]any{
		"mcpServers": map[string]any{
			"personal": map[string]any{"command": "echo"},
		},
	}, 0o600)
	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true
	_, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "mode-cur",
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()},
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%04o want 0600", info.Mode().Perm())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"personal"`) || !strings.Contains(string(raw), `"filesystem"`) {
		t.Fatalf("merge incomplete: %s", raw)
	}
}

func TestOpenCodeNativeConfig_PreservesMode0600(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	path := filepath.Join(root, opencode.ConfigRelPath)
	writeJSONMode(t, path, map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"mcp": map[string]any{
			"keep": map[string]any{"type": "local", "command": []any{"echo"}},
		},
	}, 0o600)
	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true
	_, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "mode-oc",
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs}},
		Adapters:   []mcp.AdapterID{mcp.AdapterOpenCode},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterOpenCode: opencode.New()},
	})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%04o want 0600", info.Mode().Perm())
	}
}

func TestNativeConfig_RollbackRestoresBytesAndMode(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	cursorPath := filepath.Join(root, cursor.ConfigRelPath)
	writeJSONMode(t, cursorPath, map[string]any{
		"mcpServers": map[string]any{
			"personal": map[string]any{"command": "echo"},
		},
	}, 0o600)
	writeJSONMode(t, filepath.Join(root, opencode.ConfigRelPath), map[string]any{
		"$schema": "https://opencode.ai/config.json",
		"mcp":     map[string]any{},
	}, 0o644)

	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true
	// Establish Cursor ownership first.
	res1, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "rb-mode",
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()},
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(cursorPath)
	if err != nil {
		t.Fatal(err)
	}

	gh := mustBuiltin(t, mcp.BuiltinGitHub)
	gh.Enabled = true
	_, err = mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "rb-mode",
		Desired:  mcp.DesiredState{Definitions: []mcp.Definition{fs, gh}},
		Adapters: []mcp.AdapterID{mcp.AdapterCursor, mcp.AdapterOpenCode},
		Projectors: map[mcp.AdapterID]mcp.Projector{
			mcp.AdapterCursor:   cursor.New(),
			mcp.AdapterOpenCode: failApplyOpenCode{},
		},
		Ownership: res1.Ownership,
		Now:       func() time.Time { return time.Date(2026, 10, 9, 1, 0, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback, got %v", err)
	}
	after, err := os.ReadFile(cursorPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("cursor bytes changed\nbefore=%s\nafter=%s", before, after)
	}
	infoAfter, err := os.Stat(cursorPath)
	if err != nil {
		t.Fatal(err)
	}
	if infoAfter.Mode().Perm() != infoBefore.Mode().Perm() {
		t.Fatalf("mode before=%04o after=%04o", infoBefore.Mode().Perm(), infoAfter.Mode().Perm())
	}
}

func TestOwnership_AbsentBaselineRemovedOnRollback(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true
	ownPath := mcp.OwnershipPath(home, "first-own")
	if _, err := os.Lstat(ownPath); !os.IsNotExist(err) {
		t.Fatalf("ownership must start absent: %v", err)
	}
	_, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "first-own",
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: failApplyCursor{}},
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback, got %v", err)
	}
	if _, err := os.Lstat(ownPath); !os.IsNotExist(err) {
		t.Fatalf("ownership.yaml must not exist after failed first apply: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, cursor.ConfigRelPath)); !os.IsNotExist(err) {
		t.Fatalf("native config must not remain after failed first apply: %v", err)
	}
}

func TestOwnership_Existing0600RestoredOnRollback(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true
	res1, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "own-mode",
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()},
	})
	if err != nil {
		t.Fatal(err)
	}
	ownPath := mcp.OwnershipPath(home, "own-mode")
	if err := os.Chmod(ownPath, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(ownPath)
	if err != nil {
		t.Fatal(err)
	}

	gh := mustBuiltin(t, mcp.BuiltinGitHub)
	gh.Enabled = true
	_, err = mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "own-mode",
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs, gh}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: failApplyAfterMutationV{Projector: cursor.New()}},
		Ownership:  res1.Ownership,
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback, got %v", err)
	}
	after, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("ownership bytes changed\nbefore=%s\nafter=%s", before, after)
	}
	infoAfter, err := os.Stat(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if infoAfter.Mode().Perm() != infoBefore.Mode().Perm() {
		t.Fatalf("ownership mode before=%04o after=%04o", infoBefore.Mode().Perm(), infoAfter.Mode().Perm())
	}
}

func TestOpenCodeJSONC_BlocksWithoutMutation(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	jsonc := []byte("{\n  // comment\n  \"mcp\": {}\n}\n")
	if err := os.WriteFile(filepath.Join(root, mcp.OpenCodeJSONCRel), jsonc, 0o644); err != nil {
		t.Fatal(err)
	}
	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true
	_, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: "jsonc",
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs}},
		Adapters:   []mcp.AdapterID{mcp.AdapterOpenCode},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterOpenCode: opencode.New()},
	})
	if err == nil || !strings.Contains(err.Error(), "opencode.jsonc") {
		t.Fatalf("expected JSONC block, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, opencode.ConfigRelPath)); !os.IsNotExist(err) {
		t.Fatal("opencode.json must not be created")
	}
	got, err := os.ReadFile(filepath.Join(root, mcp.OpenCodeJSONCRel))
	if err != nil || string(got) != string(jsonc) {
		t.Fatalf("jsonc mutated: %q err=%v", got, err)
	}
	if _, err := os.Lstat(mcp.OwnershipPath(home, "jsonc")); !os.IsNotExist(err) {
		t.Fatal("ownership must remain absent")
	}
}

type failApplyOpenCode struct{ opencode.Projector }

func (failApplyOpenCode) ApplyMCPProjection(string, []mcp.NativeEntry, []string, []string) error {
	return fmt.Errorf("induced opencode apply failure")
}

type failApplyCursor struct{ cursor.Projector }

func (failApplyCursor) ApplyMCPProjection(string, []mcp.NativeEntry, []string, []string) error {
	return fmt.Errorf("induced cursor apply failure")
}

type failApplyAfterMutationV struct{ cursor.Projector }

func (p failApplyAfterMutationV) ApplyMCPProjection(root string, entries []mcp.NativeEntry, managed, remove []string) error {
	if err := p.Projector.ApplyMCPProjection(root, entries, managed, remove); err != nil {
		return err
	}
	return fmt.Errorf("induced failure after cursor mutation")
}

func writeJSONMode(t *testing.T, path string, doc map[string]any, mode os.FileMode) {
	t.Helper()
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}
