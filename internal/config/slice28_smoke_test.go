package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

// Slice 28 dogfooding smoke: temporary ATLAS_HOME + projects.
func TestSlice28DogfoodingTextQualitySmoke(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)

	pass := func(msg string) { t.Log("PASS", msg) }

	// Empty directory init (mode=new) with Cursor.
	empty := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:         "empty28",
		ProjectMode:         config.ModeGreenfield,
		ToolCursorAvailable: true,
		DocsScaffold:        true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if _, err := config.ApplyConfig(config.ApplyInput{Root: empty, Draft: draft, MCP: config.EmptyMCPDraft()}); err != nil {
		t.Fatalf("empty init: %v", err)
	}
	agents, err := os.ReadFile(filepath.Join(empty, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertTextQuality(t, "empty AGENTS.md", string(agents))
	if strings.Count(string(agents), "# Atlas Project Runtime Contract") != 1 {
		t.Fatal("empty project AGENTS.md duplicate H1")
	}
	if _, err := os.Stat(filepath.Join(empty, config.FileProjectDocsREADME)); err != nil {
		t.Fatal("docs scaffold missing after selected Init")
	}
	pass("empty project: readable AGENTS.md + docs scaffold")

	// Git + README existing project with Cursor + OpenCode.
	existing := t.TempDir()
	if err := os.WriteFile(filepath.Join(existing, "README.md"), []byte("readme\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(existing, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	exDraft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:           "existing28",
		ProjectMode:           config.ModeExisting,
		ToolCursorAvailable:   true,
		ToolOpenCodeAvailable: true,
	})
	_ = exDraft.ToggleMulti("adapters.selected", "cursor")
	_ = exDraft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.ApplyConfig(config.ApplyInput{Root: existing, Draft: exDraft, MCP: config.EmptyMCPDraft()}); err != nil {
		t.Fatalf("existing init: %v", err)
	}
	agents, err = os.ReadFile(filepath.Join(existing, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertTextQuality(t, "existing AGENTS.md", string(agents))
	assertContains(t, string(agents), "## Cursor Adapter Guidance", "## OpenCode Adapter Guidance")
	cursorRaw, err := os.ReadFile(filepath.Join(existing, ".cursor/rules/atlas.mdc"))
	if err != nil {
		t.Fatal(err)
	}
	assertTextQuality(t, "cursor projection", string(cursorRaw))
	ocRaw, err := os.ReadFile(filepath.Join(existing, ".opencode/atlas.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertTextQuality(t, "opencode projection", string(ocRaw))
	reg, err := os.ReadFile(filepath.Join(existing, ".atlas/agent-registry.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertTextQuality(t, "agent-registry", string(reg))
	contract, err := os.ReadFile(filepath.Join(existing, config.FileSDDOpenSpecContract))
	if err != nil {
		t.Fatal(err)
	}
	assertTextQuality(t, "sdd contract", string(contract))
	pass("existing Cursor+OpenCode: readable runtime assets")

	// Context Economy update.
	state, err := config.LoadStateDocumentAt(existing)
	if err != nil {
		t.Fatal(err)
	}
	plan := atlascontext.BuildUpdatePlan(existing, true, state, atlascontext.DefaultPackObjective)
	if plan.Blocked {
		t.Fatalf("context plan blocked: %#v", plan.Blockers)
	}
	if _, err := atlascontext.ApplyUpdate(existing, plan.Signature(), state, atlascontext.DefaultPackObjective, func() time.Time {
		return time.Now().UTC()
	}); err != nil {
		t.Fatalf("context update: %v", err)
	}
	ce := screens.RenderContextEconomy(screens.ContextEconomyView{Plan: plan, ContentFocused: true})
	if !strings.Contains(ce, "implemented, file-based") || !strings.Contains(ce, "NOT IMPLEMENTED") {
		t.Fatalf("context economy wording:\n%s", ce)
	}
	pass("Context Economy update + honest wording")

	// Runtime drift + repair.
	if err := os.Remove(filepath.Join(existing, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	disc, err := inspect.Inspect(existing)
	if err != nil {
		t.Fatal(err)
	}
	rplan := runtime.BuildRuntimeRepairPlan(existing, disc.Runtime)
	if !rplan.NeedsApply() {
		t.Fatal("expected repair after AGENTS.md delete")
	}
	if _, err := runtime.ApplyRuntimeRepair(existing, rplan.Signature(), func() time.Time { return time.Now().UTC() }); err != nil {
		t.Fatalf("repair: %v", err)
	}
	agents, err = os.ReadFile(filepath.Join(existing, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertTextQuality(t, "repaired AGENTS.md", string(agents))
	pass("Runtime Repair restores readable AGENTS.md")

	// Configure after init: honest notices; no silent non-MCP runtime rewrite.
	before := string(agents)
	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName:         "existing28",
		ProjectMode:         config.ModeExisting,
		ToolCursorAvailable: true,
	})
	_ = cfg.ToggleMulti("adapters.selected", "cursor")
	mcp := config.EmptyMCPDraft()
	mcp.EnableBuiltin(config.MCPBuiltinFilesystem)
	res, err := config.PersistConfigure(config.ApplyInput{Root: existing, Draft: cfg, MCP: mcp})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Notice, "Configuration changes are saved to .atlas/config.yaml.") {
		t.Fatalf("configure notice: %q", res.Notice)
	}
	if !strings.Contains(res.Notice, "MCP selections were reconciled") {
		t.Fatalf("mcp projection notice missing: %q", res.Notice)
	}
	after, _ := os.ReadFile(filepath.Join(existing, "AGENTS.md"))
	if string(after) != before {
		t.Fatal("Configure Apply silently changed AGENTS.md")
	}
	cfgView := screens.ConfigureView(screens.ConfigFormView{Draft: cfg, Width: 100})
	if !strings.Contains(cfgView, "Saves config.yaml") {
		t.Fatalf("configure subtitle:\n%s", cfgView)
	}
	if strings.Contains(cfgView, "Create docs/atlas/README.md") {
		t.Fatal("Configure must not show Project Docs Scaffold control")
	}
	pass("Configure after init: honesty + no hidden non-MCP rematerialization")

	// Status/Doctor read-only wording.
	homeBefore := mustWalkSlice28(t, homeDir)
	disc, err = inspect.Inspect(existing)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(disc)
	status := screens.StatusWithReport(disc, report)
	for _, want := range []string{"MCP", "selected"} {
		if !strings.Contains(status, want) {
			t.Fatalf("status missing %q:\n%s", want, status)
		}
	}
	docView := screens.Doctor(report, disc)
	if !strings.Contains(docView, "mcp") {
		t.Fatalf("doctor missing mcp section:\n%s", docView)
	}
	if !walkEqualSlice28(homeBefore, mustWalkSlice28(t, homeDir)) {
		t.Fatal("Status/Doctor mutated Atlas Home")
	}
	pass("Status/Doctor read-only + honest future states")

	// Init Project Setup contains docs scaffold control.
	setup := screens.InitPlan(screens.InitView{
		DraftName:     "demo",
		ModeConfirmed: "existing",
		DocsScaffold:  false,
		ActiveField:   screens.InitFieldDocsScaffold,
	})
	for _, want := range []string{
		"Project Docs Scaffold",
		"[ ] Create docs/atlas/README.md",
		"Optional developer-owned docs",
	} {
		if !strings.Contains(setup, want) {
			t.Fatalf("project setup missing %q:\n%s", want, setup)
		}
	}
	pass("Init Project Setup docs scaffold text")

	t.Log("ALL SLICE 28 DOGFOODING CHECKS PASSED")
}

func mustWalkSlice28(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if info.IsDir() {
			out[rel+"/"] = "dir"
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(b)
		return nil
	})
	return out
}

func walkEqualSlice28(a, b map[string]string) bool {
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
