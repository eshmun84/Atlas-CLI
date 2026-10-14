package doctor_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/memoryhealth"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/sdd"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

// TestRepairabilityMetadata_FailAndWarnGuards prevents reintroducing FAIL/WARN
// checks without an explicit supported Repairability value.
func TestRepairabilityMetadata_FailAndWarnGuards(t *testing.T) {
	t.Parallel()

	reports := []doctor.Report{
		doctor.EvaluateWorkingDirectoryError(errors.New("permission denied")),
		doctor.EvaluateDiscoveryError(errors.New("boom")),
		doctor.Evaluate(inspect.Inspection{
			RootPath: t.TempDir(),
			Git:      project.GitInfo{IsRepo: false},
			Runtime:  runtime.Health{},
			Tools:    []project.ToolInfo{{Name: "git", Available: false}, {Name: "openspec", Available: false}},
		}),
		doctor.Evaluate(inspect.Inspection{
			RootPath: t.TempDir(),
			Git:      project.GitInfo{IsRepo: false},
			Runtime: func() runtime.Health {
				rt := healthyRuntime()
				rt.Document.SourceControl.Mode = config.SourceControlGitLocal
				rt.DependsOnSDDContract = false
				return rt
			}(),
			Tools: []project.ToolInfo{{Name: "git", Available: true}},
		}),
		doctor.Evaluate(inspect.Inspection{
			RootPath: t.TempDir(),
			Git:      project.GitInfo{IsRepo: true, CurrentBranch: "main", CommitGPGSign: "false"},
			Runtime: func() runtime.Health {
				rt := healthyRuntime()
				rt.Document.SourceControl.SigningRequired = true
				rt.DependsOnSDDContract = true
				rt.SDDContractPresent = false
				rt.SDDContractMatches = false
				rt.AgentsExists = false
				rt.RuntimeMaterialized = true
				rt.ExpectedProjections = []runtime.ProjectionStatus{{Adapter: "cursor", Path: config.FileCursorAtlasMDC, Present: false}}
				rt.RuntimeManifestMatches = false
				rt.AssetsLockMatches = false
				rt.AgentRegistryPresent = false
				rt.SkillRegistryPresent = false
				rt.Skills = skills.SkillHealth{CatalogReadable: false, CatalogError: "boom"}
				rt.CodeIntelligence = codeintel.Snapshot{Provider: codeintel.ProviderCodeGraph, State: codeintel.StateStale, Freshness: "stale"}
				rt.Memory = memoryhealth.Diagnostics{
					Status: memoryhealth.StatusFailed,
					Checks: []memoryhealth.Check{{
						ID:       "memory.schema",
						Severity: memoryhealth.SeverityFail,
						Message:  "schema unreadable",
					}},
				}
				rt.Home.Exists = false
				return rt
			}(),
			Tools: []project.ToolInfo{{Name: "git", Available: true}, {Name: "openspec", Available: false}},
			SDD: sdd.Overview{
				EngineConflict: true,
				Issues: []sdd.Issue{{
					Kind:    sdd.IssueUnsafe,
					Message: "symlink escape",
				}},
			},
		}),
	}

	// OpenSpec absent + gitignore missing block + malformed gitignore.
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("bin/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := healthyRuntime()
	rt.DependsOnSDDContract = true
	rt.SDDContractPresent = true
	rt.SDDContractMatches = true
	rt.Document.SourceControl.GovernanceFiles = config.GovernanceFilesLocalOnly
	reports = append(reports, doctor.Evaluate(inspect.Inspection{RootPath: root, Runtime: rt}))

	malformedRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(malformedRoot, ".gitignore"), []byte("# >>> Atlas local governance >>>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt2 := healthyRuntime()
	rt2.Document.SourceControl.GovernanceFiles = config.GovernanceFilesLocalOnly
	reports = append(reports, doctor.Evaluate(inspect.Inspection{RootPath: malformedRoot, Runtime: rt2}))

	failCount := 0
	warnCount := 0
	for _, report := range reports {
		if problems := doctor.ValidateRepairabilityMetadata(report); len(problems) > 0 {
			t.Fatalf("repairability metadata violations:\n%s\nreport=%#v", joinLines(problems), report.Checks)
		}
		for _, c := range report.Checks {
			switch c.Severity {
			case doctor.SeverityFail:
				failCount++
			case doctor.SeverityWarn:
				warnCount++
			}
		}
	}
	if failCount < 5 {
		t.Fatalf("guard scenario coverage too thin: failCount=%d", failCount)
	}
	if warnCount < 1 {
		t.Fatalf("expected at least one WARN in guard scenarios")
	}
}

func TestValidRepairabilityEnum(t *testing.T) {
	t.Parallel()
	for _, r := range []doctor.Repairability{
		doctor.RepairAtlas, doctor.RepairConfirm, doctor.RepairDiagnostic, doctor.RepairManual,
	} {
		if !doctor.ValidRepairability(r) {
			t.Fatalf("expected valid: %q", r)
		}
	}
	if doctor.ValidRepairability("") || doctor.ValidRepairability("auto") {
		t.Fatal("empty/unknown must be invalid")
	}
}

func joinLines(items []string) string {
	out := ""
	for _, item := range items {
		out += "- " + item + "\n"
	}
	return out
}
