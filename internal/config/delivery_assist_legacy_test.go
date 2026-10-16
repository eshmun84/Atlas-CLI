package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestDeliveryAssist_LegacyConfigLoadsAndPreserves(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".atlas", "config.yaml"), `
schema_version: 1
project:
  name: legacy-assist
  mode: existing
governance:
  workflow: sdd
  spec_engine: openspec
  openspec_version: ""
  testing_required: true
  review_required: true
  evidence_required: false
adapters:
  selected: [cursor]
source_control:
  mode: git_local
  default_remote: origin
  branch_strategy: manual
  governance_files: local_only
  delivery_assist: true
  signing_required: false
memory:
  strategy: sqlite_plus_context_capsule
`)

	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.SourceControl.DeliveryAssist {
		t.Fatal("legacy delivery_assist:true must load")
	}

	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "legacy-assist",
		ProjectMode: "existing",
	})
	config.ApplyProjectDocument(&draft, doc)
	field, ok := draft.FieldByKey("source_control.delivery_assist")
	if !ok {
		t.Fatal("draft must retain delivery_assist for persist compatibility")
	}
	if field.Value != "true" {
		t.Fatalf("value=%q", field.Value)
	}
	if field.Editable(config.ConfigModeConfigure) || field.Editable(config.ConfigModeInit) {
		t.Fatal("delivery_assist must not be editable")
	}
	// Hidden from selector UI (compat section).
	for _, sec := range draft.SelectorSections() {
		for _, f := range sec.Fields {
			if f.Key == "source_control.delivery_assist" {
				t.Fatal("delivery_assist must not appear in SelectorSections")
			}
		}
	}

	// Document rebuild from draft must preserve the legacy field (no runtime engine).
	rebuilt := config.BuildProjectDocument(draft, config.EmptyMCPDraft())
	if !rebuilt.SourceControl.DeliveryAssist {
		t.Fatal("persist mapping must not erase legacy delivery_assist:true")
	}
}

func mustWrite(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
