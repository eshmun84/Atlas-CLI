// Command smokemvp exercises Atlas Alpha 2 lower-level flows in temporary
// workspaces. It is invoked by scripts/smoke-mvp.sh and never writes into
// the Atlas repository root. Callers must set ATLAS_HOME to a temporary path.
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
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
		{"unsupported commands exit non-zero and do not mutate", checkUnsupportedExitNonZero},
		{"empty directory (fresh non-Atlas)", checkFreshProject},
		{"Git + README existing project", checkGitReadmeExistingProject},
		{"init fresh project (mode=new)", checkInitFreshNew},
		{"init existing project (mode=existing)", checkInitExisting},
		{"init Cursor+OpenCode happy path", checkInitHappyPath},
		{"atlas home mirrored on init", checkAtlasHome},
		{"ATLAS_HOME isolates default ~/.atlas", checkDefaultHomeUntouched},
		{"same-name different-root isolation", checkSameNameDifferentRootIsolation},
		{"initialized Cursor project", checkCursorProject},
		{"initialized OpenCode project", checkOpenCodeProject},
		{"Configure config-only behavior", checkConfigureConfigOnly},
		{"Init Home reset gate", checkInitHomeResetGate},
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
		{nil, cli.ModeTUI, tui.RouteStatus},
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

func checkUnsupportedExitNonZero() error {
	atlasHome := os.Getenv("ATLAS_HOME")
	if atlasHome == "" {
		return fmt.Errorf("ATLAS_HOME unset")
	}
	probe, err := os.MkdirTemp(smokeTempBase(), "atlas-smoke-unsupported-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(probe)
	if err := os.WriteFile(filepath.Join(probe, "marker.txt"), []byte("keep\n"), 0o644); err != nil {
		return err
	}
	beforeProbe, err := snapshotPaths(probe)
	if err != nil {
		return err
	}
	beforeHome, err := snapshotOptionalTree(atlasHome)
	if err != nil {
		return err
	}

	prev := cli.RunTUI
	cli.RunTUI = func(opts tui.Options) error {
		if opts.Route != tui.RouteError {
			return fmt.Errorf("expected RouteError, got %v", opts.Route)
		}
		return nil
	}
	defer func() { cli.RunTUI = prev }()

	for _, args := range [][]string{{"start"}, {"change"}, {"mcp"}} {
		if err := cli.Execute(io.Discard, io.Discard, args); err == nil {
			return fmt.Errorf("%v: expected non-nil error after TUI error dialog", args)
		} else if !strings.Contains(err.Error(), "unsupported command") {
			return fmt.Errorf("%v: error %v missing unsupported command", args, err)
		}
	}

	afterProbe, err := snapshotPaths(probe)
	if err != nil {
		return err
	}
	afterHome, err := snapshotOptionalTree(atlasHome)
	if err != nil {
		return err
	}
	if err := assertSnapshotEqual("unsupported probe", beforeProbe, afterProbe); err != nil {
		return err
	}
	if err := assertSnapshotEqual("ATLAS_HOME after unsupported", beforeHome, afterHome); err != nil {
		return err
	}

	// Supported routes and --version must still succeed with mocked TUI.
	cli.RunTUI = func(tui.Options) error { return nil }
	for _, args := range [][]string{nil, {"init"}, {"status"}, {"doctor"}, {"help"}} {
		if err := cli.Execute(io.Discard, io.Discard, args); err != nil {
			return fmt.Errorf("%v: unexpected error %v", args, err)
		}
	}
	if err := cli.Execute(io.Discard, io.Discard, []string{"--version"}); err != nil {
		return fmt.Errorf("--version: unexpected error %v", err)
	}
	return nil
}

func checkGitReadmeExistingProject() error {
	root, err := os.MkdirTemp(smokeTempBase(), "atlas-smoke-git-readme-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# existing project\n"), 0o644); err != nil {
		return err
	}
	if err := initGitRepo(root, "main"); err != nil {
		return err
	}

	before, err := snapshotPaths(root)
	if err != nil {
		return err
	}
	disc, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	afterDiscover, err := snapshotPaths(root)
	if err != nil {
		return err
	}
	if err := assertSnapshotEqual("discover git+readme", before, afterDiscover); err != nil {
		return err
	}
	if !disc.Git.IsRepo {
		return fmt.Errorf("expected git repo")
	}
	if !disc.Files.HasReadme {
		return fmt.Errorf("expected README.md")
	}
	if disc.Atlas.Initialized() || disc.Runtime.Initialized {
		return fmt.Errorf("git+readme project should not be Atlas-initialized yet")
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "git-readme",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	})
	if !draft.ToggleMulti("adapters.selected", "cursor") {
		return fmt.Errorf("toggle cursor")
	}
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   fixedNow(2026, 10, 7, 12, 0, 0),
	}); err != nil {
		return err
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || string(readme) != "# existing project\n" {
		return fmt.Errorf("README mutated by init: %q err=%v", readme, err)
	}
	return assertRuntimeReady(root)
}

