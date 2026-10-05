package config_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestBuildConfigDraft_VisibleSectionsAndDefaults(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      "Atlas-CLI",
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: false,
	})

	selector := draft.SelectorSections()
	wantSections := []string{"governance", "adapters", "source_control", "memory", "context", "mcp"}
	if len(selector) != len(wantSections) {
		t.Fatalf("selector sections = %d, want %d", len(selector), len(wantSections))
	}
	for i, key := range wantSections {
		if selector[i].Key != key {
			t.Fatalf("section[%d] = %q, want %q", i, selector[i].Key, key)
		}
	}
	for _, banned := range []string{"stack", "runtime", "skills", "technologies"} {
		for _, section := range selector {
			if section.Key == banned {
				t.Fatalf("banned section %q is visible", banned)
			}
		}
	}

	assertField(t, draft, "project.name", "Atlas-CLI", config.FieldReadonly, config.FieldLocked)
	assertField(t, draft, "governance.default_workflow", "sdd", config.FieldEditable, config.FieldEditable)
	assertField(t, draft, "governance.spec_engine", "openspec", config.FieldEditable, config.FieldEditable)
	assertField(t, draft, "governance.testing_required", "true", config.FieldEditable, config.FieldEditable)
	assertField(t, draft, "adapters.selected", "cursor", config.FieldEditable, config.FieldEditable)
	assertField(t, draft, "source_control.mode", "none", config.FieldEditable, config.FieldEditable)
	assertField(t, draft, "source_control.governance_storage", "local_only", config.FieldEditable, config.FieldEditable)
	assertField(t, draft, "memory.strategy", "sqlite_plus_context_capsule", config.FieldEditable, config.FieldEditable)
	assertField(t, draft, "context.graph.enabled", "true", config.FieldEditable, config.FieldEditable)

	if _, ok := draft.FieldByKey("governance.enabled"); ok {
		t.Fatal("governance.enabled must not exist")
	}
	if _, ok := draft.FieldByKey("stack.languages"); ok {
		t.Fatal("stack fields must not exist")
	}

	workflow, _ := draft.FieldByKey("governance.default_workflow")
	if len(workflow.Options) != 1 || workflow.Options[0] != "sdd" {
		t.Fatalf("workflow options = %#v", workflow.Options)
	}
	for _, opt := range workflow.VisibleOptions() {
		if opt.Label == "Governed basic" || opt.Label == "Review only" || opt.Label == "ODD" {
			t.Fatalf("unexpected workflow option %q", opt.Label)
		}
	}

	engine, _ := draft.FieldByKey("governance.spec_engine")
	labels := []string{engine.VisibleOptions()[0].Label, engine.VisibleOptions()[1].Label}
	if labels[0] != "OpenSpec" || labels[1] != "None" {
		t.Fatalf("spec engine labels = %#v", labels)
	}
	for _, opt := range engine.Options {
		if opt == "custom" {
			t.Fatal("custom spec engine must not exist")
		}
	}

	branch, _ := draft.FieldByKey("source_control.branch_strategy")
	for _, opt := range branch.Options {
		if opt == "custom" {
			t.Fatal("custom branch strategy must not exist")
		}
	}
}

func TestConfigDraft_Edits(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})

	if !draft.SelectOption("governance.spec_engine", "none") {
		t.Fatal("expected spec engine change")
	}
	if !draft.ToggleMulti("adapters.selected", "opencode") {
		t.Fatal("expected adapter toggle")
	}
	if !draft.SelectOption("memory.strategy", "sqlite") {
		t.Fatal("expected memory strategy change")
	}
	if !draft.SelectOption("source_control.governance_storage", "versioned") {
		t.Fatal("expected governance files change")
	}

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})
	if cfg.SetValue("project.name", "other") {
		t.Fatal("project.name must stay locked in configure")
	}
	if !cfg.SelectOption("memory.strategy", "context_capsule") {
		t.Fatal("memory.strategy should be editable in configure")
	}
	cfgSelector := cfg.SelectorSections()
	foundMCP, foundContext := false, false
	for _, section := range cfgSelector {
		if section.Key == "mcp" {
			foundMCP = true
		}
		if section.Key == "context" {
			foundContext = true
		}
	}
	if !foundMCP {
		t.Fatal("configure must show MCP section")
	}
	if !foundContext {
		t.Fatal("configure must show Context section")
	}
	if !cfg.SetValue("context.graph.enabled", "false") {
		t.Fatal("context.graph.enabled should be editable in configure")
	}
}

func assertField(t *testing.T, draft config.ConfigDraft, key, value string, initMut, confMut config.FieldMutability) {
	t.Helper()
	field, ok := draft.FieldByKey(key)
	if !ok {
		t.Fatalf("missing field %q", key)
	}
	if field.Value != value {
		t.Fatalf("%s value = %q, want %q", key, field.Value, value)
	}
	if field.InitMutability != initMut {
		t.Fatalf("%s init mutability = %q, want %q", key, field.InitMutability, initMut)
	}
	if field.ConfigureMutability != confMut {
		t.Fatalf("%s configure mutability = %q, want %q", key, field.ConfigureMutability, confMut)
	}
}
