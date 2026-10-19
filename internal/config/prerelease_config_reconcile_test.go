package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestRequireSupportedAdapters(t *testing.T) {
	t.Parallel()
	if err := config.RequireSupportedAdapters(nil); err == nil || !strings.Contains(err.Error(), config.ErrAdaptersRequired) {
		t.Fatalf("nil: %v", err)
	}
	if err := config.RequireSupportedAdapters([]string{}); err == nil {
		t.Fatal("empty")
	}
	if err := config.RequireSupportedAdapters([]string{"cursor"}); err != nil {
		t.Fatal(err)
	}
	if err := config.RequireSupportedAdapters([]string{"opencode"}); err != nil {
		t.Fatal(err)
	}
	if err := config.RequireSupportedAdapters([]string{"cursor", "opencode"}); err != nil {
		t.Fatal(err)
	}

	unavailable := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo", ProjectMode: "new",
		ToolCursorAvailable: false, ToolOpenCodeAvailable: false,
		CursorDetected: true,
	})
	if config.DraftHasSupportedAdapter(unavailable) {
		t.Fatal("selected-but-unavailable must not satisfy agent invariant")
	}
	if err := config.RequireSupportedAdaptersFromDraft(unavailable); err == nil {
		t.Fatal("mutation must reject unavailable-only selection")
	}
	available := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo", ProjectMode: "new",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: false,
	})
	_ = available.ToggleMulti("adapters.selected", "cursor")
	if !config.DraftHasSupportedAdapter(available) {
		t.Fatal("available cursor must satisfy invariant")
	}
}

func TestValidateProjectDocument_AllowsLegacyZeroAdapters(t *testing.T) {
	t.Parallel()
	doc := config.BuildProjectDocument(config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "legacy", ProjectMode: "existing",
	}), config.EmptyMCPDraft())
	doc.Adapters.Selected = nil
	if err := config.ValidateProjectDocument(doc); err != nil {
		t.Fatalf("load/inspect must allow zero adapters: %v", err)
	}
	if err := config.RequireSupportedAdapters(doc.Adapters.Selected); err == nil {
		t.Fatal("mutation must reject zero adapters")
	}
}

func TestDraft_EvidenceRequiredNotUserEditable(t *testing.T) {
	t.Parallel()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo", ProjectMode: "new",
	})
	if _, ok := draft.FieldByKey("governance.evidence_required"); ok {
		t.Fatal("evidence_required must not be a draft field")
	}
	doc := config.BuildProjectDocument(draft, config.EmptyMCPDraft())
	if !doc.Governance.EvidenceRequired {
		t.Fatal("rebuild must normalize evidence_required=true")
	}
}

func TestEvidenceRequired_LegacyFalseNormalizesOnRebuild(t *testing.T) {
	t.Parallel()
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "demo", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	doc := config.BuildProjectDocument(draft, config.EmptyMCPDraft())
	doc.Governance.EvidenceRequired = false
	// Rebuild from draft (no evidence field) always forces true.
	again := config.BuildProjectDocument(draft, config.EmptyMCPDraft())
	if !again.Governance.EvidenceRequired {
		t.Fatal("evidence must stay on")
	}
	_ = doc
}

func TestGovernanceStorage_ProviderNeutralGitLocal(t *testing.T) {
	t.Parallel()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo", ProjectMode: "new", GitRepoDetected: true,
	})
	if !draft.SelectOption("source_control.mode", "git_local") {
		t.Fatal("git_local")
	}
	if !draft.SelectOption("source_control.governance_storage", "versioned") {
		t.Fatal("versioned must work with git_local")
	}
	doc := config.BuildProjectDocument(draft, config.EmptyMCPDraft())
	if doc.SourceControl.Mode != config.SourceControlGitLocal || doc.SourceControl.GovernanceFiles != config.GovernanceFilesVersioned {
		t.Fatalf("doc = %#v", doc.SourceControl)
	}
}

