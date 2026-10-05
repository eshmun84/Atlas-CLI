package workspace_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestBuildRuntimeRepairPlan_HealthyNoop(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	result := mustDiscover(t, root)
	plan := workspace.BuildRuntimeRepairPlan(root, result.Runtime)
	if !plan.Healthy || plan.NeedsApply() {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestBuildRuntimeRepairPlan_BlockedWhenNotInitialized(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	result := mustDiscover(t, root)
	plan := workspace.BuildRuntimeRepairPlan(root, result.Runtime)
	if !plan.Blocked || plan.NeedsApply() {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestApplyRuntimeRepair_MissingAgents(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, root)
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !containsPath(plan.Creates, config.FileAgentsMD) {
		t.Fatalf("creates = %#v", plan.Creates)
	}

	fixed := time.Date(2026, 10, 5, 15, 0, 0, 0, time.UTC)
	result := applyRepair(t, root, plan.Signature(), func() time.Time { return fixed })
	if result.Noop || len(result.Created) == 0 {
		t.Fatalf("result = %#v", result)
	}
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !config.InspectAgentsMarkers(agents).Complete() {
		t.Fatalf("markers incomplete:\n%s", agents)
	}
	assertDoctorRuntimeReady(t, root)
	if snapshotEqual(before, snapshotTree(t, root)) {
		t.Fatal("expected filesystem change")
	}
}

func TestApplyRuntimeRepair_BrokenMarkersBackupReplace(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	old := "# unmarked local agents\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 10, 5, 15, 1, 0, 0, time.UTC)
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	result := applyRepair(t, root, plan.Signature(), func() time.Time { return fixed })
	if len(result.Replaced) == 0 {
		t.Fatalf("expected replace: %#v", result)
	}
	backup := filepath.Join(root, ".atlas", "backups", "20261005T150100Z", "AGENTS.md")
	data, err := os.ReadFile(backup)
	if err != nil || string(data) != old {
		t.Fatalf("backup = %q err=%v", data, err)
	}
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !config.InspectAgentsMarkers(agents).Complete() {
		t.Fatal("new agents markers")
	}
	if strings.Contains(string(agents), "unmarked local agents") {
		t.Fatal("unmarked content must not merge into USER")
	}
	assertDoctorRuntimeReady(t, root)
}

func TestApplyRuntimeRepair_PreservesUserWhenMarkersValid(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("old claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	agentsBefore, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	applyRepair(t, root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 5, 15, 2, 0, 0, time.UTC)
	})
	agentsAfter, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(agentsBefore) != string(agentsAfter) {
		t.Fatal("valid AGENTS.md must be left unchanged")
	}
	if _, err := os.Stat(filepath.Join(root, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatal("CLAUDE.md should be quarantined")
	}
}

func TestApplyRuntimeRepair_MissingAdapterProjections(t *testing.T) {
	t.Parallel()
	cursorRoot := materializeProject(t, []string{"cursor"}, true)
	if err := os.Remove(filepath.Join(cursorRoot, ".cursor", "rules", "atlas.mdc")); err != nil {
		t.Fatal(err)
	}
	plan := workspace.BuildRuntimeRepairPlan(cursorRoot, mustDiscover(t, cursorRoot).Runtime)
	applyRepair(t, cursorRoot, plan.Signature(), nil)
	if _, err := os.Stat(filepath.Join(cursorRoot, config.FileCursorAtlasMDC)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cursorRoot, config.FileOpenCodeAtlas)); !os.IsNotExist(err) {
		t.Fatal("unselected opencode projection created")
	}
	assertDoctorRuntimeReady(t, cursorRoot)

	opencodeRoot := materializeProject(t, []string{"opencode"}, true)
	if err := os.Remove(filepath.Join(opencodeRoot, ".opencode", "atlas.md")); err != nil {
		t.Fatal(err)
	}
	plan = workspace.BuildRuntimeRepairPlan(opencodeRoot, mustDiscover(t, opencodeRoot).Runtime)
	applyRepair(t, opencodeRoot, plan.Signature(), nil)
	if _, err := os.Stat(filepath.Join(opencodeRoot, config.FileOpenCodeAtlas)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(opencodeRoot, config.FileCursorAtlasMDC)); !os.IsNotExist(err) {
		t.Fatal("unselected cursor projection created")
	}
	assertDoctorRuntimeReady(t, opencodeRoot)
}

func TestApplyRuntimeRepair_NonAtlasProjectionContentReplace(t *testing.T) {
	t.Parallel()

	cursorRoot := materializeProject(t, []string{"cursor"}, true)
	oldCursor := "not an atlas cursor projection\n"
	if err := os.WriteFile(filepath.Join(cursorRoot, config.FileCursorAtlasMDC), []byte(oldCursor), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := workspace.BuildRuntimeRepairPlan(cursorRoot, mustDiscover(t, cursorRoot).Runtime)
	if !containsPath(plan.Replaces, config.FileCursorAtlasMDC) || !containsPath(plan.Backups, config.FileCursorAtlasMDC) {
		t.Fatalf("cursor plan = %#v", plan)
	}
	fixed := time.Date(2026, 10, 5, 15, 4, 0, 0, time.UTC)
	result := applyRepair(t, cursorRoot, plan.Signature(), func() time.Time { return fixed })
	if !containsPath(result.Replaced, config.FileCursorAtlasMDC) {
		t.Fatalf("result = %#v", result)
	}
	backed, err := os.ReadFile(filepath.Join(cursorRoot, ".atlas", "backups", "20261005T150400Z", ".cursor", "rules", "atlas.mdc"))
	if err != nil || string(backed) != oldCursor {
		t.Fatalf("cursor backup = %q err=%v", backed, err)
	}
	got, err := os.ReadFile(filepath.Join(cursorRoot, config.FileCursorAtlasMDC))
	if err != nil {
		t.Fatal(err)
	}
	want := config.RenderCursorAtlasMDC("demo")
	if string(got) != want {
		t.Fatalf("cursor content =\n%s\nwant\n%s", got, want)
	}
	assertDoctorRuntimeReady(t, cursorRoot)

	opencodeRoot := materializeProject(t, []string{"opencode"}, true)
	oldOpen := "not an atlas opencode projection\n"
	if err := os.WriteFile(filepath.Join(opencodeRoot, config.FileOpenCodeAtlas), []byte(oldOpen), 0o644); err != nil {
		t.Fatal(err)
	}
	plan = workspace.BuildRuntimeRepairPlan(opencodeRoot, mustDiscover(t, opencodeRoot).Runtime)
	if !containsPath(plan.Replaces, config.FileOpenCodeAtlas) || !containsPath(plan.Backups, config.FileOpenCodeAtlas) {
		t.Fatalf("opencode plan = %#v", plan)
	}
	fixed = time.Date(2026, 10, 5, 15, 5, 0, 0, time.UTC)
	result = applyRepair(t, opencodeRoot, plan.Signature(), func() time.Time { return fixed })
	if !containsPath(result.Replaced, config.FileOpenCodeAtlas) {
		t.Fatalf("result = %#v", result)
	}
	backed, err = os.ReadFile(filepath.Join(opencodeRoot, ".atlas", "backups", "20261005T150500Z", ".opencode", "atlas.md"))
	if err != nil || string(backed) != oldOpen {
		t.Fatalf("opencode backup = %q err=%v", backed, err)
	}
	got, err = os.ReadFile(filepath.Join(opencodeRoot, config.FileOpenCodeAtlas))
	if err != nil {
		t.Fatal(err)
	}
	want = config.RenderOpenCodeAtlas("demo")
	if string(got) != want {
		t.Fatalf("opencode content =\n%s\nwant\n%s", got, want)
	}
	assertDoctorRuntimeReady(t, opencodeRoot)
}

func TestApplyRuntimeRepair_MatchingProjectionIsNoop(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !plan.Healthy {
		t.Fatalf("expected healthy plan with matching projection: %#v", plan)
	}
}

func TestApplyRuntimeRepair_RejectsStalePlan(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	reviewed := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	sig := reviewed.Signature()
	if !reviewed.NeedsApply() {
		t.Fatal("expected reviewed plan to need apply")
	}

	// Mutate filesystem so the reviewed plan is stale.
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(config.RenderAgentsMD("demo", true, []string{"cursor"}, nil)), 0o644); err != nil {
		t.Fatal(err)
	}
	before := snapshotTree(t, root)

	result, err := workspace.ApplyRuntimeRepair(root, sig, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Stale {
		t.Fatalf("expected stale: %#v", result)
	}
	if result.MessageTitle != workspace.RepairStaleMessage {
		t.Fatalf("message = %q", result.MessageTitle)
	}
	if result.Plan.NeedsApply() {
		t.Fatalf("refreshed plan should be healthy: %#v", result.Plan)
	}
	assertUnchangedTree(t, root, before)
}

func TestApplyRuntimeRepair_QuarantinesCompetingArtifacts(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("claude\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "GEMINI.md"), []byte("gemini\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "x.md"), []byte("skill\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "x.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "rules", "other.mdc"), []byte("keep?\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fixed := time.Date(2026, 10, 5, 15, 3, 0, 0, time.UTC)
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	result := applyRepair(t, root, plan.Signature(), func() time.Time { return fixed })
	if len(result.Quarantined) == 0 {
		t.Fatalf("expected quarantine: %#v", result)
	}
	for _, path := range []string{"CLAUDE.md", "GEMINI.md", ".agents", ".claude"} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Fatalf("%s still active", path)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "rules", "other.mdc")); !os.IsNotExist(err) {
		t.Fatal("extra cursor rule still active")
	}
	if _, err := os.Stat(filepath.Join(root, config.FileCursorAtlasMDC)); err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := os.ReadFile(filepath.Join(root, ".atlas", "backups", "20261005T150300Z", "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest config.BackupManifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Reason != "runtime repair" || len(manifest.Entries) == 0 {
		t.Fatalf("manifest = %#v", manifest)
	}
	for _, entry := range manifest.Entries {
		if entry.OriginalPath == "" || entry.BackupPath == "" || entry.Action == "" || entry.Reason == "" || entry.Timestamp == "" || entry.Result == "" {
			t.Fatalf("incomplete entry %#v", entry)
		}
	}
	backed, err := os.ReadFile(filepath.Join(root, ".atlas", "backups", "20261005T150300Z", "CLAUDE.md"))
	if err != nil || string(backed) != "claude\n" {
		t.Fatalf("quarantine backup missing: %q %v", backed, err)
	}
	assertDoctorRuntimeReady(t, root)
}

func TestApplyRuntimeRepair_HealthyNoopDoesNotMutate(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	before := snapshotTree(t, root)
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	result := applyRepair(t, root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 5, 16, 0, 0, 0, time.UTC)
	})
	if !result.Noop {
		t.Fatalf("expected noop: %#v", result)
	}
	assertUnchangedTree(t, root, before)
}

func TestApplyRuntimeRepair_DoesNotCreateUnselectedAdapters(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	applyRepair(t, root, plan.Signature(), nil)
	if _, err := os.Stat(filepath.Join(root, ".opencode")); !os.IsNotExist(err) {
		t.Fatal("opencode created")
	}
}

func TestApplyRuntimeRepair_MissingBaseBlock(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	broken := "<!-- ATLAS:USER:BEGIN -->\nkeep me\n<!-- ATLAS:USER:END -->\n"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !containsPath(plan.Replaces, config.FileAgentsMD) {
		t.Fatalf("expected AGENTS replace for missing base: %#v", plan)
	}
	result := applyRepair(t, root, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 5, 17, 10, 0, 0, time.UTC)
	})
	if !containsPath(result.Replaced, config.FileAgentsMD) {
		t.Fatalf("result = %#v", result)
	}
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	markers := config.InspectAgentsMarkers(agents)
	if !markers.ContractSatisfied([]string{"cursor"}) {
		t.Fatalf("markers = %#v", markers)
	}
	if !strings.Contains(string(agents), "keep me") {
		t.Fatalf("USER body lost:\n%s", agents)
	}
	assertDoctorRuntimeReady(t, root)
}

