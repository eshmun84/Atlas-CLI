package doctor_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
)

func stringsContainsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func TestEvaluate_OpenSpecMissingCLIFails(t *testing.T) {
	root := t.TempDir()
	// Isolate PATH so Probe cannot see a real openspec binary.
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty-bin"))

	rt := healthyRuntime()
	rt.DependsOnSDDContract = true
	rt.SDDContractPresent = true
	rt.SDDContractMatches = true
	rt.Document.Governance.Workflow = config.WorkflowSDD
	rt.Document.Governance.SpecEngine = config.SpecProviderOpenSpec
	rt.Document.Governance.OpenSpecVersion = "1.14.1"
	report := doctor.Evaluate(inspect.Inspection{
		RootPath: root,
		Runtime:  rt,
		Tools:    []project.ToolInfo{{Name: "openspec", Available: false}},
	})
	assertHas(t, report, doctor.SeverityFail, "openspec provider", "openspec executable not found on PATH")
	for _, c := range report.Checks {
		if c.Name == "openspec provider" && c.Repairability != doctor.RepairManual {
			t.Fatalf("repairability = %q, want manual (Atlas does not install OpenSpec)", c.Repairability)
		}
		if c.Name == "openspec cli" || c.Name == "openspec layout" {
			t.Fatalf("duplicate OpenSpec check must be consolidated: %#v", c)
		}
		if c.Name == "tool openspec" {
			t.Fatalf("generic tool openspec check must be skipped when provider diagnostics own it: %#v", c)
		}
	}
}

func TestEvaluate_OpenSpecLegacyMkdirFails(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes"), 0o755); err != nil {
		t.Fatal(err)
	}
	rt := healthyRuntime()
	rt.DependsOnSDDContract = true
	rt.SDDContractPresent = true
	rt.SDDContractMatches = true
	rt.Document.Governance.Workflow = config.WorkflowSDD
	rt.Document.Governance.SpecEngine = config.SpecProviderOpenSpec
	report := doctor.Evaluate(inspect.Inspection{RootPath: root, Runtime: rt})
	found := false
	for _, c := range report.Checks {
		if c.Name == "openspec provider" && c.Severity == doctor.SeverityFail {
			found = true
			if c.Repairability != doctor.RepairManual {
				t.Fatalf("legacy mkdir repairability = %q, want manual (Init owns openspec init)", c.Repairability)
			}
			if !stringsContainsFold(c.RecommendedAction, "init") {
				t.Fatalf("legacy mkdir should guide Init: %#v", c)
			}
		}
	}
	if !found {
		t.Fatalf("expected openspec provider FAIL for legacy mkdir tree, got %#v", report.Checks)
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
	rt.Document.Governance.Workflow = config.WorkflowSDD
	rt.Document.Governance.SpecEngine = config.SpecProviderOpenSpec
	report := doctor.Evaluate(inspect.Inspection{RootPath: root, Runtime: rt})
	found := false
	for _, c := range report.Checks {
		if c.Name == "openspec provider" && c.Severity == doctor.SeverityFail {
			found = true
			if c.Repairability != doctor.RepairManual {
				t.Fatalf("partial layout repairability = %q, want manual", c.Repairability)
			}
		}
	}
	if !found {
		t.Fatal("expected openspec provider FAIL for partial tree")
	}
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
