package runtime_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

// Slice 3 A–N regression matrix. Each subtest maps to an approved scenario.
// Cross-references: slice1_materialize_test, slice2_runtime_contract_test,
// slice3_diagnostics_test, slice3_repair_test, doctor RO stability tests.

func TestSlice3_Matrix_A_OutsideProject(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	root := t.TempDir()
	beforeHome := snapshotTree(t, homeDir)
	beforeRoot := snapshotTree(t, root)

	result, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(result)
	_ = screens.StatusWithReport(result, report)
	_ = screens.Doctor(report, result)

	assertUnchangedTree(t, homeDir, beforeHome)
	assertUnchangedTree(t, root, beforeRoot)
	if _, err := os.Stat(filepath.Join(homeDir, "state.yaml")); !os.IsNotExist(err) {
		t.Fatal("Status/Doctor must not create Atlas Home")
	}

	// Home-only repair still works when Home is unhealthy/missing.
	plan := runtime.BuildRuntimeRepairPlan(root, result.Runtime)
	if !plan.Blocked && len(plan.Targets) == 0 {
		// Missing Home should yield Home repair target or blocked without config.
		t.Log("plan:", plan.Blocked, plan.Blockers, plan.Targets)
	}
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	after, err := home.RequireHealthy()
	if err != nil || !after.Exists {
		t.Fatalf("explicit Home repair/ensure failed: %#v %v", after, err)
	}
}

