package workspace_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestEvaluateRuntimeHealth_NotInitialized(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	files, err := workspace.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	atlas := workspace.EvaluateAtlasStatus(root, files)
	health := workspace.EvaluateRuntimeHealth(root, atlas, files)

	if health.Initialized || health.ConfigExists || health.StateExists || health.AgentsExists {
		t.Fatalf("unexpected health: %#v", health)
	}
	if health.BackupsDirExists {
		t.Fatal("backups should be absent")
	}
	for _, art := range health.ForbiddenArtifacts {
		if art.Present {
			t.Fatalf("forbidden present: %#v", art)
		}
	}
	assertUnchangedTree(t, root, snapshotTree(t, root))
}

func TestEvaluateRuntimeHealth_InitializedCursor(t *testing.T) {
	t.Parallel()

	root := materializeProject(t, []string{"cursor"}, true)
	before := snapshotTree(t, root)

	result, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	h := result.Runtime
	if !h.Initialized || !h.ConfigLoads || !h.StateLoads || !h.RuntimeMaterialized {
		t.Fatalf("health = %#v", h)
	}
	if !h.AgentsExists || !h.AgentsMarkers.Complete() {
		t.Fatalf("agents = exists=%v markers=%#v", h.AgentsExists, h.AgentsMarkers)
	}
	if len(h.ExpectedProjections) != 1 || !h.ExpectedProjections[0].Present {
		t.Fatalf("projections = %#v", h.ExpectedProjections)
	}
	if !h.ContextGraphReadable || !h.ContextGraphEnabled {
		t.Fatalf("context graph = readable=%v enabled=%v", h.ContextGraphReadable, h.ContextGraphEnabled)
	}
	if !h.BackupsDirExists {
		t.Fatal("backups missing")
	}
	assertUnchangedTree(t, root, before)
}

func TestEvaluateRuntimeHealth_InitializedOpenCode(t *testing.T) {
	t.Parallel()

	root := materializeProject(t, []string{"opencode"}, true)
	before := snapshotTree(t, root)
	h := workspace.EvaluateRuntimeHealth(root, workspace.EvaluateAtlasStatus(root, mustFiles(t, root)), mustFiles(t, root))
	if len(h.ExpectedProjections) != 1 || h.ExpectedProjections[0].Path != config.FileOpenCodeAtlas || !h.ExpectedProjections[0].Present {
		t.Fatalf("projections = %#v", h.ExpectedProjections)
	}
	assertUnchangedTree(t, root, before)
}

func TestEvaluateRuntimeHealth_InvalidConfigPartial(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".atlas", "config.yaml"), "project:\n  name: demo\n  mode: new\ngovernance:\n  workflow: sdd\n  spec_engine: none\nadapters:\n  selected: [Cursor]\nsource_control:\n  mode: none\n  default_remote: origin\n  branch_strategy: manual\n  governance_files: local_only\nmemory:\n  strategy: sqlite\ncontext:\n  graph:\n    enabled: true\nmcp:\n  builtins:\n    jira:\n      enabled: false\n    context7:\n      enabled: false\n    chrome_devtools:\n      enabled: false\n  custom: []\n")
	writeFile(t, filepath.Join(root, "AGENTS.md"), config.RenderAgentsMD("demo", true, nil))

	files := mustFiles(t, root)
	atlas := workspace.EvaluateAtlasStatus(root, files)
	h := workspace.EvaluateRuntimeHealth(root, atlas, files)
	if !h.ConfigExists || h.ConfigLoads || h.ConfigError == "" {
		t.Fatalf("expected partial invalid config health: %#v", h)
	}
	if !h.AgentsExists || !h.AgentsMarkers.Complete() {
		t.Fatalf("agents should still be inspected: %#v", h)
	}
}

func TestEvaluateRuntimeHealth_MissingAgentsAndProjection(t *testing.T) {
	t.Parallel()

	root := materializeProject(t, []string{"cursor"}, true)
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".cursor", "rules", "atlas.mdc")); err != nil {
		t.Fatal(err)
	}

	h := workspace.EvaluateRuntimeHealth(root, workspace.EvaluateAtlasStatus(root, mustFiles(t, root)), mustFiles(t, root))
	if h.AgentsExists {
		t.Fatal("agents should be missing")
	}
	if len(h.ExpectedProjections) != 1 || h.ExpectedProjections[0].Present {
		t.Fatalf("projection should be missing: %#v", h.ExpectedProjections)
	}
	joined := strings.Join(h.Warnings, "\n")
	if !strings.Contains(joined, "AGENTS.md is missing") {
		t.Fatalf("warnings = %s", joined)
	}
	if !strings.Contains(joined, ".cursor/rules/atlas.mdc") {
		t.Fatalf("warnings = %s", joined)
	}
}

func TestEvaluateRuntimeHealth_BrokenMarkers(t *testing.T) {
	t.Parallel()

	root := materializeProject(t, nil, true)
	writeFile(t, filepath.Join(root, "AGENTS.md"), "# broken\n<!-- ATLAS:MANAGED:BEGIN -->\n")
	h := workspace.EvaluateRuntimeHealth(root, workspace.EvaluateAtlasStatus(root, mustFiles(t, root)), mustFiles(t, root))
	if h.AgentsMarkers.Complete() {
		t.Fatalf("markers should be incomplete: %#v", h.AgentsMarkers)
	}
	if !strings.Contains(strings.Join(h.Warnings, "\n"), "markers are incomplete") {
		t.Fatalf("warnings = %#v", h.Warnings)
	}
}

func materializeProject(t *testing.T, adapters []string, contextGraph bool) string {
	t.Helper()
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "demo",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	})
	for _, adapter := range adapters {
		if !draft.ToggleMulti("adapters.selected", adapter) {
			t.Fatalf("toggle %s", adapter)
		}
	}
	if !contextGraph {
		if !draft.ToggleBool("context.graph.enabled") {
			t.Fatal("toggle context graph")
		}
	}
	fixed := time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed },
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return root
}

func mustFiles(t *testing.T, root string) workspace.FileInfo {
	t.Helper()
	files, err := workspace.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			out[filepath.ToSlash(rel)+"/"] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func assertUnchangedTree(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := snapshotTree(t, root)
	if len(before) != len(after) {
		t.Fatalf("tree size changed: before=%d after=%d", len(before), len(after))
	}
	for path, content := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf("path removed: %s", path)
		}
		if got != content {
			t.Fatalf("path mutated: %s", path)
		}
	}
}
