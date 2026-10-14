package doctor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func TestEvaluate_OpenSpecAbsentFailsConfirmRepair(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	rt := healthyRuntime()
	rt.DependsOnSDDContract = true
	rt.SDDContractPresent = true
	rt.SDDContractMatches = true
	rt.Document.Governance.Workflow = config.WorkflowSDD
	rt.Document.Governance.SpecEngine = config.SpecProviderOpenSpec
	report := doctor.Evaluate(inspect.Inspection{
		RootPath: root,
		Runtime:  rt,
		Tools:    []project.ToolInfo{{Name: "openspec", Available: false}},
	})
	assertHas(t, report, doctor.SeverityFail, "openspec layout", "required OpenSpec tree absent")
	assertHas(t, report, doctor.SeverityInfo, "openspec cli", "not on PATH (not required for Atlas artifact governance)")
	for _, c := range report.Checks {
		if c.Name == "openspec layout" && c.Repairability != doctor.RepairConfirm {
			t.Fatalf("repairability = %q, want confirm-repair", c.Repairability)
		}
	}
}

func TestEvaluate_OpenSpecPartialFailsManual(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	rt := healthyRuntime()
	rt.DependsOnSDDContract = true
	rt.SDDContractPresent = true
	rt.SDDContractMatches = true
	report := doctor.Evaluate(inspect.Inspection{RootPath: root, Runtime: rt})
	found := false
	for _, c := range report.Checks {
		if c.Name == "openspec layout" && c.Severity == doctor.SeverityFail {
			found = true
			if c.Repairability != doctor.RepairManual {
				t.Fatalf("partial layout repairability = %q", c.Repairability)
			}
		}
	}
	if !found {
		t.Fatal("expected openspec layout FAIL for partial tree")
	}
}

func TestEvaluate_OpenSpecValidPasses(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := openspec.EnsureBootstrap(root); err != nil {
		t.Fatal(err)
	}
	rt := healthyRuntime()
	rt.DependsOnSDDContract = true
	rt.SDDContractPresent = true
	rt.SDDContractMatches = true
	report := doctor.Evaluate(inspect.Inspection{RootPath: root, Runtime: rt})
	assertHas(t, report, doctor.SeverityPass, "openspec layout", "openspec/ and openspec/changes/ present")
}

func TestEvaluate_GitignoreMissingBlock(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("bin/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := healthyRuntime()
	rt.Document.SourceControl.GovernanceFiles = config.GovernanceFilesLocalOnly
	report := doctor.Evaluate(inspect.Inspection{RootPath: root, Runtime: rt})
	assertHas(t, report, doctor.SeverityFail, "gitignore", "no Atlas local governance block")
}

func TestEvaluate_GitignoreMalformedManual(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("# >>> Atlas local governance >>>\n.bin/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := healthyRuntime()
	rt.Document.SourceControl.GovernanceFiles = config.GovernanceFilesLocalOnly
	report := doctor.Evaluate(inspect.Inspection{RootPath: root, Runtime: rt})
	found := false
	for _, c := range report.Checks {
		if c.Name == "gitignore" && c.Severity == doctor.SeverityFail && c.Repairability == doctor.RepairManual {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected malformed gitignore FAIL manual, got %#v", report.Checks)
	}
}

func TestEvaluate_SigningPolicyVisibility(t *testing.T) {
	t.Parallel()
	rt := healthyRuntime()
	rt.Document.SourceControl.SigningRequired = true
	report := doctor.Evaluate(inspect.Inspection{
		RootPath: t.TempDir(),
		Git:      project.GitInfo{IsRepo: true, CurrentBranch: "main", CommitGPGSign: "false"},
		Runtime:  rt,
		Tools:    []project.ToolInfo{{Name: "git", Available: true}},
	})
	assertHas(t, report, doctor.SeverityPass, "signing policy", "Atlas signing_required=true")
	assertHas(t, report, doctor.SeverityWarn, "git commit.gpgsign", "false while Atlas signing_required=true (explicit commit -S still required)")
}
