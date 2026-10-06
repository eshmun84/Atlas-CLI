// Command smokemvp exercises Atlas Alpha lower-level flows in temporary
// workspaces. It is invoked by scripts/smoke-mvp.sh and never writes into
// the Atlas repository root. Callers must set ATLAS_HOME to a temporary path.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "smoke-mvp helper: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("smoke-mvp helper: ok")
}

func run() error {
	if strings.TrimSpace(os.Getenv("ATLAS_HOME")) == "" {
		homeDir, err := os.MkdirTemp(smokeTempBase(), "atlas-home-smoke-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(homeDir)
		if err := os.Setenv("ATLAS_HOME", homeDir); err != nil {
			return err
		}
	}

	steps := []struct {
		name string
		fn   func() error
	}{
		{"cli routes (no console reports)", checkCLIRoutes},
		{"fresh non-Atlas project", checkFreshProject},
		{"init fresh project (mode=new)", checkInitFreshNew},
		{"init existing project (mode=existing)", checkInitExisting},
		{"init Cursor+OpenCode happy path", checkInitHappyPath},
		{"atlas home mirrored on init", checkAtlasHome},
		{"ATLAS_HOME isolates default ~/.atlas", checkDefaultHomeUntouched},
		{"initialized Cursor project", checkCursorProject},
		{"initialized OpenCode project", checkOpenCodeProject},
		{"developer-owned files preserved", checkDeveloperOwnedPreserved},
		{"status + doctor surfaces", checkStatusDoctor},
		{"status/doctor do not create atlas home", checkStatusDoctorNoHomeCreate},
		{"runtime repair healthy no-op", checkRepairHealthyNoop},
		{"runtime repair missing AGENTS.md", checkRepairMissingAgents},
		{"runtime repair missing adapter projection", checkRepairMissingProjection},
		{"runtime repair Atlas agents + preserve external", checkRepairAtlasAgents},
		{"runtime repair SDD/OpenSpec contract", checkRepairSDDContract},
		{"runtime repair stale plan rejection", checkRepairStalePlan},
		{"context economy update + stale + readonly", checkContextEconomy},
	}
	for _, step := range steps {
		fmt.Printf("  • %s\n", step.name)
		if err := step.fn(); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return nil
}

func checkCLIRoutes() error {
	cases := []struct {
		args  []string
		mode  cli.Mode
		route tui.Route
	}{
		{nil, cli.ModeTUI, tui.RouteDashboard},
		{[]string{"init"}, cli.ModeTUI, tui.RouteInitPlan},
		{[]string{"status"}, cli.ModeTUI, tui.RouteStatus},
		{[]string{"doctor"}, cli.ModeTUI, tui.RouteDoctor},
		{[]string{"help"}, cli.ModeTUI, tui.RouteHelp},
		{[]string{"start"}, cli.ModeTUI, tui.RouteError},
		{[]string{"change"}, cli.ModeTUI, tui.RouteError},
		{[]string{"mcp"}, cli.ModeTUI, tui.RouteError},
		{[]string{"--version"}, cli.ModeVersion, tui.DefaultRoute},
	}
	for _, tc := range cases {
		action := cli.Resolve(tc.args)
		if action.Mode != tc.mode {
			return fmt.Errorf("args %v: mode=%v want %v", tc.args, action.Mode, tc.mode)
		}
		if tc.mode == cli.ModeTUI && action.Route != tc.route {
			return fmt.Errorf("args %v: route=%v want %v", tc.args, action.Route, tc.route)
		}
	}
	return nil
}

func checkFreshProject() error {
	root, err := os.MkdirTemp(smokeTempBase(), "atlas-smoke-fresh-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	if result.Atlas.Initialized() || result.Runtime.Initialized {
		return fmt.Errorf("fresh project reported as initialized")
	}
	plan := workspace.BuildRuntimeRepairPlan(root, result.Runtime)
	if !plan.Blocked {
		return fmt.Errorf("repair should be blocked on uninitialized project")
	}
	return nil
}

func checkInitFreshNew() error {
	root, err := materializeMode([]string{"cursor"}, true, "new")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	return assertRuntimeReady(root)
}

func checkInitExisting() error {
	root, err := materializeMode([]string{"opencode"}, true, "existing")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	return assertRuntimeReady(root)
}

func checkInitHappyPath() error {
	root, err := materialize([]string{"cursor", "opencode"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	if got := len(assets.AtlasAgentFilenames); got != 14 {
		return fmt.Errorf("expected 14 Atlas agents in pack, got %d", got)
	}

	for _, rel := range []string{
		config.FileConfig,
		config.FileState,
		config.FileAgentsMD,
		config.FileCursorAtlasMDC,
		config.FileOpenCodeAtlas,
		config.FileAgentRegistry,
		config.FileRuntimeManifest,
		config.FileAssetsLock,
		config.FileSDDOpenSpecContract,
		".cursor/agents/atlas-orchestrator.md",
		".opencode/agents/atlas-orchestrator.md",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return fmt.Errorf("missing %s: %w", rel, err)
		}
	}
	agentPaths := config.AtlasAgentRuntimePaths([]string{"cursor", "opencode"})
	if want := 14 * 2; len(agentPaths) != want {
		return fmt.Errorf("expected %d agent runtime paths, got %d", want, len(agentPaths))
	}
	for _, path := range agentPaths {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			return fmt.Errorf("missing atlas agent %s: %w", path, err)
		}
	}
	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	if !result.Runtime.Initialized || !result.Runtime.RuntimeMaterialized {
		return fmt.Errorf("runtime not materialized: %#v", result.Runtime)
	}
	if !result.Runtime.AgentsMarkers.Complete() {
		return fmt.Errorf("AGENTS.md markers incomplete")
	}
	if !result.Runtime.AgentRegistryMatches || !result.Runtime.RuntimeManifestMatches {
		return fmt.Errorf("registry/manifest unhealthy: %#v", result.Runtime)
	}
	if !result.Runtime.DependsOnSDDContract || !result.Runtime.SDDContractPresent || !result.Runtime.SDDContractMatches {
		return fmt.Errorf("sdd contract unhealthy: %#v", result.Runtime)
	}
	agentsText, err := os.ReadFile(filepath.Join(root, config.FileAgentRegistry))
	if err != nil {
		return err
	}
	if !strings.Contains(string(agentsText), "Atlas Home:") {
		return fmt.Errorf("agent registry missing Atlas Home reference")
	}
	if !strings.Contains(string(agentsText), config.FileSDDOpenSpecContract) {
		return fmt.Errorf("agent registry missing SDD contract reference")
	}
	lockText, err := os.ReadFile(filepath.Join(root, config.FileAssetsLock))
	if err != nil {
		return err
	}
	if !strings.Contains(string(lockText), "home_path:") || !strings.Contains(string(lockText), "checksum:") {
		return fmt.Errorf("assets.lock missing home/checksum metadata")
	}
	if !strings.Contains(string(lockText), "contracts/sdd-openspec.md") {
		return fmt.Errorf("assets.lock missing SDD contract entry")
	}
	orch, err := os.ReadFile(filepath.Join(root, ".cursor", "agents", "atlas-orchestrator.md"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(orch), config.FileSDDOpenSpecContract) {
		return fmt.Errorf("orchestrator missing SDD contract reference")
	}
	return nil
}

func checkAtlasHome() error {
	homeDir := os.Getenv("ATLAS_HOME")
	if homeDir == "" {
		return fmt.Errorf("ATLAS_HOME unset")
	}
	root, err := materialize([]string{"cursor"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	resolved, err := home.Resolve()
	if err != nil {
		return err
	}
	if filepath.Clean(resolved) != filepath.Clean(homeDir) {
		return fmt.Errorf("home.Resolve=%q want ATLAS_HOME=%q", resolved, homeDir)
	}

	for _, dir := range []string{"assets", "agents", "skills", "rules", "templates", "adapters", "contracts", "context", "state"} {
		if info, err := os.Stat(filepath.Join(homeDir, dir)); err != nil || !info.IsDir() {
			return fmt.Errorf("home layout missing %s: %v", dir, err)
		}
	}
	for _, rel := range []string{
		"assets/agents/base.md",
		"assets/agents/adapters/cursor.md",
		"assets/agents/runtime/atlas-orchestrator.md",
		"assets/adapter-files/cursor/atlas.mdc",
		"assets/contracts/sdd-openspec.md",
		"agents/atlas-orchestrator.md",
		"adapters/cursor/atlas.mdc",
		"contracts/sdd-openspec.md",
		"state/home.yaml",
	} {
		if _, err := os.Stat(filepath.Join(homeDir, rel)); err != nil {
			return fmt.Errorf("home asset missing %s: %w", rel, err)
		}
	}
	return nil
}

func checkDefaultHomeUntouched() error {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	defaultHome := filepath.Join(userHome, home.DefaultDirName)
	before, err := snapshotOptionalTree(defaultHome)
	if err != nil {
		return err
	}

	root, err := materialize([]string{"cursor", "opencode"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	objective := "smoke isolation"
	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	plan := atlascontext.BuildUpdatePlan(root, true, result.Runtime.State, objective)
	if plan.NeedsApply() {
		if _, err := atlascontext.ApplyUpdate(root, plan.Signature(), result.Runtime.State, objective, fixedNow(2026, 10, 6, 19, 0, 0)); err != nil {
			return err
		}
	}

	after, err := snapshotOptionalTree(defaultHome)
	if err != nil {
		return err
	}
	if len(before) != len(after) {
		return fmt.Errorf("default ~/.atlas tree size changed while ATLAS_HOME set")
	}
	for path, content := range before {
		if after[path] != content {
			return fmt.Errorf("default ~/.atlas mutated at %s while ATLAS_HOME set", path)
		}
	}
	atlasHome := os.Getenv("ATLAS_HOME")
	if atlasHome == "" {
		return fmt.Errorf("ATLAS_HOME unset")
	}
	if _, err := os.Stat(filepath.Join(atlasHome, "state", "home.yaml")); err != nil {
		return fmt.Errorf("expected writes under ATLAS_HOME: %w", err)
	}
	return nil
}

func checkDeveloperOwnedPreserved() error {
	root, err := os.MkdirTemp(smokeTempBase(), "atlas-smoke-owned-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	owned := map[string]string{
		"README.md":  "# developer readme\n",
		".gitignore": "bin/\n.tmp/\n",
		"notes.txt":  "keep\n",
	}
	for rel, body := range owned {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			return err
		}
	}
	external := filepath.Join(root, ".cursor", "agents", "external-helper.md")
	if err := os.MkdirAll(filepath.Dir(external), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(external, []byte("external keep\n"), 0o644); err != nil {
		return err
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "owned",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	})
	for _, adapter := range []string{"cursor", "opencode"} {
		if !draft.ToggleMulti("adapters.selected", adapter) {
			return fmt.Errorf("toggle adapter %s", adapter)
		}
	}
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   fixedNow(2026, 10, 5, 12, 30, 0),
	}); err != nil {
		return err
	}

	// Drift an Atlas agent so Repair must rewrite Atlas-owned files only.
	if err := os.WriteFile(filepath.Join(root, ".cursor", "agents", "atlas-worker.md"), []byte("drift\n"), 0o644); err != nil {
		return err
	}
	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	plan := workspace.BuildRuntimeRepairPlan(root, result.Runtime)
	if !plan.NeedsApply() {
		return fmt.Errorf("expected repair for agent drift")
	}
	if _, err := workspace.ApplyRuntimeRepair(root, plan.Signature(), fixedNow(2026, 10, 5, 12, 31, 0)); err != nil {
		return err
	}

	for rel, body := range owned {
		got, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil || string(got) != body {
			return fmt.Errorf("developer-owned %s mutated: %q err=%v", rel, got, err)
		}
	}
	ext, err := os.ReadFile(external)
	if err != nil || string(ext) != "external keep\n" {
		return fmt.Errorf("external agent mutated: %q err=%v", ext, err)
	}
	for _, forbidden := range []string{"CLAUDE.md", "GEMINI.md", ".agents", ".claude"} {
		if _, err := os.Stat(filepath.Join(root, forbidden)); !os.IsNotExist(err) {
			return fmt.Errorf("forbidden path unexpectedly present: %s", forbidden)
		}
	}
	return assertRuntimeReady(root)
}

func checkStatusDoctorNoHomeCreate() error {
	prev := os.Getenv("ATLAS_HOME")
	missing := filepath.Join(smokeTempBase(), "atlas-home-should-stay-missing")
	_ = os.RemoveAll(missing)
	if err := os.Setenv("ATLAS_HOME", missing); err != nil {
		return err
	}
	defer func() { _ = os.Setenv("ATLAS_HOME", prev) }()

	root, err := os.MkdirTemp(smokeTempBase(), "atlas-smoke-readonly-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	_ = doctor.Evaluate(result)
	_ = screens.Status(result)
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		return fmt.Errorf("status/doctor created Atlas Home at %s", missing)
	}
	return nil
}

func checkCursorProject() error {
	root, err := materialize([]string{"cursor"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	if _, err := os.Stat(filepath.Join(root, config.FileCursorAtlasMDC)); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, config.FileOpenCodeAtlas)); !os.IsNotExist(err) {
		return fmt.Errorf("unselected OpenCode projection present")
	}
	return assertRuntimeReady(root)
}

func checkOpenCodeProject() error {
	root, err := materialize([]string{"opencode"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	if _, err := os.Stat(filepath.Join(root, config.FileOpenCodeAtlas)); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, config.FileCursorAtlasMDC)); !os.IsNotExist(err) {
		return fmt.Errorf("unselected Cursor projection present")
	}
	return assertRuntimeReady(root)
}

func checkStatusDoctor() error {
	root, err := materialize([]string{"cursor"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	status := screens.Status(result)
	if !strings.Contains(status, "Atlas Status") || !strings.Contains(status, "AGENTS.md") ||
		!strings.Contains(status, "SDD/OpenSpec contract") || !strings.Contains(status, "Context Economy") {
		return fmt.Errorf("status render missing expected headings")
	}
	beforeTree, err := snapshotPaths(root)
	if err != nil {
		return err
	}
	report := doctor.Evaluate(result)
	doctorView := screens.Doctor(report)
	if !strings.Contains(doctorView, "Atlas Doctor") {
		return fmt.Errorf("doctor render missing heading")
	}
	afterTree, err := snapshotPaths(root)
	if err != nil {
		return err
	}
	if len(beforeTree) != len(afterTree) {
		return fmt.Errorf("status/doctor mutated project tree size")
	}
	for path, content := range beforeTree {
		if afterTree[path] != content {
			return fmt.Errorf("status/doctor mutated %s", path)
		}
	}
	for _, check := range report.Checks {
		if strings.HasPrefix(check.Name, "tool ") || check.Name == "git" || check.Name == "branch" || check.Name == "remotes" || check.Name == "workspace" {
			continue
		}
		if check.Severity == doctor.SeverityFail {
			return fmt.Errorf("doctor fail %s: %s", check.Name, check.Message)
		}
	}
	return nil
}

func checkRepairHealthyNoop() error {
	root, err := materialize([]string{"cursor"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	plan := workspace.BuildRuntimeRepairPlan(root, result.Runtime)
	if !plan.Healthy || plan.NeedsApply() {
		return fmt.Errorf("expected healthy plan: %#v", plan)
	}
	applied, err := workspace.ApplyRuntimeRepair(root, plan.Signature(), nil)
	if err != nil {
		return err
	}
	if !applied.Noop {
		return fmt.Errorf("expected noop: %#v", applied)
	}
	return nil
}

func checkRepairMissingAgents() error {
	root, err := materialize([]string{"cursor"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	if err := os.Remove(filepath.Join(root, config.FileAgentsMD)); err != nil {
		return err
	}
	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	plan := workspace.BuildRuntimeRepairPlan(root, result.Runtime)
	if !plan.NeedsApply() {
		return fmt.Errorf("expected apply for missing AGENTS.md: %#v", plan)
	}
	applied, err := workspace.ApplyRuntimeRepair(root, plan.Signature(), fixedNow(2026, 10, 5, 17, 0, 0))
	if err != nil {
		return err
	}
	if applied.Noop || applied.Stale {
		return fmt.Errorf("unexpected result: %#v", applied)
	}
	agents, err := os.ReadFile(filepath.Join(root, config.FileAgentsMD))
	if err != nil {
		return err
	}
	if !config.InspectAgentsMarkers(agents).Complete() {
		return fmt.Errorf("repaired AGENTS.md markers incomplete")
	}
	return assertRuntimeReady(root)
}

func checkRepairMissingProjection() error {
	root, err := materialize([]string{"cursor"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	if err := os.Remove(filepath.Join(root, config.FileCursorAtlasMDC)); err != nil {
		return err
	}
	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	plan := workspace.BuildRuntimeRepairPlan(root, result.Runtime)
	if !plan.NeedsApply() {
		return fmt.Errorf("expected apply for missing projection: %#v", plan)
	}
	if _, err := workspace.ApplyRuntimeRepair(root, plan.Signature(), fixedNow(2026, 10, 5, 17, 1, 0)); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, config.FileCursorAtlasMDC)); err != nil {
		return err
	}
	return assertRuntimeReady(root)
}

func checkRepairAtlasAgents() error {
	root, err := materialize([]string{"cursor", "opencode"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	external := filepath.Join(root, ".cursor", "agents", "external-helper.md")
	if err := os.WriteFile(external, []byte("keep me\n"), 0o644); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(root, ".cursor", "agents", "atlas-orchestrator.md")); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, ".opencode", "agents", "atlas-worker.md"), []byte("drift\n"), 0o644); err != nil {
		return err
	}

	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	plan := workspace.BuildRuntimeRepairPlan(root, result.Runtime)
	if !plan.NeedsApply() {
		return fmt.Errorf("expected apply for atlas agent drift: %#v", plan)
	}
	for _, path := range plan.Quarantines {
		if path == ".cursor/agents/external-helper.md" {
			return fmt.Errorf("external agent quarantined")
		}
	}
	if _, err := workspace.ApplyRuntimeRepair(root, plan.Signature(), fixedNow(2026, 10, 5, 17, 2, 0)); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "agents", "atlas-orchestrator.md")); err != nil {
		return err
	}
	want, err := config.RenderAtlasAgent("atlas-worker.md")
	if err != nil {
		return err
	}
	got, err := os.ReadFile(filepath.Join(root, ".opencode", "agents", "atlas-worker.md"))
	if err != nil {
		return err
	}
	if string(got) != want {
		return fmt.Errorf("opencode worker not restored")
	}
	ext, err := os.ReadFile(external)
	if err != nil || string(ext) != "keep me\n" {
		return fmt.Errorf("external agent touched: %q err=%v", ext, err)
	}
	return assertRuntimeReady(root)
}

func checkRepairSDDContract() error {
	root, err := materialize([]string{"cursor"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	external := filepath.Join(root, ".cursor", "agents", "external-helper.md")
	if err := os.WriteFile(external, []byte("keep me\n"), 0o644); err != nil {
		return err
	}
	contractPath := filepath.Join(root, filepath.FromSlash(config.FileSDDOpenSpecContract))
	if err := os.Remove(contractPath); err != nil {
		return err
	}
	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	plan := workspace.BuildRuntimeRepairPlan(root, result.Runtime)
	if !plan.NeedsApply() {
		return fmt.Errorf("expected apply for missing SDD contract: %#v", plan)
	}
	if _, err := workspace.ApplyRuntimeRepair(root, plan.Signature(), fixedNow(2026, 10, 5, 17, 3, 0)); err != nil {
		return err
	}
	want, err := config.RenderSDDOpenSpecContract()
	if err != nil {
		return err
	}
	got, err := os.ReadFile(contractPath)
	if err != nil || string(got) != want {
		return fmt.Errorf("contract not restored: %q err=%v", got, err)
	}
	ext, err := os.ReadFile(external)
	if err != nil || string(ext) != "keep me\n" {
		return fmt.Errorf("external agent touched: %q err=%v", ext, err)
	}
	return assertRuntimeReady(root)
}

func checkContextEconomy() error {
	root, err := materialize([]string{"cursor", "opencode"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	external := filepath.Join(root, ".cursor", "agents", "external-helper.md")
	if err := os.WriteFile(external, []byte("keep me\n"), 0o644); err != nil {
		return err
	}

	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	if result.Runtime.ContextEconomy.State != atlascontext.StatusMissing {
		return fmt.Errorf("expected missing context economy before update: %#v", result.Runtime.ContextEconomy)
	}

	before := snapshotPathsMust(root)
	_ = screens.Status(result)
	_ = doctor.Evaluate(result)
	after := snapshotPathsMust(root)
	if len(before) != len(after) {
		return fmt.Errorf("status/doctor mutated tree before context update")
	}

	objective := "implement SDD verify for atlas agents"
	plan := atlascontext.BuildUpdatePlan(root, true, result.Runtime.State, objective)
	if !plan.NeedsApply() {
		return fmt.Errorf("expected context update plan: %#v", plan)
	}
	applied, err := atlascontext.ApplyUpdate(root, plan.Signature(), result.Runtime.State, objective, fixedNow(2026, 10, 6, 18, 0, 0))
	if err != nil || applied.Noop {
		return fmt.Errorf("context apply failed: %#v err=%v", applied, err)
	}
	for _, path := range []string{applied.CapsulePath, applied.PackPath, atlascontext.IndexPath(plan.HomePath, plan.ProjectID)} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("missing context artifact %s: %w", path, err)
		}
	}
	packRaw, err := os.ReadFile(applied.PackPath)
	if err != nil {
		return err
	}
	if !strings.Contains(string(packRaw), "objective:") || !strings.Contains(string(packRaw), "candidates:") {
		return fmt.Errorf("pack incomplete:\n%s", packRaw)
	}
	if !strings.Contains(string(packRaw), "path:") || !strings.Contains(string(packRaw), "reason:") {
		return fmt.Errorf("pack missing path/reason:\n%s", packRaw)
	}
	if _, err := os.Stat(filepath.Join(root, "context")); !os.IsNotExist(err) {
		return fmt.Errorf("context payload written into product repo")
	}
	stateRaw, err := os.ReadFile(filepath.Join(root, config.FileState))
	if err != nil {
		return err
	}
	if !strings.Contains(string(stateRaw), "context_economy_project_id:") {
		return fmt.Errorf("state missing context economy refs")
	}

	refreshed, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	if refreshed.Runtime.ContextEconomy.State != atlascontext.StatusPresent {
		return fmt.Errorf("expected present context economy: %#v", refreshed.Runtime.ContextEconomy)
	}
	before = snapshotPathsMust(root)
	status := screens.Status(refreshed)
	_ = doctor.Evaluate(refreshed)
	if !strings.Contains(status, "Context Economy") {
		return fmt.Errorf("status missing Context Economy")
	}
	after = snapshotPathsMust(root)
	for path, content := range before {
		if after[path] != content {
			return fmt.Errorf("status/doctor mutated %s", path)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "extra_context_probe.go"), []byte("package main\n"), 0o644); err != nil {
		return err
	}
	staleDisc, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	if staleDisc.Runtime.ContextEconomy.State != atlascontext.StatusStale {
		return fmt.Errorf("expected stale after file change: %#v", staleDisc.Runtime.ContextEconomy)
	}

	// Runtime Repair must not delete/replace Context Economy Home payloads.
	capsuleBefore, err := os.ReadFile(applied.CapsulePath)
	if err != nil {
		return err
	}
	repairPlan := workspace.BuildRuntimeRepairPlan(root, staleDisc.Runtime)
	for _, target := range repairPlan.Targets {
		if strings.Contains(target.Path, "context/projects") || strings.HasSuffix(target.Path, "capsule.md") {
			return fmt.Errorf("repair targets context economy path: %#v", target)
		}
	}
	if repairPlan.NeedsApply() {
		if _, err := workspace.ApplyRuntimeRepair(root, repairPlan.Signature(), fixedNow(2026, 10, 6, 18, 5, 0)); err != nil {
			return err
		}
	}
	capsuleAfter, err := os.ReadFile(applied.CapsulePath)
	if err != nil {
		return err
	}
	if string(capsuleAfter) != string(capsuleBefore) {
		return fmt.Errorf("runtime repair mutated context capsule")
	}
	ext, err := os.ReadFile(external)
	if err != nil || string(ext) != "keep me\n" {
		return fmt.Errorf("external agent touched: %q err=%v", ext, err)
	}
	return nil
}

func snapshotPathsMust(root string) map[string]string {
	out, err := snapshotPaths(root)
	if err != nil {
		panic(err)
	}
	return out
}

func checkRepairStalePlan() error {
	root, err := materialize([]string{"cursor"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	if err := os.Remove(filepath.Join(root, config.FileAgentsMD)); err != nil {
		return err
	}
	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	reviewed := workspace.BuildRuntimeRepairPlan(root, result.Runtime)
	sig := reviewed.Signature()
	if !reviewed.NeedsApply() {
		return fmt.Errorf("reviewed plan should need apply")
	}

	// Heal AGENTS.md so the reviewed signature is stale.
	if err := os.WriteFile(filepath.Join(root, config.FileAgentsMD), []byte(config.RenderAgentsMD("smoke", true, []string{"cursor"}, nil)), 0o644); err != nil {
		return err
	}
	applied, err := workspace.ApplyRuntimeRepair(root, sig, nil)
	if err != nil {
		return err
	}
	if !applied.Stale {
		return fmt.Errorf("expected stale rejection: %#v", applied)
	}
	if applied.MessageTitle != workspace.RepairStaleMessage {
		return fmt.Errorf("stale message = %q", applied.MessageTitle)
	}
	return nil
}

func materialize(adapters []string, contextGraph bool) (string, error) {
	return materializeMode(adapters, contextGraph, "existing")
}

func materializeMode(adapters []string, contextGraph bool, projectMode string) (string, error) {
	root, err := os.MkdirTemp(smokeTempBase(), "atlas-smoke-*")
	if err != nil {
		return "", err
	}
	mode := strings.TrimSpace(projectMode)
	if mode == "" {
		mode = "existing"
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "smoke",
		ProjectMode:   mode,
		DefaultRemote: "origin",
	})
	for _, adapter := range adapters {
		if !draft.ToggleMulti("adapters.selected", adapter) {
			os.RemoveAll(root)
			return "", fmt.Errorf("toggle adapter %s", adapter)
		}
	}
	if !contextGraph {
		if !draft.ToggleBool("context.graph.enabled") {
			os.RemoveAll(root)
			return "", fmt.Errorf("toggle context graph")
		}
	}
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   fixedNow(2026, 10, 5, 12, 0, 0),
	}); err != nil {
		os.RemoveAll(root)
		return "", err
	}
	return root, nil
}

func snapshotOptionalTree(root string) (map[string]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}
	return snapshotPaths(root)
}

func assertRuntimeReady(root string) error {
	result, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	report := doctor.Evaluate(result)
	for _, check := range report.Checks {
		if strings.HasPrefix(check.Name, "tool ") || check.Name == "git" || check.Name == "branch" || check.Name == "remotes" || check.Name == "workspace" {
			continue
		}
		if check.Severity == doctor.SeverityFail {
			return fmt.Errorf("runtime doctor fail %s: %s", check.Name, check.Message)
		}
	}
	if !result.Runtime.AgentsExists || !result.Runtime.AgentsMarkers.Complete() {
		return fmt.Errorf("agents not healthy")
	}
	if !result.Runtime.DependsOnSDDContract || !result.Runtime.SDDContractPresent || !result.Runtime.SDDContractMatches {
		return fmt.Errorf("sdd contract not healthy")
	}
	return nil
}

func snapshotPaths(root string) (map[string]string, error) {
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
	return out, err
}

func smokeTempBase() string {
	if dir := strings.TrimSpace(os.Getenv("ATLAS_SMOKE_TMP")); dir != "" {
		return dir
	}
	return ""
}

func fixedNow(y int, m time.Month, d, hh, mm, ss int) func() time.Time {
	t := time.Date(y, m, d, hh, mm, ss, 0, time.UTC)
	return func() time.Time { return t }
}
