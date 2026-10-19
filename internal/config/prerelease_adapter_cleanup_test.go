package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func assertAssetsLockSynced(t *testing.T, root string) {
	t.Helper()
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	homePath, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	want, err := config.RenderAssetsLockYAMLFor(homePath, doc)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, config.FileAssetsLock))
	if err != nil {
		t.Fatalf("assets.lock missing: %v", err)
	}
	if string(got) != want {
		t.Fatalf("assets.lock drifted after configure\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func seedOpenSpecProviderMarkers(t *testing.T, root, adapter string) {
	t.Helper()
	var commandsDir, skillsDir string
	switch adapter {
	case "cursor":
		commandsDir = filepath.Join(root, ".cursor", "commands")
		skillsDir = filepath.Join(root, ".cursor", "skills")
	case "opencode":
		commandsDir = filepath.Join(root, ".opencode", "commands")
		skillsDir = filepath.Join(root, ".opencode", "skills")
	default:
		t.Fatalf("unknown adapter %s", adapter)
	}
	if err := os.MkdirAll(commandsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"opsx-propose.md", "opsx-apply.md", "opsx-archive.md"} {
		if err := os.WriteFile(filepath.Join(commandsDir, name), []byte("provider\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, skill := range []string{"openspec-propose", "openspec-apply-change"} {
		dir := filepath.Join(skillsDir, skill)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("provider\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func assertNoProviderMarkers(t *testing.T, root, adapter string) {
	t.Helper()
	if missing := openspec.MissingToolProjections(root, []string{adapter}); len(missing) != 1 || missing[0] != adapter {
		t.Fatalf("expected %s provider markers absent, MissingToolProjections=%v", adapter, missing)
	}
	files, err := openspec.ListStaleToolProjectionFiles(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	prefix := "." + adapter
	if adapter == "opencode" {
		prefix = ".opencode"
	}
	if adapter == "cursor" {
		prefix = ".cursor"
	}
	for _, f := range files {
		if strings.HasPrefix(f, prefix+"/") {
			t.Fatalf("stale provider file remains for %s: %s", adapter, f)
		}
	}
}

func assertAdapterRootGone(t *testing.T, root, adapter string) {
	t.Helper()
	var rel string
	switch adapter {
	case "cursor":
		rel = ".cursor"
	case "opencode":
		rel = ".opencode"
	}
	if _, err := os.Lstat(filepath.Join(root, rel)); !os.IsNotExist(err) {
		t.Fatalf("%s should be fully removed when exclusively managed; err=%v", rel, err)
	}
}

func TestAdapterTransition_T1_CursorToOpenCode(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "t1")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	seedOpenSpecProviderMarkers(t, root, "cursor")
	assertCursorAtlasSurfaces(t, root, true)
	assertAssetsLockSynced(t, root)

	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	assertOpenCodeAtlasSurfaces(t, root, true)
	assertCursorAtlasSurfaces(t, root, false)
	assertNoProviderMarkers(t, root, "cursor")
	assertAdapterRootGone(t, root, "cursor")
	assertAssetsLockSynced(t, root)
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := config.JoinChips(doc.Adapters.Selected); got != "opencode" {
		t.Fatalf("adapters=%q", got)
	}
}

func TestAdapterTransition_T2_OpenCodeToCursor(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "t2")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	seedOpenSpecProviderMarkers(t, root, "opencode")

	_ = draft.ToggleMulti("adapters.selected", "opencode")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	assertCursorAtlasSurfaces(t, root, true)
	assertOpenCodeAtlasSurfaces(t, root, false)
	assertNoProviderMarkers(t, root, "opencode")
	assertAssetsLockSynced(t, root)
}

func TestAdapterTransition_T3_CursorToBoth(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "t3")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	seedOpenSpecProviderMarkers(t, root, "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	assertCursorAtlasSurfaces(t, root, true)
	assertOpenCodeAtlasSurfaces(t, root, true)
	if _, err := os.Stat(filepath.Join(root, ".cursor", "commands", "opsx-propose.md")); err != nil {
		t.Fatal("cursor provider markers must remain when cursor still selected")
	}
	assertAssetsLockSynced(t, root)
}

func TestAdapterTransition_T4_BothToOpenCode(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "t4")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	seedOpenSpecProviderMarkers(t, root, "cursor")
	seedOpenSpecProviderMarkers(t, root, "opencode")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	assertOpenCodeAtlasSurfaces(t, root, true)
	assertCursorAtlasSurfaces(t, root, false)
	assertNoProviderMarkers(t, root, "cursor")
	assertAdapterRootGone(t, root, "cursor")
	if _, err := os.Stat(filepath.Join(root, ".opencode", "commands", "opsx-propose.md")); err != nil {
		t.Fatal("opencode provider markers must remain")
	}
	assertAssetsLockSynced(t, root)
}

func TestAdapterTransition_T5_DeselectPreservesUserOwned(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "t5")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	seedOpenSpecProviderMarkers(t, root, "cursor")
	userRule := filepath.Join(root, ".cursor", "rules", "mine.mdc")
	if err := os.WriteFile(userRule, []byte("user owned\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	userCmd := filepath.Join(root, ".cursor", "commands", "my-cmd.md")
	if err := os.WriteFile(userCmd, []byte("user cmd\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(userRule)
	if err != nil || string(got) != "user owned\n" {
		t.Fatalf("user rule lost: %v %q", err, got)
	}
	got, err = os.ReadFile(userCmd)
	if err != nil || string(got) != "user cmd\n" {
		t.Fatalf("user command lost: %v %q", err, got)
	}
	assertNoProviderMarkers(t, root, "cursor")
	if _, err := os.Lstat(filepath.Join(root, ".cursor")); err != nil {
		t.Fatal(".cursor must remain when user-owned content present")
	}
	assertAssetsLockSynced(t, root)
}

func TestAdapterTransition_T6_DeselectFullyManagedRemovesRoot(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "t6")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	seedOpenSpecProviderMarkers(t, root, "cursor")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	assertAdapterRootGone(t, root, "cursor")
	assertOpenCodeAtlasSurfaces(t, root, true)
	assertAssetsLockSynced(t, root)
}

func TestAdapterTransition_T7_TransactionalFailureRollsBack(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "t7")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	seedOpenSpecProviderMarkers(t, root, "cursor")
	beforeLock, _ := os.ReadFile(filepath.Join(root, config.FileAssetsLock))
	beforeMCP, _ := os.ReadFile(filepath.Join(root, ".cursor", "mcp.json"))
	beforeProvider, _ := os.ReadFile(filepath.Join(root, ".cursor", "commands", "opsx-propose.md"))

	config.SetAfterSkillsReconcileHookForTest(func() error {
		return os.ErrInvalid
	})
	t.Cleanup(func() { config.SetAfterSkillsReconcileHookForTest(nil) })

	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	_, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback error, got %v", err)
	}
	afterLock, _ := os.ReadFile(filepath.Join(root, config.FileAssetsLock))
	if string(afterLock) != string(beforeLock) {
		t.Fatal("assets.lock not restored after rollback")
	}
	afterMCP, _ := os.ReadFile(filepath.Join(root, ".cursor", "mcp.json"))
	if string(afterMCP) != string(beforeMCP) {
		t.Fatal("cursor mcp.json not restored")
	}
	afterProvider, _ := os.ReadFile(filepath.Join(root, ".cursor", "commands", "opsx-propose.md"))
	if string(afterProvider) != string(beforeProvider) {
		t.Fatal("provider projection not restored")
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "rules", "atlas.mdc")); err != nil {
		t.Fatal("cursor atlas rule must be restored")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(opencode.ConfigRelPath))); !os.IsNotExist(err) {
		t.Fatal("partial opencode MCP must not remain after rollback")
	}
}

func TestAdapterTransition_T8_AssetsLockFailureRollsBack(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := configureDraftBoth(t, "t8")
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := fullMCPDraft(t)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	beforeCfg, _ := os.ReadFile(filepath.Join(root, config.FileConfig))
	beforeLock, _ := os.ReadFile(filepath.Join(root, config.FileAssetsLock))
	beforeRule, _ := os.ReadFile(filepath.Join(root, ".cursor", "rules", "atlas.mdc"))

	config.SetRenderRuntimeManifestYAMLForTest(func(string, []string) (string, error) {
		return "", os.ErrInvalid
	})
	t.Cleanup(func() { config.SetRenderRuntimeManifestYAMLForTest(nil) })

	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	_, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected assets lock / runtime rollback, got %v", err)
	}
	afterCfg, _ := os.ReadFile(filepath.Join(root, config.FileConfig))
	if string(afterCfg) != string(beforeCfg) {
		t.Fatal("config.yaml not restored")
	}
	afterLock, _ := os.ReadFile(filepath.Join(root, config.FileAssetsLock))
	if string(afterLock) != string(beforeLock) {
		t.Fatal("assets.lock not restored")
	}
	afterRule, _ := os.ReadFile(filepath.Join(root, ".cursor", "rules", "atlas.mdc"))
	if string(afterRule) != string(beforeRule) {
		t.Fatal("cursor runtime not restored")
	}
}