func TestApplyRuntimeRepair_MissingSelectedAdapterBlock(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	baseOnly := config.RenderAgentsMD("demo", true, nil, []byte(
		config.AgentsUserBegin+"\nuser note\n"+config.AgentsUserEnd,
	))
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(baseOnly), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !containsPath(plan.Replaces, config.FileAgentsMD) {
		t.Fatalf("expected replace for missing adapter block: %#v", plan)
	}
	applyRepair(t, root, plan.Signature(), nil)
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), config.AdapterBlockBegin("cursor")) {
		t.Fatalf("cursor block missing:\n%s", agents)
	}
	if !strings.Contains(string(agents), "user note") {
		t.Fatalf("USER lost:\n%s", agents)
	}
	assertDoctorRuntimeReady(t, root)
}

func TestApplyRuntimeRepair_DriftedSelectedAdapterBlock(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(agents), "## Cursor Adapter Guidance", "## Cursor Adapter Guidance\nTAMPERED", 1)
	if tampered == string(agents) {
		t.Fatal("failed to tamper adapter block")
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !containsPath(plan.Replaces, config.FileAgentsMD) {
		t.Fatalf("expected replace for drifted adapters: %#v", plan)
	}
	applyRepair(t, root, plan.Signature(), nil)
	fixed, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(fixed), "TAMPERED") {
		t.Fatalf("drift not repaired:\n%s", fixed)
	}
	want := config.RenderAgentsMD("demo", true, []string{"cursor"}, fixed)
	if string(fixed) != want {
		t.Fatalf("repaired AGENTS.md does not match renderer")
	}
	assertDoctorRuntimeReady(t, root)
}