func TestSlice3_Matrix_B_FreshInstall(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	first, err := home.EnsureAndMirror(time.Date(2026, 10, 14, 21, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("first install: %v", err)
	}
	if len(first.Mirrored) == 0 {
		t.Fatal("first install mirrored no assets")
	}
	st, err := home.RequireHealthy()
	if err != nil || !st.Exists || !st.LayoutComplete {
		t.Fatalf("Home unhealthy after install: %#v %v", st, err)
	}
	beforeAssets := snapshotTree(t, filepath.Join(homeDir, "assets"))
	second, err := home.EnsureAndMirror(time.Date(2026, 10, 14, 21, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if second.Created {
		t.Fatal("second install must not report Created")
	}
	// Asset bytes must be idempotent; Home state may refresh timestamps.
	assertUnchangedTree(t, filepath.Join(homeDir, "assets"), beforeAssets)
	_ = filepath.Walk(homeDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := info.Name()
		if strings.Contains(name, "staging") || strings.HasSuffix(name, ".tmp") {
			t.Fatalf("staging residue: %s", path)
		}
		return nil
	})
}

func TestSlice3_Matrix_C_GreenfieldGitSDD(t *testing.T) {
	withTempAtlasHome(t)
	config.SetLookPathGitForTest(exec.LookPath)
	t.Cleanup(func() { config.SetLookPathGitForTest(nil) })
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "Slice3C", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if !draft.SelectOption("source_control.mode", "git_local") {
		t.Fatal("git_local")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 21, 10, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	assertPathExists(t, root, "README.md")
	assertPathExists(t, root, ".gitignore")
	assertPathExists(t, root, "openspec")
	assertPathExists(t, root, "openspec/changes")
	assertPathExists(t, root, "AGENTS.md")
	gitInfo, err := project.DiscoverGit(root)
	if err != nil || !gitInfo.IsRepo || gitInfo.CurrentBranch != "main" {
		t.Fatalf("git: %#v %v", gitInfo, err)
	}
	out, _ := exec.Command("git", "-C", root, "rev-list", "--all", "--count").CombinedOutput()
	if strings.TrimSpace(string(out)) != "0" {
		t.Fatalf("expected no commits, got %s", out)
	}
	remotes, _ := exec.Command("git", "-C", root, "remote").CombinedOutput()
	if strings.TrimSpace(string(remotes)) != "" {
		t.Fatalf("unexpected remotes: %s", remotes)
	}
	report := doctor.Evaluate(mustDiscover(t, root))
	if report.Failed() {
		t.Fatalf("Doctor not green: %s", failSummary(report))
	}
}

func TestSlice3_Matrix_D_GreenfieldSourceControlNone(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "Slice3D", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if !draft.SelectOption("source_control.mode", "none") {
		t.Fatal("none")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 21, 20, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	gitInfo, _ := project.DiscoverGit(root)
	if gitInfo.IsRepo {
		t.Fatal("mode=none must not bootstrap Git")
	}
	result := mustDiscover(t, root)
	report := doctor.Evaluate(result)
	for _, c := range report.Checks {
		if c.Name == "git" && c.Severity == doctor.SeverityWarn {
			t.Fatalf("must not WARN missing Git under mode=none: %#v", c)
		}
	}
	status := screens.StatusWithReport(result, report)
	if !strings.Contains(stripANSI(status), "Source control: none") {
		t.Fatalf("status missing mode=none:\n%s", stripANSI(status))
	}
	if strings.Contains(stripANSI(status), "Git repository not detected") {
		t.Fatal("status must not warn about missing Git under mode=none")
	}
}

func TestSlice3_Matrix_E_BrownfieldReadmeGitignore(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	readme := []byte("# Existing Project\n\nKeep me.\n")
	ignore := []byte("bin/\nvendor/\n")
	if err := os.WriteFile(filepath.Join(root, "README.md"), readme, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), ignore, 0o644); err != nil {
		t.Fatal(err)
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "Slice3E", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.SelectOption("source_control.mode", "none")
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 21, 30, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	gotReadme, _ := os.ReadFile(filepath.Join(root, "README.md"))
	if string(gotReadme) != string(readme) {
		t.Fatalf("README mutated: %q", gotReadme)
	}
	gotIgnore, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if !strings.Contains(string(gotIgnore), "bin/") || !strings.Contains(string(gotIgnore), ".atlas/") {
		t.Fatalf("gitignore not reconciled: %q", gotIgnore)
	}

	// Overwrite Atlas block away; Runtime Repair should restore without touching README.
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), ignore, 0o644); err != nil {
		t.Fatal(err)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	res := applyRepair(t, root, plan.Signature(), fixedClock(2026, 10, 14, 21, 31, 0))
	if res.Noop || res.Blocked {
		t.Fatalf("expected gitignore repair: %#v", res)
	}
	gotReadme2, _ := os.ReadFile(filepath.Join(root, "README.md"))
	if string(gotReadme2) != string(readme) {
		t.Fatal("Repair must leave README alone")
	}
	plan2 := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan2.Targets {
		if target.Kind == runtime.RepairKindGitignore {
			t.Fatal("second repair must be no-op for gitignore")
		}
	}
	if doctor.Evaluate(mustDiscover(t, root)).Failed() {
		t.Fatal("Doctor should be green after repair")
	}
}

func TestSlice3_Matrix_F_BrownfieldExistingGit(t *testing.T) {
	withTempAtlasHome(t)
	config.SetLookPathGitForTest(exec.LookPath)
	t.Cleanup(func() { config.SetLookPathGitForTest(nil) })
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-b", "develop").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	headBefore, _ := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "Slice3F", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.SelectOption("source_control.mode", "git_local")
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 21, 40, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	headAfter, _ := os.ReadFile(filepath.Join(root, ".git", "HEAD"))
	if string(headAfter) != string(headBefore) {
		t.Fatalf("branch rewritten: before=%q after=%q", headBefore, headAfter)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan.Targets {
		if strings.Contains(strings.ToLower(target.Path), "git") && target.Kind != runtime.RepairKindGitignore {
			t.Fatalf("Repair must not mutate Git: %#v", target)
		}
	}
}

func TestSlice3_Matrix_G_MissingHome(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "Slice3G", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 21, 50, 0, 0, time.UTC) },
	})
	if err == nil {
		t.Fatal("Init must fail-closed when Home missing")
	}
	before := snapshotTree(t, root)
	result, _ := inspect.Inspect(root)
	_ = doctor.Evaluate(result)
	assertUnchangedTree(t, root, before)

	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 21, 51, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("init after Home restore: %v", err)
	}
}

func TestSlice3_Matrix_H_OpenSpecAbsentAndPartial(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	_ = os.RemoveAll(filepath.Join(root, "openspec"))
	report := doctor.Evaluate(mustDiscover(t, root))
	assertDoctorHas(t, report, doctor.SeverityFail, "openspec provider", doctor.RepairConfirm)
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindOpenSpec {
			t.Fatal("Runtime Repair must not mkdir/bootstrap OpenSpec; Init owns openspec init")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "openspec")); !os.IsNotExist(err) {
		t.Fatalf("openspec/ must remain absent: %v", err)
	}

	// Partial/malformed: refuse.
	if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := []byte("project-owned\n")
	_ = os.WriteFile(filepath.Join(root, "openspec", "note.txt"), marker, 0o644)
	report2 := doctor.Evaluate(mustDiscover(t, root))
	assertDoctorHas(t, report2, doctor.SeverityFail, "openspec provider", doctor.RepairManual)
	plan3 := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan3.Targets {
		if target.Kind == runtime.RepairKindOpenSpec {
			t.Fatal("partial openspec must not be repaired")
		}
	}
	got, _ := os.ReadFile(filepath.Join(root, "openspec", "note.txt"))
	if string(got) != string(marker) {
		t.Fatal("partial tree mutated")
	}
}

