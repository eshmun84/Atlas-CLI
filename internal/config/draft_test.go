package config_test

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestBuildConfigDraft_VisibleSectionsAndDefaults(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:           "Atlas-CLI",
		ProjectMode:           "new",
		DefaultRemote:         "origin",
		ToolCursorAvailable:   true,
		ToolOpenCodeAvailable: false,
	})

	selector := draft.SelectorSections()
	wantSections := []string{"governance", "adapters", "source_control", "mcp"}
	if len(selector) != len(wantSections) {
		t.Fatalf("selector sections = %d, want %d", len(selector), len(wantSections))
	}
	for i, key := range wantSections {
		if selector[i].Key != key {
			t.Fatalf("section[%d] = %q, want %q", i, selector[i].Key, key)
		}
	}
	if selector[2].Title != "Delivery" {
		t.Fatalf("source_control title = %q, want Delivery", selector[2].Title)
	}
	for _, banned := range []string{"stack", "runtime", "skills", "technologies", "compat", "memory", "context"} {
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
	assertField(t, draft, "adapters.selected", "", config.FieldEditable, config.FieldEditable) // no seed without detection
	assertField(t, draft, "source_control.mode", "none", config.FieldEditable, config.FieldEditable)
	assertField(t, draft, "source_control.governance_storage", "local_only", config.FieldEditable, config.FieldEditable)
	assertField(t, draft, "memory.strategy", "sqlite_plus_context_capsule", config.FieldReadonly, config.FieldReadonly)
	assertField(t, draft, "context.graph.enabled", "true", config.FieldReadonly, config.FieldReadonly)

	adapters, _ := draft.FieldByKey("adapters.selected")
	if !adapters.OptionDisabled("opencode") {
		t.Fatal("opencode should be disabled when tool unavailable")
	}
	if adapters.OptionDisabled("cursor") {
		t.Fatal("cursor should be available when tool is present")
	}
	opts := adapters.VisibleOptions()
	if len(opts) != 2 || !strings.Contains(opts[0].Label, "Available") || !strings.Contains(opts[1].Label, "Not available") {
		t.Fatalf("adapter labels = %#v", opts)
	}

	govStorage, _ := draft.FieldByKey("source_control.governance_storage")
	if len(govStorage.Options) != 2 {
		t.Fatalf("governance options = %#v", govStorage.Options)
	}
	if !govStorage.OptionDisabled("versioned") {
		t.Fatal("versioned must be disabled without GitHub")
	}

	if _, ok := draft.FieldByKey("governance.enabled"); ok {
		t.Fatal("governance.enabled must not exist")
	}
	if _, ok := draft.FieldByKey("stack.languages"); ok {
		t.Fatal("stack fields must not exist")
	}
	if _, ok := draft.FieldByKey("memory.enabled"); ok {
		t.Fatal("memory.enabled must not be an Init field")
	}

	workflow, _ := draft.FieldByKey("governance.default_workflow")
	if len(workflow.Options) != 1 || workflow.Options[0] != "sdd" {
		t.Fatalf("workflow options = %#v", workflow.Options)
	}

	engine, _ := draft.FieldByKey("governance.spec_engine")
	labels := []string{engine.VisibleOptions()[0].Label, engine.VisibleOptions()[1].Label}
	if labels[0] != "OpenSpec" || labels[1] != "None" {
		t.Fatalf("spec engine labels = %#v", labels)
	}

	branch, _ := draft.FieldByKey("source_control.branch_strategy")
	if branch.Value != "manual" || branch.InitMutability != config.FieldReadonly {
		t.Fatalf("branch strategy compat field = %#v", branch)
	}
}

func TestConfigDraft_DeliveryPlatformUnlocksVersioned(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})
	if draft.SelectOption("source_control.governance_storage", "versioned") {
		field, _ := draft.FieldByKey("source_control.governance_storage")
		if field.Value == "versioned" {
			t.Fatal("versioned must stay unavailable without GitHub")
		}
	}
	if !draft.SelectOption("source_control.mode", "git_github") {
		t.Fatal("expected delivery platform change")
	}
	field, _ := draft.FieldByKey("source_control.governance_storage")
	if len(field.Options) != 2 || field.OptionDisabled("versioned") {
		t.Fatalf("governance options with GitHub = %#v disabled=%v", field.Options, field.DisabledValues)
	}
	if !draft.SelectOption("source_control.governance_storage", "versioned") {
		t.Fatal("expected governance files change")
	}
	field, _ = draft.FieldByKey("source_control.governance_storage")
	if field.Value != "versioned" {
		t.Fatalf("governance = %q", field.Value)
	}
	if !draft.SelectOption("source_control.mode", "none") {
		t.Fatal("expected delivery platform reset")
	}
	field, _ = draft.FieldByKey("source_control.governance_storage")
	if field.Value != "local_only" {
		t.Fatalf("governance after platform clear = %q", field.Value)
	}
	if !field.OptionDisabled("versioned") {
		t.Fatal("versioned should be disabled again after leaving GitHub")
	}
}

func TestConfigDraft_Edits(t *testing.T) {
	t.Parallel()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:           "demo",
		ProjectMode:           "existing",
		ToolOpenCodeAvailable: true,
	})

	if !draft.SelectOption("governance.spec_engine", "none") {
		t.Fatal("expected spec engine change")
	}
	if !draft.ToggleMulti("adapters.selected", "opencode") {
		t.Fatal("expected adapter toggle")
	}
	adaptersUnavailable, _ := draft.FieldByKey("adapters.selected")
	if !adaptersUnavailable.OptionDisabled("cursor") {
		t.Fatal("cursor should remain marked unavailable without tool")
	}
	if draft.SelectOption("memory.strategy", "sqlite") {
		t.Fatal("memory.strategy must not be editable in Init UI")
	}

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "demo",
		ProjectMode: "existing",
	})
	if cfg.SetValue("project.name", "other") {
		t.Fatal("project.name must stay locked in configure")
	}
	if cfg.SelectOption("memory.strategy", "context_capsule") {
		t.Fatal("memory.strategy should stay readonly in configure")
	}
	cfgSelector := cfg.SelectorSections()
	foundMCP, foundContext, foundMemory := false, false, false
	for _, section := range cfgSelector {
		if section.Key == "mcp" {
			foundMCP = true
		}
		if section.Key == "context" {
			foundContext = true
		}
		if section.Key == "memory" {
			foundMemory = true
		}
	}
	if !foundMCP {
		t.Fatal("configure must show MCP section")
	}
	if foundContext {
		t.Fatal("configure must not show Context section in Slice 25")
	}
	if foundMemory {
		t.Fatal("configure must not show Memory section")
	}
	if cfg.SetValue("context.graph.enabled", "false") {
		t.Fatal("context.graph.enabled should stay readonly compatibility field")
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