func TestApplyRuntimeRepair_RemovesUnselectedAdapterBlock(t *testing.T) {
	t.Parallel()
	root := materializeProject(t, []string{"cursor"}, true)
	both := config.RenderAgentsMD("demo", true, []string{"cursor", "opencode"}, []byte(
		config.AgentsUserBegin+"\nstay\n"+config.AgentsUserEnd,
	))
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(both), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := workspace.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if !containsPath(plan.Replaces, config.FileAgentsMD) {
		t.Fatalf("expected replace for unselected adapter block: %#v", plan)
	}
	applyRepair(t, root, plan.Signature(), nil)
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(agents), config.AdapterBlockBegin("opencode")) {
		t.Fatalf("unselected opencode block still present:\n%s", agents)
	}
	if !strings.Contains(string(agents), config.AdapterBlockBegin("cursor")) {
		t.Fatal("cursor block missing")
	}
	if !strings.Contains(string(agents), "stay") {
		t.Fatal("USER lost")
	}
	assertDoctorRuntimeReady(t, root)
}

func applyRepair(t *testing.T, root, signature string, nowFn func() time.Time) workspace.RuntimeRepairResult {
	t.Helper()
	result, err := workspace.ApplyRuntimeRepair(root, signature, nowFn)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stale {
		t.Fatalf("unexpected stale: %#v", result)
	}
	return result
}

func mustDiscover(t *testing.T, root string) workspace.DiscoveryResult {
	t.Helper()
	result, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func containsPath(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func snapshotEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func assertDoctorRuntimeReady(t *testing.T, root string) {
	t.Helper()
	result := mustDiscover(t, root)
	report := doctor.Evaluate(result)
	for _, check := range report.Checks {
		if strings.HasPrefix(check.Name, "tool ") {
			continue
		}
		if check.Name == "git" || check.Name == "branch" || check.Name == "remotes" || check.Name == "workspace" {
			continue
		}
		if check.Severity == doctor.SeverityFail {
			t.Fatalf("runtime doctor fail %s %s: %s", check.Severity, check.Name, check.Message)
		}
	}
	if !result.Runtime.AgentsExists || !result.Runtime.AgentsMarkers.Complete() {
		t.Fatalf("agents not healthy: %#v", result.Runtime)
	}
}
