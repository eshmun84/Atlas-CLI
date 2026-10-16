package doctor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/hookproj"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func mustInspect(t *testing.T, root string) inspect.Inspection {
	t.Helper()
	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	return insp
}

func findCheck(t *testing.T, report doctor.Report, name string) doctor.Check {
	t.Helper()
	for _, c := range report.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("missing check %q in %#v", name, report.Checks)
	return doctor.Check{}
}

func TestDoctorHooks_Healthy(t *testing.T) {
	root := materializeDoctorProject(t, []string{"cursor"})
	insp := mustInspect(t, root)
	report := doctor.Evaluate(insp)
	c := findCheck(t, report, "hooks projections")
	if c.Severity != doctor.SeverityPass {
		t.Fatalf("want PASS, got %#v", c)
	}
	if c.Repairability != "" {
		t.Fatalf("healthy hooks must not claim repairability: %#v", c)
	}
}

func TestDoctorHooks_MissingOwnedWrapper_Repairable(t *testing.T) {
	root := materializeDoctorProject(t, []string{"cursor"})
	wrapper := filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")
	if err := os.Remove(wrapper); err != nil {
		t.Fatal(err)
	}
	insp := mustInspect(t, root)
	report := doctor.Evaluate(insp)
	c := findCheck(t, report, "hooks projections")
	if c.Severity != doctor.SeverityFail || c.Repairability != doctor.RepairAtlas {
		t.Fatalf("missing wrapper: %#v", c)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, insp.Runtime)
	found := false
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindHook && target.Path == ".cursor/hooks/atlas/dispatch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Doctor RepairAtlas must match Repair plan target: %#v", plan.Targets)
	}
}

func TestDoctorHooks_DriftedOwnedWrapper_Manual(t *testing.T) {
	root := materializeDoctorProject(t, []string{"cursor"})
	wrapper := filepath.Join(root, ".cursor", "hooks", "atlas", "dispatch")
	if err := os.WriteFile(wrapper, []byte("user-edited-hooks\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	insp := mustInspect(t, root)
	report := doctor.Evaluate(insp)
	c := findCheck(t, report, "hooks projections")
	if c.Severity != doctor.SeverityFail || c.Repairability != doctor.RepairManual {
		t.Fatalf("drifted wrapper must be manual: %#v", c)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, insp.Runtime)
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindHook {
			t.Fatalf("Repair must not claim drifted hooks: %#v", plan.Targets)
		}
	}
	var drifted bool
	for _, p := range insp.Runtime.Hooks.Projections {
		if p.State == hookproj.StateDrifted {
			drifted = true
		}
	}
	if !drifted {
		t.Fatalf("expected drifted projection: %#v", insp.Runtime.Hooks.Projections)
	}
}

func TestDoctorHooks_ConflictUnowned_Manual(t *testing.T) {
	root := materializeDoctorProject(t, []string{"cursor"})
	// Remove ownership so existing wrapper becomes unowned conflict.
	homePath := os.Getenv("ATLAS_HOME")
	ownPath := filepath.Join(homePath, "projects")
	entries, _ := os.ReadDir(ownPath)
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(ownPath, e.Name(), "hooks"))
	}
	insp := mustInspect(t, root)
	report := doctor.Evaluate(insp)
	c := findCheck(t, report, "hooks projections")
	if c.Severity != doctor.SeverityFail || c.Repairability != doctor.RepairManual {
		t.Fatalf("unowned conflict must be manual: %#v", c)
	}
}

func TestDoctorHooks_Disabled_NoFalseFailure(t *testing.T) {
	// No adapters → hooks not configured.
	root := t.TempDir()
	t.Setenv("ATLAS_HOME", t.TempDir())
	insp := inspect.Inspection{
		Runtime: runtime.Health{
			ConfigLoads: true,
			Hooks:       hookproj.HookHealth{Configured: false},
		},
	}
	report := doctor.Evaluate(insp)
	c := findCheck(t, report, "hooks")
	if c.Severity != doctor.SeverityInfo {
		t.Fatalf("disabled hooks must be INFO, got %#v", c)
	}
	_ = root
}

func TestDoctorHooks_MetadataMatchesRepairTarget(t *testing.T) {
	root := materializeDoctorProject(t, []string{"opencode"})
	plugin := filepath.Join(root, ".opencode", "plugins", "atlas-hooks", "index.ts")
	if err := os.Remove(plugin); err != nil {
		t.Fatal(err)
	}
	insp := mustInspect(t, root)
	report := doctor.Evaluate(insp)
	c := findCheck(t, report, "hooks projections")
	if c.Repairability != doctor.RepairAtlas {
		t.Fatalf("expected atlas-repair: %#v", c)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, insp.Runtime)
	found := false
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindHook && target.Path == ".opencode/plugins/atlas-hooks/index.ts" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Doctor/Repair mismatch: %#v", plan.Targets)
	}
}