func checkSameNameDifferentRootIsolation() error {
	homeDir := os.Getenv("ATLAS_HOME")
	if homeDir == "" {
		return fmt.Errorf("ATLAS_HOME unset")
	}
	rootA, err := materializeNamed("same-name", []string{"cursor"}, "existing")
	if err != nil {
		return err
	}
	defer os.RemoveAll(rootA)
	rootB, err := materializeNamed("same-name", []string{"opencode"}, "existing")
	if err != nil {
		return err
	}
	defer os.RemoveAll(rootB)

	idA, err := home.ProjectID(rootA, "same-name")
	if err != nil {
		return err
	}
	idB, err := home.ProjectID(rootB, "same-name")
	if err != nil {
		return err
	}
	if idA == idB {
		return fmt.Errorf("same name different root must yield distinct project ids: %q", idA)
	}
	if !home.ProjectDataPresent(homeDir, idA) || !home.ProjectDataPresent(homeDir, idB) {
		return fmt.Errorf("expected Home data for both project ids")
	}
	if err := home.ResetProject(homeDir, idA); err != nil {
		return err
	}
	if home.ProjectDataPresent(homeDir, idA) {
		return fmt.Errorf("project A Home data should be reset")
	}
	if !home.ProjectDataPresent(homeDir, idB) {
		return fmt.Errorf("project B Home data must remain after resetting A")
	}
	if _, err := os.Stat(filepath.Join(rootB, config.FileAgentsMD)); err != nil {
		return fmt.Errorf("product repo B must remain: %w", err)
	}
	return nil
}

func checkConfigureConfigOnly() error {
	root, err := materialize([]string{"cursor"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	agentsBefore, err := os.ReadFile(filepath.Join(root, config.FileAgentsMD))
	if err != nil {
		return err
	}
	cursorBefore, err := os.ReadFile(filepath.Join(root, config.FileCursorAtlasMDC))
	if err != nil {
		return err
	}
	beforeTree, err := snapshotPaths(root)
	if err != nil {
		return err
	}

	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName:           "smoke",
		ProjectMode:           "existing",
		ToolCursorAvailable:   true,
		ToolOpenCodeAvailable: true,
	})
	if !draft.ToggleMulti("adapters.selected", "cursor") {
		return fmt.Errorf("toggle cursor")
	}
	if !draft.ToggleMulti("adapters.selected", "opencode") {
		return fmt.Errorf("toggle opencode")
	}
	mcp := config.EmptyMCPDraft()
	mcp.ToggleBuiltin(0)
	res, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp})
	if err != nil {
		return err
	}
	if !strings.Contains(res.Notice, "config.yaml") {
		return fmt.Errorf("missing config-only notice: %q", res.Notice)
	}
	if !res.Impact.RuntimeRepairNeeded {
		return fmt.Errorf("adapter change should recommend Runtime Repair")
	}
	agentsAfter, err := os.ReadFile(filepath.Join(root, config.FileAgentsMD))
	if err != nil {
		return err
	}
	if string(agentsAfter) != string(agentsBefore) {
		return fmt.Errorf("Configure mutated AGENTS.md")
	}
	cursorAfter, err := os.ReadFile(filepath.Join(root, config.FileCursorAtlasMDC))
	if err != nil {
		return err
	}
	if string(cursorAfter) != string(cursorBefore) {
		return fmt.Errorf("Configure mutated Cursor projection")
	}
	if _, err := os.Stat(filepath.Join(root, config.FileOpenCodeAtlas)); !os.IsNotExist(err) {
		return fmt.Errorf("Configure must not materialize OpenCode projection")
	}
	afterTree, err := snapshotPaths(root)
	if err != nil {
		return err
	}
	configRel := filepath.ToSlash(config.FileConfig)
	for path, content := range beforeTree {
		got, ok := afterTree[path]
		if !ok {
			return fmt.Errorf("Configure removed %s", path)
		}
		if path == configRel {
			continue
		}
		if got != content {
			return fmt.Errorf("Configure mutated %s", path)
		}
	}
	doc, err := config.LoadProjectDocument(filepath.Join(root, config.FileConfig))
	if err != nil {
		return err
	}
	if !containsString(doc.Adapters.Selected, "opencode") {
		return fmt.Errorf("config.yaml missing opencode preference: %#v", doc.Adapters.Selected)
	}
	return nil
}

