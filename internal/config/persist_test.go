package config_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestBuildProjectDocument_FromDraftAndMCP(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      "Atlas-CLI",
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: true,
	})
	if !draft.SelectOption("source_control.mode", "git_github") {
		t.Fatal("source control")
	}
	// Branch strategy is not an Init setup decision; persist defaults to manual.
	mcp := config.EmptyMCPDraft()
	if !mcp.ToggleBuiltin(0) {
		t.Fatal("jira")
	}
	if !mcp.ToggleBuiltin(2) {
		t.Fatal("chrome")
	}
	if _, err := mcp.AddCustom("Internal Docs", config.MCPTransportHTTP, "https://example.local/mcp", "", ""); err != nil {
		t.Fatalf("add custom: %v", err)
	}
	mcp.ToggleCustom(0)

	doc := config.BuildProjectDocument(draft, mcp)
	if err := config.ValidateProjectDocument(doc); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if doc.Project.Name != "Atlas-CLI" || doc.Project.Mode != "new" {
		t.Fatalf("project = %#v", doc.Project)
	}
	if doc.Governance.Workflow != "sdd" || doc.Governance.SpecEngine != "openspec" {
		t.Fatalf("governance = %#v", doc.Governance)
	}
	if !doc.Governance.TestingRequired || !doc.Governance.ReviewRequired || !doc.Governance.EvidenceRequired {
		t.Fatalf("governance flags = %#v", doc.Governance)
	}
	if len(doc.Adapters.Selected) != 2 {
		t.Fatalf("adapters = %#v", doc.Adapters.Selected)
	}
	if doc.SourceControl.Mode != "git_github" || doc.SourceControl.DefaultRemote != "origin" {
		t.Fatalf("source control = %#v", doc.SourceControl)
	}
	if doc.SourceControl.BranchStrategy != "manual" || doc.SourceControl.GovernanceFiles != "local_only" {
		t.Fatalf("source control policy = %#v", doc.SourceControl)
	}
	if doc.SourceControl.DeliveryAssist {
		t.Fatal("delivery assist should default false")
	}
	if doc.Memory.Strategy != "sqlite_plus_context_capsule" {
		t.Fatalf("memory = %q", doc.Memory.Strategy)
	}
	if !doc.ContextGraphEnabled() {
		t.Fatalf("context graph default enabled = %#v", doc.Context)
	}
	if !doc.MCP.Builtins.Jira.Enabled || doc.MCP.Builtins.Context7.Enabled || !doc.MCP.Builtins.ChromeDevTools.Enabled {
		t.Fatalf("builtins = %#v", doc.MCP.Builtins)
	}
	if len(doc.MCP.Custom) != 1 || doc.MCP.Custom[0].Name != "Internal Docs" || doc.MCP.Custom[0].Transport != "http" {
		t.Fatalf("custom = %#v", doc.MCP.Custom)
	}
	if !doc.MCP.Custom[0].Enabled {
		t.Fatal("custom should be enabled")
	}

	cfg := doc.ToConfig()
	if err := config.Validate(cfg); err != nil {
		t.Fatalf("mapped config invalid: %v", err)
	}
	if cfg.Project.Mode != config.ModeGreenfield {
		t.Fatalf("mapped mode = %q", cfg.Project.Mode)
	}
	if !cfg.Adapters.Cursor || !cfg.Adapters.OpenCode {
		t.Fatalf("mapped adapters = %#v", cfg.Adapters)
	}
	if !cfg.ExternalContextProviders.MCPEnabled || !cfg.ExternalContextProviders.JiraEnabled {
		t.Fatalf("mapped mcp flags = %#v", cfg.ExternalContextProviders)
	}
}

func TestBuildLocalStateAndLockDocuments(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:         "demo",
		ProjectMode:         "existing",
		DefaultRemote:       "upstream",
		ToolCursorAvailable: true,
	})
	local := config.BuildLocalDocument(draft)
	if local.SchemaVersion != 1 || local.CredentialsStored || local.SourceControl.DefaultRemote != "upstream" {
		t.Fatalf("local = %#v", local)
	}

	state := config.BuildStateDocument(draft, time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC).Format(time.RFC3339), false)
	if !state.Initialized || state.RuntimeMaterialized || state.RuntimeMaterializedAt != "" || state.ProjectName != "demo" {
		t.Fatalf("state = %#v", state)
	}
	stateRuntime := config.BuildStateDocument(draft, state.AppliedAt, true)
	if !stateRuntime.RuntimeMaterialized || stateRuntime.RuntimeMaterializedAt != state.AppliedAt {
		t.Fatalf("runtime state = %#v", stateRuntime)
	}

	lock := config.BuildAssetsLockDocument(nil)
	if lock.SchemaVersion != 1 || lock.Assets == nil || len(lock.Assets) == 0 {
		t.Fatalf("lock = %#v", lock)
	}
	var sawRegistry bool
	for _, entry := range lock.Assets {
		if entry.ID == "project/agent-registry.md" {
			sawRegistry = true
		}
		if entry.Source == "" || entry.Checksum == "" {
			t.Fatalf("incomplete lock entry %#v", entry)
		}
	}
	if !sawRegistry {
		t.Fatalf("registry entry missing: %#v", lock.Assets)
	}
	lockCursor := config.BuildAssetsLockDocument([]string{"cursor"})
	var sawCursorAgent bool
	for _, entry := range lockCursor.Assets {
		if entry.ID == "agents/runtime/atlas-orchestrator.md" {
			sawCursorAgent = true
			if len(entry.ProjectPaths) != 1 || entry.ProjectPaths[0] != ".cursor/agents/atlas-orchestrator.md" {
				t.Fatalf("cursor orchestrator paths = %#v", entry.ProjectPaths)
			}
		}
	}
	if !sawCursorAgent {
		t.Fatalf("cursor orchestrator missing from lock: %#v", lockCursor.Assets)
	}

	if !draft.ToggleMulti("adapters.selected", "cursor") {
		t.Fatal("toggle cursor")
	}
	doc := config.BuildProjectDocument(draft, config.EmptyMCPDraft())
	lockSDD := config.BuildAssetsLockDocumentFor("/tmp/atlas-home", doc, "0.1.0")
	if lockSDD.HomePath != "" {
		t.Fatalf("portable assets.lock must not embed absolute HomePath: %q", lockSDD.HomePath)
	}
	var sawContract bool
	for _, entry := range lockSDD.Assets {
		if entry.ID == config.EmbedPathSDDOpenSpecContract {
			sawContract = true
			if entry.Family != "contracts" || entry.Checksum == "" || entry.HomePath == "" {
				t.Fatalf("contract lock entry incomplete: %#v", entry)
			}
			if filepath.IsAbs(entry.HomePath) {
				t.Fatalf("entry HomePath must be Home-relative: %q", entry.HomePath)
			}
			if len(entry.ProjectPaths) != 1 || entry.ProjectPaths[0] != config.FileSDDOpenSpecContract {
				t.Fatalf("contract project paths = %#v", entry.ProjectPaths)
			}
		}
	}
	if !sawContract {
		t.Fatalf("contract missing from lock: %#v", lockSDD.Assets)
	}
	if !config.DependsOnSDDOpenSpecContract(doc) {
		t.Fatal("expected SDD/OpenSpec dependency")
	}
	body, err := config.RenderSDDOpenSpecContract()
	if err != nil || !strings.Contains(body, "Operational Contract") {
		t.Fatalf("RenderSDDOpenSpecContract = %q err=%v", body, err)
	}
}

