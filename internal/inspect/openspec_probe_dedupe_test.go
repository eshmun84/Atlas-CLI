package inspect_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

func TestInspect_GovernedSDD_ProbesOpenSpecOnceForStatusDoctor(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".atlas", "config.yaml"), `
schema_version: 1
project:
  name: probe-dedupe
  mode: existing
governance:
  workflow: sdd
  spec_engine: openspec
  openspec_version: "1.0.0"
  testing_required: true
  review_required: true
  evidence_required: false
adapters:
  selected: [cursor]
source_control:
  mode: none
  default_remote: origin
  branch_strategy: manual
  governance_files: local_only
  delivery_assist: false
  signing_required: false
memory:
  strategy: sqlite_plus_context_capsule
`)

	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if !insp.Runtime.ConfigLoads {
		t.Fatalf("config must load: %s", insp.Runtime.ConfigError)
	}
	if !insp.Runtime.DependsOnSDDContract {
		t.Fatal("expected DependsOnSDDContract")
	}
	if !insp.OpenSpecProviderProbed {
		t.Fatal("Inspect must probe OpenSpec provider once for governed SDD")
	}

	report := doctor.Evaluate(insp)
	_ = screens.StatusWithReport(insp, report)
	_ = screens.Doctor(report, insp)
	_ = doctor.Evaluate(insp)

	if !insp.OpenSpecProviderProbed {
		t.Fatal("probe flag must remain set after Status/Doctor")
	}
}

func TestInspect_NonSDD_NoOpenSpecProviderProbe(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if insp.OpenSpecProviderProbed {
		t.Fatal("non-SDD inspection must not probe OpenSpec provider")
	}
}

func mustWriteFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
