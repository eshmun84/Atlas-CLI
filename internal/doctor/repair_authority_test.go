package doctor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

// DOGFOOD-019: Doctor atlas-repair / confirm-repair claims must match Runtime Repair authority.

func TestDoctorRepairAuthority_AtlasRepairHasPlanSupport(t *testing.T) {
	root := materializeDoctorProject(t, []string{"cursor"})

	if err := os.Remove(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal(err)
	}
	mdc := filepath.Join(root, filepath.FromSlash(config.FileCursorAtlasMDC))
	if err := os.WriteFile(mdc, []byte("# not atlas\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(insp)
	plan := runtime.BuildRuntimeRepairPlan(root, insp.Runtime)

	for _, c := range report.Checks {
		if c.Severity != doctor.SeverityFail && c.Severity != doctor.SeverityWarn {
			continue
		}
		if c.Repairability != doctor.RepairAtlas && c.Repairability != doctor.RepairConfirm {
			continue
		}
		if !doctorClaimSupportedByPlan(c, plan) {
			t.Fatalf("Doctor %s %q (%s) claims %s but Repair plan lacks support:\ncheck=%#v\nplan targets=%#v",
				c.Severity, c.Name, c.Message, c.Repairability, c, plan.Targets)
		}
	}
}

func TestDoctorRepairAuthority_OpenSpecInitNotAtlasRepair(t *testing.T) {
	root := materializeDoctorProject(t, []string{"cursor"})
	_ = os.RemoveAll(filepath.Join(root, "openspec"))

	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(insp)
	plan := runtime.BuildRuntimeRepairPlan(root, insp.Runtime)

	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindOpenSpec {
			t.Fatal("Repair must not plan OpenSpec bootstrap")
		}
	}

	found := false
	for _, c := range report.Checks {
		if c.Name != "openspec provider" || c.Severity != doctor.SeverityFail {
			continue
		}
		found = true
		if c.Repairability == doctor.RepairAtlas || c.Repairability == doctor.RepairConfirm {
			t.Fatalf("OpenSpec absent must not claim Runtime Repair: %#v", c)
		}
		if c.Repairability != doctor.RepairManual {
			t.Fatalf("OpenSpec absent repairability = %q, want manual", c.Repairability)
		}
		if !strings.Contains(strings.ToLower(c.RecommendedAction), "init") {
			t.Fatalf("OpenSpec absent should guide Init: %#v", c)
		}
	}
	if !found {
		t.Fatalf("expected openspec provider FAIL, checks=%#v", report.Checks)
	}
}

func TestDoctorRepairAuthority_AdapterDriftDiagnosed(t *testing.T) {
	root := materializeDoctorProject(t, []string{"cursor"})
	mdc := filepath.Join(root, filepath.FromSlash(config.FileCursorAtlasMDC))
	if err := os.WriteFile(mdc, []byte("# drifted adapter projection\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	var proj *runtime.ProjectionStatus
	for i := range insp.Runtime.ExpectedProjections {
		p := &insp.Runtime.ExpectedProjections[i]
		if p.Adapter == "cursor" {
			proj = p
			break
		}
	}
	if proj == nil || !proj.Present || proj.Matches {
		t.Fatalf("expected present-but-drifted projection: %#v", insp.Runtime.ExpectedProjections)
	}

	report := doctor.Evaluate(insp)
	found := false
	for _, c := range report.Checks {
		if c.Name == "adapter projection cursor" && c.Severity == doctor.SeverityFail {
			found = true
			if c.Repairability != doctor.RepairAtlas {
				t.Fatalf("adapter drift repairability = %q", c.Repairability)
			}
			if !strings.Contains(strings.ToLower(c.Message), "drift") {
				t.Fatalf("expected drift message: %#v", c)
			}
		}
	}
	if !found {
		t.Fatalf("Doctor must FAIL drifted adapter projection, checks=%#v", report.Checks)
	}

	plan := runtime.BuildRuntimeRepairPlan(root, insp.Runtime)
	foundReplace := false
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindAdapter && target.Action == runtime.RepairActionReplace {
			foundReplace = true
		}
	}
	if !foundReplace {
		t.Fatalf("Repair must plan adapter replace for drift: %#v", plan.Targets)
	}
}

func TestDoctorRepairAuthority_InvalidSkillsNotAtlasRepair(t *testing.T) {
	root := materializeDoctorProject(t, []string{"cursor"})
	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(insp.Runtime.Skills.Projections) == 0 {
		t.Skip("no skill projections to mutate")
	}

	projs := make([]skills.ProjectionStatus, len(insp.Runtime.Skills.Projections))
	copy(projs, insp.Runtime.Skills.Projections)
	for i := range projs {
		projs[i].State = "ready"
		projs[i].Owned = true
	}
	projs[0].State = "invalid"
	insp.Runtime.Skills.Projections = projs

	report := doctor.Evaluate(insp)
	found := false
	for _, c := range report.Checks {
		if c.Name != "skills projections" || c.Severity != doctor.SeverityFail {
			continue
		}
		found = true
		if c.Repairability == doctor.RepairAtlas {
			t.Fatalf("invalid skills must not claim atlas-repair: %#v", c)
		}
		if c.Repairability != doctor.RepairManual {
			t.Fatalf("invalid skills repairability = %q", c.Repairability)
		}
	}
	if !found {
		t.Fatalf("expected skills projections FAIL for invalid, checks=%#v", report.Checks)
	}
}

func doctorClaimSupportedByPlan(c doctor.Check, plan runtime.RuntimeRepairPlan) bool {
	name := strings.ToLower(c.Name)
	switch {
	case strings.Contains(name, "openspec provider"):
		return false
	case strings.Contains(name, "adapter projection"):
		return planHasKind(plan, runtime.RepairKindAdapter)
	case strings.HasPrefix(name, "agents") || name == "runtime materialization":
		return planHasKind(plan, runtime.RepairKindAgents) || planHasPath(plan, config.FileAgentsMD)
	case name == "atlas agents":
		return planHasKind(plan, runtime.RepairKindAgent)
	case name == "agent registry" || name == "skill registry" || name == "runtime manifest" ||
		name == "assets lock" || name == "sdd openspec contract":
		return planHasKind(plan, runtime.RepairKindAtlas)
	case name == "skills projections":
		return planHasKind(plan, runtime.RepairKindSkill)
	case name == "gitignore":
		return planHasKind(plan, runtime.RepairKindGitignore)
	case strings.Contains(name, "atlas home"):
		return planHasKind(plan, runtime.RepairKindHome)
	default:
		return len(plan.Targets) > 0
	}
}

func planHasKind(plan runtime.RuntimeRepairPlan, kind string) bool {
	for _, t := range plan.Targets {
		if t.Kind == kind {
			return true
		}
	}
	return false
}

func planHasPath(plan runtime.RuntimeRepairPlan, path string) bool {
	for _, t := range plan.Targets {
		if t.Path == path {
			return true
		}
	}
	return false
}

func materializeDoctorProject(t *testing.T, adapters []string) string {
	t.Helper()
	t.Setenv("ATLAS_HOME", t.TempDir())
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatalf("home: %v", err)
	}
	root := t.TempDir()
	in := config.ProjectSetupInput{
		ProjectName:   "demo",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	}
	for _, adapter := range adapters {
		switch adapter {
		case "cursor":
			in.ToolCursorAvailable = true
		case "opencode":
			in.ToolOpenCodeAvailable = true
		}
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, in)
	for _, adapter := range adapters {
		if !draft.ToggleMulti("adapters.selected", adapter) {
			t.Fatalf("toggle %s", adapter)
		}
	}
	fixed := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed },
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return root
}
