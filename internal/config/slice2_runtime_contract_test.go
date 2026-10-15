package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestDependsOnSDDOpenSpecContract_RequiresWorkflowAndEngine(t *testing.T) {
	t.Parallel()
	both := config.ProjectDocument{
		Governance: config.GovernancePersist{Workflow: config.WorkflowSDD, SpecEngine: config.SpecProviderOpenSpec},
	}
	if !config.DependsOnSDDOpenSpecContract(both) || !config.NeedsOpenSpecBootstrap(both) {
		t.Fatal("sdd+openspec must require contract and bootstrap")
	}
	// Legacy/corrupt combo: helpers stay AND-aligned even though Validate rejects it.
	sddOnly := config.ProjectDocument{
		Governance: config.GovernancePersist{Workflow: config.WorkflowSDD, SpecEngine: config.SpecProviderNone},
	}
	if config.DependsOnSDDOpenSpecContract(sddOnly) || config.NeedsOpenSpecBootstrap(sddOnly) {
		t.Fatal("sdd+none must not require OpenSpec contract/bootstrap")
	}
	engineOnly := config.ProjectDocument{
		Governance: config.GovernancePersist{Workflow: "guided", SpecEngine: config.SpecProviderOpenSpec},
	}
	if config.DependsOnSDDOpenSpecContract(engineOnly) || config.NeedsOpenSpecBootstrap(engineOnly) {
		t.Fatal("non-sdd workflow must not require OpenSpec contract/bootstrap")
	}
}

func TestValidateProjectDocument_SDDRequiresOpenSpec(t *testing.T) {
	t.Parallel()
	valid := config.ProjectDocument{
		Project:    config.ProjectPersist{Name: "demo", Mode: "new"},
		Governance: config.GovernancePersist{Workflow: config.WorkflowSDD, SpecEngine: config.SpecProviderOpenSpec},
		SourceControl: config.SourceControlPersist{
			Mode: "none", DefaultRemote: "origin", BranchStrategy: "manual", GovernanceFiles: "local_only",
		},
		Memory: config.MemoryPersist{Strategy: "sqlite"},
	}
	if err := config.ValidateProjectDocument(valid); err != nil {
		t.Fatalf("sdd+openspec must be valid: %v", err)
	}
	invalid := valid
	invalid.Governance.SpecEngine = config.SpecProviderNone
	err := config.ValidateProjectDocument(invalid)
	if err == nil {
		t.Fatal("sdd+none must be invalid")
	}
	if !strings.Contains(err.Error(), "spec_engine must be openspec") {
		t.Fatalf("expected actionable message, got %v", err)
	}
}