func TestContextGraphEnabled_DefaultsTrueWhenMissing(t *testing.T) {
	t.Parallel()
	doc := config.ProjectDocument{}
	if !doc.ContextGraphEnabled() {
		t.Fatal("missing context.graph.enabled must default to true")
	}
	off := false
	doc.Context.Graph.Enabled = &off
	if doc.ContextGraphEnabled() {
		t.Fatal("explicit false must stay false")
	}
}

func TestValidateProjectDocument_RejectsEmptyName(t *testing.T) {
	t.Parallel()

	doc := config.BuildProjectDocument(config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "new",
	}), config.EmptyMCPDraft())
	doc.Project.Name = ""
	err := config.ValidateProjectDocument(doc)
	if err == nil || !strings.Contains(err.Error(), "project.name") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateProjectDocument_AdaptersStrictCanonical(t *testing.T) {
	t.Parallel()

	base := func() config.ProjectDocument {
		return config.BuildProjectDocument(config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
			ProjectName: "demo",
			ProjectMode: "new",
		}), config.EmptyMCPDraft())
	}

	ok := base()
	ok.Adapters.Selected = []string{"cursor", "opencode"}
	if err := config.ValidateProjectDocument(ok); err != nil {
		t.Fatalf("canonical adapters rejected: %v", err)
	}

	cases := []struct {
		name     string
		selected []string
		want     string
	}{
		{name: "unknown", selected: []string{"claude"}, want: "invalid"},
		{name: "non-canonical casing", selected: []string{"Cursor"}, want: "invalid"},
		{name: "OpenCode casing", selected: []string{"OpenCode"}, want: "invalid"},
		{name: "leading space", selected: []string{" cursor"}, want: "leading or trailing spaces"},
		{name: "trailing space", selected: []string{"opencode "}, want: "leading or trailing spaces"},
		{name: "duplicate", selected: []string{"cursor", "cursor"}, want: "duplicated"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := base()
			doc.Adapters.Selected = tc.selected
			err := config.ValidateProjectDocument(doc)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestPersistProjectMode(t *testing.T) {
	t.Parallel()
	if config.PersistProjectMode("new") != "new" || config.PersistProjectMode(config.ModeGreenfield) != "new" {
		t.Fatal("new")
	}
	if config.PersistProjectMode("existing") != "existing" {
		t.Fatal("existing")
	}
}

func TestProjectDocument_ToMCPDraftAndApply(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "new",
	})
	mcp := config.EmptyMCPDraft()
	mcp.ToggleBuiltin(0)
	mcp.ToggleBuiltin(1)
	if _, err := mcp.AddCustom("Docs", config.MCPTransportHTTP, "https://example.local", "", ""); err != nil {
		t.Fatal(err)
	}
	mcp.ToggleCustom(0)
	doc := config.BuildProjectDocument(draft, mcp)

	loaded := doc.ToMCPDraft()
	if !loaded.Builtins[0].Enabled || !loaded.Builtins[1].Enabled || loaded.Builtins[2].Enabled {
		t.Fatalf("builtins = %#v", loaded.Builtins)
	}
	if len(loaded.CustomServers) != 1 || loaded.CustomServers[0].Name != "Docs" || loaded.CustomServers[0].Transport != config.MCPTransportHTTP {
		t.Fatalf("custom = %#v", loaded.CustomServers)
	}

	cfgDraft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "placeholder",
		ProjectMode: "existing",
	})
	config.ApplyProjectDocument(&cfgDraft, doc)
	if cfgDraft.ProjectName() != "demo" {
		t.Fatalf("name = %q", cfgDraft.ProjectName())
	}
	if field, _ := cfgDraft.FieldByKey("governance.spec_engine"); field.Value != "openspec" {
		t.Fatalf("spec engine = %q", field.Value)
	}
}