func TestGitignore_LocalOnlyAndVersioned(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("bin/\nuser-keep/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := config.ProjectDocument{
		Adapters:      config.AdaptersPersist{Selected: []string{"cursor", "opencode"}},
		SourceControl: config.SourceControlPersist{GovernanceFiles: config.GovernanceFilesLocalOnly},
	}
	plan, err := config.PlanGovernanceGitignore(root, doc)
	if err != nil || !plan.Changed {
		t.Fatalf("local_only plan: %#v err=%v", plan, err)
	}
	if !strings.Contains(plan.Content, "bin/") || !strings.Contains(plan.Content, "user-keep/") {
		t.Fatalf("user gitignore must be preserved: %s", plan.Content)
	}
	for _, want := range []string{
		".atlas/", "AGENTS.md", ".cursor/", ".opencode/atlas.md", ".opencode/agents/atlas-*.md",
		".opencode/plugins/atlas-hooks/", ".opencode/opencode.json",
	} {
		if !strings.Contains(plan.Content, want) {
			t.Fatalf("managed block missing %q: %s", want, plan.Content)
		}
	}
	if strings.Contains(plan.Content, "\n.opencode/\n") {
		t.Fatalf("must not blanket-ignore .opencode/: %s", plan.Content)
	}
	for _, line := range strings.Split(plan.Content, "\n") {
		if strings.TrimSpace(line) == "opencode.json" {
			t.Fatalf("must not ignore root opencode.json: %s", plan.Content)
		}
	}

	homePath := os.Getenv("ATLAS_HOME")
	res, _, _, err := config.ReconcileGovernanceGitignore(root, homePath, "proj", doc, time.Now().UTC())
	if err != nil || !res.Changed {
		t.Fatalf("reconcile local_only: %#v %v", res, err)
	}

	doc.SourceControl.GovernanceFiles = config.GovernanceFilesVersioned
	plan2, err := config.PlanGovernanceGitignore(root, doc)
	if err != nil || !plan2.Changed {
		t.Fatalf("versioned cleanup plan: %#v err=%v", plan2, err)
	}
	if strings.Contains(plan2.Content, "# >>> Atlas local governance >>>") {
		t.Fatal("versioned must strip Atlas managed block")
	}
	if !strings.Contains(plan2.Content, "bin/") || !strings.Contains(plan2.Content, "user-keep/") {
		t.Fatalf("user content must remain: %s", plan2.Content)
	}
}

func TestManagedMCPCatalog_OnlyContext7AndChrome(t *testing.T) {
	t.Parallel()
	draft := config.DefaultMCPDraft()
	if len(draft.Builtins) != 2 {
		t.Fatalf("builtins=%d", len(draft.Builtins))
	}
	ids := []config.MCPBuiltinID{draft.Builtins[0].ID, draft.Builtins[1].ID}
	if ids[0] != config.MCPBuiltinContext7 || ids[1] != config.MCPBuiltinChromeDevTools {
		t.Fatalf("ids=%v", ids)
	}
	for _, banned := range []config.MCPBuiltinID{
		config.MCPBuiltinFilesystem, config.MCPBuiltinGitHub, config.MCPBuiltinJira,
	} {
		if draft.BuiltinIndex(banned) >= 0 {
			t.Fatalf("retired builtin %s still present", banned)
		}
	}
}

func TestAdapterSwap_CursorToOpenCodePreservesUnknown(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()

	// Init-like configure with Cursor + both managed MCPs + local_only.
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "swap", ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.SelectOption("source_control.mode", "git_local")
	_ = draft.SelectOption("source_control.governance_storage", "local_only")
	mcp := config.EmptyMCPDraft()
	_ = mcp.EnableBuiltin(config.MCPBuiltinContext7)
	_ = mcp.EnableBuiltin(config.MCPBuiltinChromeDevTools)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	// Seed unknown user-owned Cursor content.
	userRule := filepath.Join(root, ".cursor", "rules", "mine.mdc")
	if err := os.MkdirAll(filepath.Dir(userRule), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userRule, []byte("user owned\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Swap to OpenCode only.
	next := draft
	_ = next.ToggleMulti("adapters.selected", "cursor")
	_ = next.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: next, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor", "rules", "atlas.mdc")); !os.IsNotExist(err) {
		t.Fatal("Atlas cursor rule must be removed")
	}
	got, err := os.ReadFile(userRule)
	if err != nil || string(got) != "user owned\n" {
		t.Fatalf("unknown cursor content must be preserved: %v %q", err, got)
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "atlas.md")); err != nil {
		t.Fatal("opencode atlas projection missing")
	}
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(agents), "ATLAS:ADAPTER:CURSOR") {
		t.Fatal("cursor adapter block must leave AGENTS.md")
	}
	if !strings.Contains(string(agents), "ATLAS:ADAPTER:OPENCODE") {
		t.Fatal("opencode adapter block missing")
	}
	ocMCP, err := os.ReadFile(filepath.Join(root, ".opencode", "opencode.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ocMCP), "context7") || !strings.Contains(string(ocMCP), "chrome_devtools") {
		t.Fatalf("managed MCP not projected to opencode: %s", ocMCP)
	}
}

func TestAdapterAdd_MultiKeepsCursor(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "multi", ProjectMode: "existing",
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp := config.EmptyMCPDraft()
	_ = mcp.EnableBuiltin(config.MCPBuiltinContext7)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	cursorBefore, _ := os.ReadFile(filepath.Join(root, ".cursor", "rules", "atlas.mdc"))
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatal(err)
	}
	cursorAfter, err := os.ReadFile(filepath.Join(root, ".cursor", "rules", "atlas.mdc"))
	if err != nil {
		t.Fatal(err)
	}
	if string(cursorBefore) != string(cursorAfter) {
		t.Fatal("cursor atlas rule should remain when adding opencode")
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode", "atlas.md")); err != nil {
		t.Fatal("opencode projection missing")
	}
}