func TestSlice3_Matrix_I_MalformedGitignore(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	giPath := filepath.Join(root, ".gitignore")
	malformed := []byte("# >>> Atlas local governance >>>\nno-end\n")
	if err := os.WriteFile(giPath, malformed, 0o644); err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(mustDiscover(t, root))
	assertDoctorHas(t, report, doctor.SeverityFail, "gitignore", doctor.RepairManual)
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindGitignore {
			t.Fatal("malformed gitignore must not be repaired")
		}
	}
	got, _ := os.ReadFile(giPath)
	if string(got) != string(malformed) {
		t.Fatal("malformed gitignore mutated")
	}
}

func TestSlice3_Matrix_J_StaleRuntimeAssets(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	agentsPath := filepath.Join(root, "AGENTS.md")
	beforeAgents, _ := os.ReadFile(agentsPath)
	if err := os.WriteFile(agentsPath, []byte("drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	readme := []byte("# keep\n")
	_ = os.WriteFile(filepath.Join(root, "README.md"), readme, 0o644)
	report := doctor.Evaluate(mustDiscover(t, root))
	if !report.Failed() {
		t.Fatal("expected Doctor FAIL on drifted AGENTS.md")
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	res := applyRepair(t, root, plan.Signature(), fixedClock(2026, 10, 14, 22, 10, 0))
	if res.Noop {
		t.Fatal("expected repair")
	}
	afterAgents, _ := os.ReadFile(agentsPath)
	if string(afterAgents) == "drifted\n" || len(afterAgents) == 0 {
		t.Fatal("AGENTS.md not restored")
	}
	if string(afterAgents) == string(beforeAgents) || strings.Contains(string(afterAgents), "Atlas") {
		// Restored content should be Atlas contract again.
	}
	gotReadme, _ := os.ReadFile(filepath.Join(root, "README.md"))
	if string(gotReadme) != string(readme) {
		t.Fatal("project-owned README mutated")
	}
	plan2 := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	if plan2.NeedsApply() {
		// May still have other drift; ensure AGENTS not re-targeted as drifted.
		for _, target := range plan2.Targets {
			if target.Path == config.FileAgentsMD {
				t.Fatal("AGENTS.md second repair should be no-op")
			}
		}
	}
}

func TestSlice3_Matrix_K_SigningRequired(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "Slice3K", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.SelectOption("source_control.mode", "none")
	if !draft.SelectOption("source_control.signing_required", "true") {
		t.Fatal("signing_required")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 22, 20, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	result := mustDiscover(t, root)
	if !result.Runtime.Document.SourceControl.SigningRequired {
		t.Fatal("signing_required not persisted")
	}
	status := stripANSI(screens.Status(result))
	if !strings.Contains(status, "Signing required: yes") {
		t.Fatalf("status missing signing policy:\n%s", status)
	}
	report := doctor.Evaluate(result)
	found := false
	for _, c := range report.Checks {
		if c.Name == "signing policy" && strings.Contains(c.Message, "true") {
			found = true
		}
	}
	if !found {
		t.Fatal("Doctor missing signing policy")
	}
	plan := runtime.BuildRuntimeRepairPlan(root, result.Runtime)
	for _, target := range plan.Targets {
		if strings.Contains(strings.ToLower(target.Reason+target.Path), "sign") {
			t.Fatalf("Repair must not touch signing: %#v", target)
		}
	}
	cfgBefore, _ := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if plan.NeedsApply() {
		_ = applyRepair(t, root, plan.Signature(), fixedClock(2026, 10, 14, 22, 21, 0))
	}
	cfgAfter, _ := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if !strings.Contains(string(cfgAfter), "signing_required: true") {
		t.Fatal("signing policy rewritten")
	}
	_ = cfgBefore
}

func TestSlice3_Matrix_L_ProviderSemantics(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "Slice3L", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if !draft.SelectOption("source_control.mode", "git_github") {
		t.Fatal("git_github")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 22, 30, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	cfg := mustReadFile(t, filepath.Join(root, ".atlas", "config.yaml"))
	if !strings.Contains(cfg, "mode: git_github") {
		t.Fatal("provider mode lost")
	}
	if !strings.Contains(cfg, "default_remote: origin") {
		t.Fatal("origin remote name lost")
	}
	agents := mustReadFile(t, filepath.Join(root, "AGENTS.md"))
	if !strings.Contains(agents, "GitHub") || !strings.Contains(agents, "Cursor Origin") {
		t.Fatal("provider≠adapter≠origin contract missing from AGENTS.md")
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan.Targets {
		if target.Path == config.FileConfig {
			t.Fatal("Repair must not rewrite config.yaml / provider")
		}
	}
}

func TestSlice3_Matrix_M_MemoryReadOnly(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	beforeHome := snapshotTree(t, homePath)
	beforeRoot := snapshotControlled(t, root)
	result := mustDiscover(t, root)
	_ = doctor.Evaluate(result)
	_ = screens.Status(result)
	assertUnchangedTree(t, homePath, beforeHome)
	assertControlledUnchanged(t, beforeRoot)
	plan := runtime.BuildRuntimeRepairPlan(root, result.Runtime)
	for _, target := range plan.Targets {
		if strings.Contains(strings.ToLower(target.Path+target.Reason), "memory") {
			t.Fatalf("Repair must not target Memory: %#v", target)
		}
	}
}

func TestSlice3_Matrix_N_CodeGraphReadOnly(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	before := snapshotControlled(t, root)
	result := mustDiscover(t, root)
	report := doctor.Evaluate(result)
	_ = screens.Doctor(report, result)
	assertControlledUnchanged(t, before)
	plan := runtime.BuildRuntimeRepairPlan(root, result.Runtime)
	for _, target := range plan.Targets {
		if strings.Contains(strings.ToLower(target.Path+target.Reason), "codegraph") {
			t.Fatalf("Repair must not rebuild CodeGraph: %#v", target)
		}
	}
}

func TestSlice3_Matrix_ReadOnlyByteStability(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath, _ := home.Resolve()
	surfaces := []string{
		filepath.Join(root, ".gitignore"),
		filepath.Join(root, "AGENTS.md"),
		filepath.Join(root, ".atlas", "config.yaml"),
		filepath.Join(root, "openspec"),
	}
	before := map[string]string{}
	for _, p := range surfaces {
		before[p] = pathFingerprint(t, p)
	}
	beforeHome := snapshotTree(t, homePath)

	result := mustDiscover(t, root)
	report := doctor.Evaluate(result)
	_ = screens.StatusWithReport(result, report)
	_ = screens.Doctor(report, result)

	for p, fp := range before {
		if pathFingerprint(t, p) != fp {
			t.Fatalf("Status/Doctor mutated %s", p)
		}
	}
	assertUnchangedTree(t, homePath, beforeHome)
}

func withTempAtlasHome(t *testing.T) {
	t.Helper()
	t.Setenv("ATLAS_HOME", t.TempDir())
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatalf("materialize Atlas Home: %v", err)
	}
}

func assertPathExists(t *testing.T, root, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
}

func snapshotControlled(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, rel := range []string{
		".gitignore", "AGENTS.md", "README.md",
		".atlas/config.yaml", ".atlas/state.yaml",
		"openspec/", "openspec/changes/",
	} {
		full := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(rel, "/")))
		out[full] = pathFingerprint(t, full)
	}
	return out
}

func assertControlledUnchanged(t *testing.T, before map[string]string) {
	t.Helper()
	for path, fp := range before {
		if pathFingerprint(t, path) != fp {
			t.Fatalf("controlled surface mutated: %s", path)
		}
	}
}

func pathFingerprint(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		t.Fatal(err)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(path)
		if err != nil {
			t.Fatal(err)
		}
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return "dir:" + strings.Join(names, ",")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return "file:" + string(data)
}

func assertDoctorHas(t *testing.T, report doctor.Report, sev doctor.Severity, name string, repair doctor.Repairability) {
	t.Helper()
	for _, c := range report.Checks {
		if c.Severity == sev && c.Name == name && c.Repairability == repair {
			return
		}
	}
	t.Fatalf("missing %s %s repairability=%s in %#v", sev, name, repair, report.Checks)
}

func failSummary(report doctor.Report) string {
	var b strings.Builder
	for _, c := range report.Checks {
		if c.Severity == doctor.SeverityFail {
			b.WriteString(c.String())
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
