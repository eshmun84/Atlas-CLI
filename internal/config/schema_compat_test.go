package config_test

import (
	"errors"
	"os"
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
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
	"gopkg.in/yaml.v3"
)

func TestCheckProjectSchema_LegacyZeroCompatible(t *testing.T) {
	if err := config.CheckProjectSchema(config.ProjectDocument{SchemaVersion: 0}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckProjectSchema_Current(t *testing.T) {
	if err := config.CheckProjectSchema(config.ProjectDocument{SchemaVersion: config.CurrentProjectSchemaVersion}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckProjectSchema_Newer(t *testing.T) {
	err := config.CheckProjectSchema(config.ProjectDocument{SchemaVersion: config.CurrentProjectSchemaVersion + 1})
	if err == nil || !errors.Is(err, config.ErrProjectSchemaNewer) {
		t.Fatalf("want newer, got %v", err)
	}
}

func TestRequireProjectSchemaCompatible_FreshAllowed(t *testing.T) {
	root := t.TempDir()
	if err := config.RequireProjectSchemaCompatible(root); err != nil {
		t.Fatal(err)
	}
}

func TestPersistConfigure_NewerProjectSchema_BlocksBeforeOpenSpec(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()

	// Seed a valid current project via Init, then bump schema.
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "schema-newer", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
	})
	_ = draft.SetValue("adapters.selected", "cursor")
	fixed := time.Date(2026, 10, 15, 22, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	before := listTree(t, root)

	cfgPath := filepath.Join(root, ".atlas", "config.yaml")
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc config.ProjectDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc.SchemaVersion = config.CurrentProjectSchemaVersion + 1
	out, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, out, 0o644); err != nil {
		t.Fatal(err)
	}
	beforePoison := listTree(t, root)

	runner := &configureOSRunner{fn: func([]string, string) (openspec.RunResult, error) {
		t.Fatal("OpenSpec runner must not run when project schema is incompatible")
		return openspec.RunResult{}, nil
	}}
	config.SetConfigureOpenSpecRunnerForTest(runner)
	t.Cleanup(func() { config.SetConfigureOpenSpecRunnerForTest(nil) })

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "schema-newer", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, OpenCodeDetected: true,
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = cfg.SetValue("adapters.selected", "cursor,opencode")
	_, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: config.EmptyMCPDraft()})
	if err == nil || !errors.Is(err, config.ErrProjectSchemaNewer) {
		t.Fatalf("expected schema newer block, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("OpenSpec invoked: %v", runner.calls)
	}
	after := listTree(t, root)
	if strings.Join(after, "\n") != strings.Join(beforePoison, "\n") {
		t.Fatalf("project mutated\nbefore=%v\nafter=%v\norig=%v", beforePoison, after, before)
	}
	raw2, _ := os.ReadFile(cfgPath)
	var doc2 config.ProjectDocument
	_ = yaml.Unmarshal(raw2, &doc2)
	if doc2.SchemaVersion != config.CurrentProjectSchemaVersion+1 {
		t.Fatalf("schema rewritten to %d", doc2.SchemaVersion)
	}
}

func TestApplyConfig_FreshProjectAllowed(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "schema-fresh", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
	})
	_ = draft.SetValue("adapters.selected", "cursor")
	res, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 15, 22, 5, 0, 0, time.UTC) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.ProjectID == "" {
		t.Fatal("expected project id")
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != config.CurrentProjectSchemaVersion {
		t.Fatalf("schema stamped = %d", doc.SchemaVersion)
	}
}

func TestApplyConfig_LegacyMissingSchemaMarker_Compatible(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	// Seed legacy config without schema_version, then Configure.
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := []byte(`project:
  name: schema-legacy
  mode: new
governance:
  workflow: sdd
  spec_engine: openspec
adapters:
  selected: [cursor]
source_control:
  mode: none
  default_remote: origin
  branch_strategy: manual
  governance_files: local_only
memory:
  strategy: sqlite
context:
  graph:
    enabled: false
mcp:
  builtins:
    jira:
      enabled: false
    context7:
      enabled: false
    chrome_devtools:
      enabled: false
  custom: []
`)
	if err := os.WriteFile(filepath.Join(root, ".atlas", "config.yaml"), legacy, 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if doc.SchemaVersion != 0 {
		t.Fatalf("expected legacy 0, got %d", doc.SchemaVersion)
	}
	if err := config.CheckProjectSchema(doc); err != nil {
		t.Fatal(err)
	}
	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "schema-legacy", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
	})
	_ = cfg.SetValue("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: config.EmptyMCPDraft()}); err != nil {
		t.Fatalf("legacy configure: %v", err)
	}
	doc2, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if doc2.SchemaVersion != config.CurrentProjectSchemaVersion {
		t.Fatalf("expected stamp to current on write, got %d", doc2.SchemaVersion)
	}
}