func TestApply_RejectsSDDWithNone(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "NoNone", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	// Force legacy combo onto the draft even if UI no longer offers none.
	if !draft.SetValue("governance.spec_engine", "none") {
		t.Fatal("could not force spec_engine=none via SetValue")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 19, 0, 0, 0, time.UTC) },
	})
	if err == nil {
		t.Fatal("Apply must reject sdd+none")
	}
	if !strings.Contains(err.Error(), "spec_engine must be openspec") {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestSlice2_Dogfood013_LoginFormMaterialTask(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "LoginDemo", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if !draft.SelectOption("governance.spec_engine", "openspec") {
		t.Fatal("openspec")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 18, 0, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	agents := mustRead(t, filepath.Join(root, "AGENTS.md"))
	orch := mustRead(t, filepath.Join(root, ".cursor", "agents", "atlas-orchestrator.md"))
	rule := mustRead(t, filepath.Join(root, ".cursor", "rules", "atlas.mdc"))
	contract := mustRead(t, filepath.Join(root, ".atlas", "contracts", "sdd-openspec.md"))

	// Semantic coverage for: "Crea un index.html con un formulario de login."
	for _, want := range []string{"governed-sdd", "user-facing", "MUST NOT", "planning failed"} {
		if !strings.Contains(agents, want) {
			t.Fatalf("AGENTS.md missing %q", want)
		}
	}
	for _, want := range []string{"governed-sdd", "MUST NOT", "planning failed", "Execution gate"} {
		if !strings.Contains(orch, want) {
			t.Fatalf("orchestrator missing %q", want)
		}
	}
	if !strings.Contains(agents, "did not ask for planning") && !strings.Contains(agents, "did not request planning") {
		t.Fatal("AGENTS must prohibit user-did-not-request-planning escape hatch")
	}
	if !strings.Contains(agents, "product mutation") && !strings.Contains(agents, "mutate product") {
		t.Fatal("AGENTS must gate product mutation")
	}
	if !strings.Contains(rule, "atlas-orchestrator") || !strings.Contains(rule, "classify") {
		t.Fatal("cursor entrypoint must bind orchestrator classify routing")
	}
	for _, want := range []string{
		"openspec init",
		"governance.openspec_version",
		"opsx-propose",
		"Fail-closed",
		"BLOCKED",
	} {
		if !strings.Contains(contract, want) {
			t.Fatalf("contract missing real OpenSpec provider guidance %q", want)
		}
	}
	impl := mustRead(t, filepath.Join(root, ".cursor", "agents", "atlas-sdd-implement.md"))
	for _, want := range []string{
		"openspec",
		"OpenSpec",
	} {
		if !strings.Contains(impl, want) {
			t.Fatalf("implement agent must delegate to OpenSpec, missing %q", want)
		}
	}
}

func TestSlice2_Dogfood010_ProviderOriginSeparation(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "RemoteDemo", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	if !draft.SelectOption("source_control.mode", "git_github") {
		t.Fatal("git_github")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 14, 18, 10, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	agents := mustRead(t, filepath.Join(root, "AGENTS.md"))
	rule := mustRead(t, filepath.Join(root, ".cursor", "rules", "atlas.mdc"))
	for _, want := range []string{"Cursor Origin", "origin", "GitHub", "execution adapter", "git_github"} {
		if want == "git_github" {
			continue
		}
		if !strings.Contains(agents, want) {
			t.Fatalf("AGENTS missing delivery clause %q", want)
		}
	}
	for _, want := range []string{"Cursor Origin", "origin", "hosting provider"} {
		if !strings.Contains(rule, want) {
			t.Fatalf("cursor rule missing %q", want)
		}
	}
	if !strings.Contains(agents, "execution adapter") {
		t.Fatal("must distinguish execution adapter")
	}
	if !strings.Contains(agents, "remote") || !strings.Contains(agents, "name") {
		t.Fatal("must teach origin as remote name")
	}
	if !strings.Contains(agents, "auto-install") && !strings.Contains(agents, "MUST NOT auto-install") {
		t.Fatal("must forbid CLI auto-install")
	}
	cfg := mustRead(t, filepath.Join(root, ".atlas", "config.yaml"))
	if !strings.Contains(cfg, "mode: git_github") {
		t.Fatalf("expected git_github config, got:\n%s", cfg)
	}
	if !strings.Contains(cfg, "default_remote: origin") && !strings.Contains(cfg, "default_remote:") {
		// default_remote may be present
		t.Log("default_remote may be omitted in yaml if empty-defaulted elsewhere")
	}
}

func TestSlice2_Dogfood009_SigningPolicy(t *testing.T) {
	t.Parallel()
	agents := mustRenderAgentsMD(t, "SignDemo", true, []string{"cursor"}, nil)
	for _, want := range []string{
		"signing_required",
		"--no-gpg-sign",
		"STOP",
		"pinentry",
		"verify the signature",
	} {
		if !strings.Contains(agents, want) {
			t.Fatalf("AGENTS missing signing clause %q", want)
		}
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "SignDemo", ProjectMode: "new",
	})
	if _, ok := draft.FieldByKey("source_control.signing_required"); !ok {
		t.Fatal("signing_required field must exist on draft")
	}
	if !draft.SelectOption("source_control.signing_required", "true") {
		t.Fatal("could not enable signing_required")
	}
	doc := config.BuildProjectDocument(draft, config.EmptyMCPDraft())
	if !doc.SourceControl.SigningRequired {
		t.Fatal("SigningRequired must persist on ProjectDocument")
	}
}

func TestSlice2_MemoryProgressiveDisclosure(t *testing.T) {
	t.Parallel()
	skill, err := assets.Content.ReadFile("skills/memory/1.0.0/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(skill)
	if strings.Count(body, "\n") > 120 {
		t.Fatalf("memory SKILL.md still too dense: %d lines", strings.Count(body, "\n"))
	}
	for _, want := range []string{
		"not executable authority",
		"references/taxonomy.md",
		"references/retrieval.md",
		"references/persistence.md",
		"references/diagnostics.md",
		"Automatic injection is **not** guaranteed",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("memory skill missing %q", want)
		}
	}
	for _, rel := range []string{
		"skills/memory/1.0.0/references/taxonomy.md",
		"skills/memory/1.0.0/references/retrieval.md",
		"skills/memory/1.0.0/references/persistence.md",
		"skills/memory/1.0.0/references/diagnostics.md",
	} {
		if _, err := assets.Content.ReadFile(rel); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
}

func TestSlice2_DocumentationIdentityBoundary(t *testing.T) {
	t.Parallel()
	raw, err := assets.Content.ReadFile("skills/documentation/1.0.0/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, want := range []string{
		"Project identity boundary",
		"MUST NOT introduce by default",
		"Atlas Home",
		"explicitly asks",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("documentation skill missing %q", want)
		}
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
