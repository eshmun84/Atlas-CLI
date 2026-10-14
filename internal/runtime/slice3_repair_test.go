package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func TestRepair_OpenSpecAbsentBootstrapIdempotent(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	_ = os.RemoveAll(filepath.Join(root, "openspec"))

	health := mustDiscover(t, root).Runtime
	if !health.DependsOnSDDContract {
		t.Fatal("expected DependsOnSDDContract")
	}
	plan := runtime.BuildRuntimeRepairPlan(root, health)
	found := false
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindOpenSpec {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected openspec repair target, plan=%#v", plan)
	}
	res := applyRepair(t, root, plan.Signature(), fixedClock(2026, 10, 14, 20, 0, 0))
	if res.Noop || res.Blocked {
		t.Fatalf("unexpected result: %#v", res)
	}
	assess, err := openspec.AssessLayout(root)
	if err != nil || !assess.Valid {
		t.Fatalf("layout after repair: %#v err=%v", assess, err)
	}

	plan2 := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan2.Targets {
		if target.Kind == runtime.RepairKindOpenSpec {
			t.Fatal("second plan must not include openspec bootstrap")
		}
	}
}

func TestRepair_OpenSpecPartialRefused(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	_ = os.RemoveAll(filepath.Join(root, "openspec"))
	if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	marker := []byte("keep-me\n")
	if err := os.WriteFile(filepath.Join(root, "openspec", "marker.txt"), marker, 0o644); err != nil {
		t.Fatal(err)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindOpenSpec {
			t.Fatal("partial openspec must not be auto-repairable")
		}
	}
	got, err := os.ReadFile(filepath.Join(root, "openspec", "marker.txt"))
	if err != nil || string(got) != string(marker) {
		t.Fatalf("partial tree mutated: %q %v", got, err)
	}
}

func TestRepair_GitignoreReconcileIdempotent(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	// Default init uses local_only governance; overwrite to a project-owned body without Atlas block.
	giPath := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(giPath, []byte("bin/\n.tmp/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	found := false
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindGitignore {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected gitignore repair, drift=%v targets=%v", plan.Drift, plan.Targets)
	}
	before, _ := os.ReadFile(giPath)
	res := applyRepair(t, root, plan.Signature(), fixedClock(2026, 10, 14, 20, 2, 0))
	if res.Blocked {
		t.Fatalf("blocked: %#v", res)
	}
	after, err := os.ReadFile(giPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "bin/") {
		t.Fatalf("project rules lost: %q", after)
	}
	if !strings.Contains(string(after), ".atlas/") {
		t.Fatalf("atlas block missing: %q", after)
	}
	if string(after) == string(before) {
		t.Fatal("expected gitignore mutation")
	}

	plan2 := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan2.Targets {
		if target.Kind == runtime.RepairKindGitignore {
			t.Fatal("second plan must not re-repair gitignore")
		}
	}
}

func TestRepair_GitignoreMalformedRefused(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	giPath := filepath.Join(root, ".gitignore")
	malformed := []byte("# >>> Atlas local governance >>>\nonly-begin\n")
	if err := os.WriteFile(giPath, malformed, 0o644); err != nil {
		t.Fatal(err)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindGitignore {
			t.Fatal("malformed gitignore must not be auto-repaired")
		}
	}
	got, _ := os.ReadFile(giPath)
	if string(got) != string(malformed) {
		t.Fatal("malformed gitignore was mutated")
	}
}

func fixedClock(y int, m time.Month, d, hh, mm, ss int) func() time.Time {
	return func() time.Time {
		return time.Date(y, m, d, hh, mm, ss, 0, time.UTC)
	}
}
