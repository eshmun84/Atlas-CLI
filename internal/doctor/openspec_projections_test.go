package doctor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

func TestDoctorOpenSpecToolProjections_ReadyHealthy(t *testing.T) {
	root := t.TempDir()
	writeOpenSpecReadyMarkers(t, root, []string{"cursor"})
	insp := baseReadyOpenSpecInspection(root, []string{"cursor"})

	report := doctor.Evaluate(insp)
	c := findCheck(t, report, "openspec tool projections")
	if c.Severity != doctor.SeverityPass {
		t.Fatalf("want PASS, got %#v", c)
	}
}

func TestDoctorOpenSpecToolProjections_MissingCursor_ConfigurePath(t *testing.T) {
	root := t.TempDir()
	writeOpenSpecTreeOnly(t, root)

	insp := baseReadyOpenSpecInspection(root, []string{"cursor"})
	before := treeDigest(t, root)
	report := doctor.Evaluate(insp)
	_ = screens.StatusWithReport(insp, report)
	after := treeDigest(t, root)
	if before != after {
		t.Fatal("Status/Doctor must remain read-only")
	}

	c := findCheck(t, report, "openspec tool projections")
	if c.Severity != doctor.SeverityFail || c.Repairability != doctor.RepairManual {
		t.Fatalf("missing cursor projections: %#v", c)
	}
	if !strings.Contains(c.Message, "cursor") {
		t.Fatalf("message should name cursor: %q", c.Message)
	}
	if !strings.Contains(strings.ToLower(c.RecommendedAction), "configure") {
		t.Fatalf("remediation must point to Configure/Init: %q", c.RecommendedAction)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, insp.Runtime)
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindOpenSpec {
			t.Fatal("Repair must not claim OpenSpec tool projection authority")
		}
	}
}

func TestDoctorOpenSpecToolProjections_MissingOpenCode_ConfigurePath(t *testing.T) {
	root := t.TempDir()
	writeOpenSpecTreeOnly(t, root)

	insp := baseReadyOpenSpecInspection(root, []string{"opencode"})
	report := doctor.Evaluate(insp)
	c := findCheck(t, report, "openspec tool projections")
	if c.Severity != doctor.SeverityFail || c.Repairability != doctor.RepairManual {
		t.Fatalf("missing opencode projections: %#v", c)
	}
	if !strings.Contains(c.Message, "opencode") {
		t.Fatalf("message should name opencode: %q", c.Message)
	}
}

func baseReadyOpenSpecInspection(root string, adapters []string) inspect.Inspection {
	insp := inspect.Inspection{
		Runtime: runtime.Health{
			ConfigLoads:          true,
			DependsOnSDDContract: true,
			SDDContractPresent:   true,
			SDDContractMatches:   true,
			Document: config.ProjectDocument{
				Project: config.ProjectPersist{Name: "demo", Mode: "existing"},
				Governance: config.GovernancePersist{
					Workflow:        config.WorkflowSDD,
					SpecEngine:      config.SpecProviderOpenSpec,
					OpenSpecVersion: "1.2.3",
				},
				Adapters: config.AdaptersPersist{Selected: adapters},
			},
			SelectedAdapters: adapters,
		},
		OpenSpecProvider: openspec.Health{
			State:       openspec.StateReady,
			Installed:   "1.2.3",
			RequiredPin: "1.2.3",
			Project:     openspec.ProjectReady,
		},
		OpenSpecProviderProbed: true,
	}
	insp.RootPath = root
	return insp
}

func writeOpenSpecTreeOnly(t *testing.T, root string) {
	t.Helper()
	mustWrite(t, filepath.Join(root, "openspec", "config.yaml"), "schema: 1\n")
	mustMkdir(t, filepath.Join(root, "openspec", "specs"))
	mustMkdir(t, filepath.Join(root, "openspec", "changes"))
	mustMkdir(t, filepath.Join(root, "openspec", "changes", "archive"))
}

func writeOpenSpecReadyMarkers(t *testing.T, root string, adapters []string) {
	t.Helper()
	writeOpenSpecTreeOnly(t, root)
	for _, a := range adapters {
		switch a {
		case "cursor":
			mustWrite(t, filepath.Join(root, ".cursor", "commands", "opsx-propose.md"), "# opsx\n")
			mustWrite(t, filepath.Join(root, ".cursor", "skills", "openspec-propose", "SKILL.md"), "# skill\n")
		case "opencode":
			mustWrite(t, filepath.Join(root, ".opencode", "commands", "opsx-propose.md"), "# opsx\n")
			mustWrite(t, filepath.Join(root, ".opencode", "skills", "openspec-propose", "SKILL.md"), "# skill\n")
		}
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

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func treeDigest(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		b.WriteString(filepath.ToSlash(rel))
		b.WriteByte('|')
		if !info.IsDir() {
			data, _ := os.ReadFile(path)
			b.WriteString(string(data))
		}
		b.WriteByte('\n')
		return nil
	})
	return b.String()
}