func checkInitHomeResetGate() error {
	root, err := materializeNamed("reset-gate", []string{"cursor"}, "existing")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	homeDir := os.Getenv("ATLAS_HOME")
	if homeDir == "" {
		return fmt.Errorf("ATLAS_HOME unset")
	}
	pid, err := home.ProjectID(root, "reset-gate")
	if err != nil {
		return err
	}
	if !home.ProjectDataPresent(homeDir, pid) {
		return fmt.Errorf("expected Home project data before re-init")
	}
	marker := filepath.Join(home.ProjectContextDir(homeDir, pid), "keep-me.txt")
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(marker, []byte("stale\n"), 0o644); err != nil {
		return err
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:         "reset-gate",
		ProjectMode:         "existing",
		ToolCursorAvailable: true,
	})
	if !draft.ToggleMulti("adapters.selected", "cursor") {
		return fmt.Errorf("toggle cursor")
	}
	review := initplan.BuildReview(initplan.ReviewInput{Draft: draft, MCP: config.EmptyMCPDraft(), Root: root})
	if !review.HomeDataDetected {
		return fmt.Errorf("review should detect Home data")
	}
	if review.AcceptHomeReset {
		return fmt.Errorf("AcceptHomeReset must default false")
	}
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(), AcceptHomeReset: false,
		Now: fixedNow(2026, 10, 7, 13, 0, 0),
	})
	if err == nil || !strings.Contains(err.Error(), "reset acceptance") {
		return fmt.Errorf("expected Home reset gate, got %v", err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		return fmt.Errorf("gate failure must not wipe Home data: %v", statErr)
	}

	applied, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(), AcceptHomeReset: true,
		Now: fixedNow(2026, 10, 7, 13, 1, 0),
	})
	if err != nil {
		return err
	}
	if !applied.HomeReset {
		return fmt.Errorf("expected HomeReset=true after accepted re-init")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		return fmt.Errorf("accepted reset must clear prior project Home data")
	}
	if !home.ProjectDataPresent(homeDir, pid) {
		return fmt.Errorf("re-init should recreate project Home layout")
	}
	return assertRuntimeReady(root)
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
	for _, want := range []string{
		"Atlas Status",
		"Workspace",
		"Atlas Runtime",
		"Source Control / Delivery Tools",
		"Project Technology",
		"Adapters",
		"Governance Tools",
		"MCP / External Context",
		"Health",
		"AGENTS.md contract",
		"SDD/OpenSpec contract",
		"Context Economy",
	} {
		if !strings.Contains(status, want) {
			return fmt.Errorf("status render missing %q", want)
		}
	}
	beforeTree, err := snapshotPaths(root)
	if err != nil {
		return err
	}
	report := doctor.Evaluate(result)
	doctorView := screens.Doctor(report, result)
	for _, want := range []string{
		"Atlas Doctor",
		"Overall Health",
		"WARNING",
		"Atlas Home",
		"Code Intelligence",
		"MCP / External Context",
	} {
		if !strings.Contains(doctorView, want) {
			return fmt.Errorf("doctor render missing %q", want)
		}
	}
	if !strings.Contains(status, "Code Intelligence") {
		return fmt.Errorf("status render missing Code Intelligence")
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
		if strings.Contains(target.Path, "/context/") || strings.Contains(target.Path, "projects/") || strings.HasSuffix(target.Path, "capsule.md") {
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
	return materializeNamedMode("smoke", adapters, "existing", contextGraph)
}

func materializeMode(adapters []string, contextGraph bool, projectMode string) (string, error) {
	return materializeNamedMode("smoke", adapters, projectMode, contextGraph)
}

func materializeNamed(projectName string, adapters []string, projectMode string) (string, error) {
	return materializeNamedMode(projectName, adapters, projectMode, true)
}

func materializeNamedMode(projectName string, adapters []string, projectMode string, contextGraph bool) (string, error) {
	root, err := os.MkdirTemp(smokeTempBase(), "atlas-smoke-*")
	if err != nil {
		return "", err
	}
	mode := strings.TrimSpace(projectMode)
	if mode == "" {
		mode = "existing"
	}
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "smoke"
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   name,
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

func initGitRepo(root, branch string) error {
	if _, err := exec.LookPath("git"); err != nil {
		return fmt.Errorf("git unavailable: %w", err)
	}
	cmd := exec.Command("git", "-C", root, "init", "--template=", "-b", branch)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TEMPLATE_DIR=",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git init: %v\n%s", err, out)
	}
	return nil
}

func assertSnapshotEqual(label string, before, after map[string]string) error {
	if len(before) != len(after) {
		return fmt.Errorf("%s: tree size changed before=%d after=%d", label, len(before), len(after))
	}
	for path, content := range before {
		got, ok := after[path]
		if !ok {
			return fmt.Errorf("%s: path removed: %s", label, path)
		}
		if got != content {
			return fmt.Errorf("%s: path mutated: %s", label, path)
		}
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
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