func TestApplyConfig_ProductVersionMismatchAlone_OK(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "schema-prov", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
	})
	_ = draft.SetValue("adapters.selected", "cursor")
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 15, 22, 10, 0, 0, time.UTC) },
	}); err != nil {
		t.Fatal(err)
	}
	// Rewrite state atlas_version only.
	statePath := filepath.Join(root, ".atlas", "state.yaml")
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var st config.StateDocument
	if err := yaml.Unmarshal(raw, &st); err != nil {
		t.Fatal(err)
	}
	st.AtlasVersion = "0.0.9"
	out, _ := yaml.Marshal(&st)
	_ = os.WriteFile(statePath, out, 0o644)

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "schema-prov", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
	})
	_ = cfg.SetValue("adapters.selected", "cursor")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: config.EmptyMCPDraft()}); err != nil {
		t.Fatalf("atlas_version mismatch alone must not block: %v", err)
	}
}

func TestRuntimeRepair_RefusesNewerProjectSchema(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "schema-repair", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
	})
	_ = draft.SetValue("adapters.selected", "cursor")
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 15, 22, 15, 0, 0, time.UTC) },
	}); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, ".atlas", "config.yaml")
	raw, _ := os.ReadFile(cfgPath)
	var doc config.ProjectDocument
	_ = yaml.Unmarshal(raw, &doc)
	doc.SchemaVersion = config.CurrentProjectSchemaVersion + 1
	out, _ := yaml.Marshal(&doc)
	_ = os.WriteFile(cfgPath, out, 0o644)
	before := listTree(t, root)

	files, err := project.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	atlas := project.EvaluateAtlasStatus(root, files)
	health := runtime.EvaluateHealth(root, atlas, files)
	plan := runtime.BuildRuntimeRepairPlan(root, health)
	_, err = runtime.ApplyRuntimeRepair(root, plan.Signature(), time.Now)
	if err == nil || !errors.Is(err, config.ErrProjectSchemaNewer) {
		t.Fatalf("expected repair refuse, got %v", err)
	}
	after := listTree(t, root)
	if strings.Join(after, "\n") != strings.Join(before, "\n") {
		t.Fatalf("repair mutated incompatible project")
	}
}

func TestDoctor_ProjectSchemaNewer_Manual(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "schema-doc", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
	})
	_ = draft.SetValue("adapters.selected", "cursor")
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 15, 22, 20, 0, 0, time.UTC) },
	}); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(root, ".atlas", "config.yaml")
	raw, _ := os.ReadFile(cfgPath)
	var doc config.ProjectDocument
	_ = yaml.Unmarshal(raw, &doc)
	doc.SchemaVersion = config.CurrentProjectSchemaVersion + 1
	out, _ := yaml.Marshal(&doc)
	_ = os.WriteFile(cfgPath, out, 0o644)

	result, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(result)
	found := false
	for _, c := range report.Checks {
		if c.Name == "atlas project schema" {
			found = true
			if c.Severity != doctor.SeverityFail {
				t.Fatalf("severity=%s", c.Severity)
			}
			if c.Repairability != doctor.RepairManual {
				t.Fatalf("repairability=%s want manual", c.Repairability)
			}
			if !strings.Contains(c.Message, "newer") {
				t.Fatalf("message=%q", c.Message)
			}
		}
	}
	if !found {
		t.Fatal("missing atlas project schema check")
	}
	// Read-only inspect must not rewrite schema.
	raw2, _ := os.ReadFile(cfgPath)
	var doc2 config.ProjectDocument
	_ = yaml.Unmarshal(raw2, &doc2)
	if doc2.SchemaVersion != config.CurrentProjectSchemaVersion+1 {
		t.Fatal("doctor rewrote schema")
	}
}

func TestDoctor_HomeSchemaNewer_Manual(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(homeDir, "state", "home.yaml")
	raw, _ := os.ReadFile(statePath)
	var doc home.StateDocument
	_ = yaml.Unmarshal(raw, &doc)
	doc.SchemaVersion = home.CurrentSchemaVersion + 1
	out, _ := yaml.Marshal(&doc)
	_ = os.WriteFile(statePath, out, 0o600)

	root := t.TempDir()
	result, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(result)
	found := false
	for _, c := range report.Checks {
		if c.Name == "atlas home schema" {
			found = true
			if c.Severity != doctor.SeverityFail || c.Repairability != doctor.RepairManual {
				t.Fatalf("check=%#v", c)
			}
		}
	}
	if !found {
		t.Fatal("missing home schema check")
	}
}
