// Command smokemvp exercises Atlas MVP lower-level flows in temporary
// workspaces. It is invoked by scripts/smoke-mvp.sh and never writes into
// the Atlas repository root.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
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
	steps := []struct {
		name string
		fn   func() error
	}{
		{"cli routes (no console reports)", checkCLIRoutes},
		{"fresh non-Atlas project", checkFreshProject},
		{"init materialization happy path", checkInitHappyPath},
		{"initialized Cursor project", checkCursorProject},
		{"initialized OpenCode project", checkOpenCodeProject},
		{"status + doctor surfaces", checkStatusDoctor},
		{"runtime repair healthy no-op", checkRepairHealthyNoop},
		{"runtime repair missing AGENTS.md", checkRepairMissingAgents},
		{"runtime repair missing adapter projection", checkRepairMissingProjection},
		{"runtime repair stale plan rejection", checkRepairStalePlan},
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

func checkInitHappyPath() error {
	root, err := materialize([]string{"cursor", "opencode"}, true)
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)

	for _, rel := range []string{
		config.FileConfig,
		config.FileState,
		config.FileAgentsMD,
		config.FileCursorAtlasMDC,
		config.FileOpenCodeAtlas,
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return fmt.Errorf("missing %s: %w", rel, err)
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
	if !strings.Contains(status, "Atlas Status") || !strings.Contains(status, "AGENTS.md") {
		return fmt.Errorf("status render missing expected headings")
	}
	report := doctor.Evaluate(result)
	doctorView := screens.Doctor(report)
	if !strings.Contains(doctorView, "Atlas Doctor") {
		return fmt.Errorf("doctor render missing heading")
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
	if err := os.WriteFile(filepath.Join(root, config.FileAgentsMD), []byte(config.RenderAgentsMD("smoke", true, nil)), 0o644); err != nil {
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
	root, err := os.MkdirTemp(smokeTempBase(), "atlas-smoke-*")
	if err != nil {
		return "", err
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "smoke",
		ProjectMode:   "existing",
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
	return nil
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
