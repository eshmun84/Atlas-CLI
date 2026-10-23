package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/hookproj"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/sdd"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
	"gopkg.in/yaml.v3"
)

func TestSlice02_CorruptHookOwnership_NotRepairable(t *testing.T) {
	root, doc := slice02Project(t)
	writeCorruptOwnership(t, root, doc.Project.Name, hookproj.OwnershipPath)

	health := config.InspectHookHealth(root, doc)
	if health.OwnershipError == "" || len(health.Projections) != 0 || health.RepairableCount() != 0 {
		t.Fatalf("corrupt hooks ownership must not invent projections: %#v", health)
	}
	report := doctor.Evaluate(slice02Inspection(root, doc, health, config.InspectSkillHealth(root, doc)))
	check := slice02Check(t, report, "hooks ownership")
	if check.Severity != doctor.SeverityFail || check.Repairability != doctor.RepairManual {
		t.Fatalf("hooks ownership diagnostic = %#v", check)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, slice02Runtime(doc, health, skills.SkillHealth{}))
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindHook {
			t.Fatalf("repair plan must not target hooks when ownership is unreadable: %#v", plan.Targets)
		}
	}
}

func TestSlice02_CorruptSkillOwnership_NotRepairable(t *testing.T) {
	root, doc := slice02Project(t)
	doc.Skills.Enabled = []config.SkillPinPersist{{ID: "demo-skill", Version: "0.1.0"}}
	writeCatalogSkill(t, "demo-skill", "0.1.0")
	writeCorruptOwnership(t, root, doc.Project.Name, skills.OwnershipPath)

	health := config.InspectSkillHealth(root, doc)
	if health.OwnershipError == "" || len(health.Projections) != 0 {
		t.Fatalf("corrupt skills ownership must not invent projections: %#v", health)
	}
	report := doctor.Evaluate(slice02Inspection(root, doc, hookproj.HookHealth{Configured: false}, health))
	check := slice02Check(t, report, "skills ownership")
	if check.Severity != doctor.SeverityFail || check.Repairability != doctor.RepairManual {
		t.Fatalf("skills ownership diagnostic = %#v", check)
	}
	plan := runtime.BuildRuntimeRepairPlan(root, slice02Runtime(doc, hookproj.HookHealth{}, health))
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindSkill {
			t.Fatalf("repair plan must not target skills when ownership is unreadable: %#v", plan.Targets)
		}
	}
}

func TestSlice02_OwnershipError_DoesNotClaimHealthyRepair(t *testing.T) {
	root, doc := slice02Project(t)
	doc.SourceControl.GovernanceFiles = config.GovernanceFilesVersioned
	body, err := config.RenderAgentsMD(doc.Project.Name, doc.ContextGraphEnabled(), doc.Adapters.Selected, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	markers := config.InspectAgentsMarkers([]byte(body))
	health := slice02Runtime(doc, hookproj.HookHealth{
		Configured:     true,
		OwnershipError: "hooks ownership: parse: boom",
	}, skills.SkillHealth{
		CatalogReadable: true,
		OwnershipError:  "skills ownership: parse: boom",
	})
	health.SelectedAdapters = append([]string{}, doc.Adapters.Selected...)
	health.AgentsExists = true
	health.AgentsMarkers = markers
	health.AgentRegistryPresent = true
	health.AgentRegistryMatches = true
	health.SkillRegistryPresent = true
	health.SkillRegistryMatches = true
	health.RuntimeManifestPresent = true
	health.RuntimeManifestMatches = true
	health.AssetsLockPresent = true
	health.AssetsLockMatches = true
	health.StateExists = true
	health.StateLoads = true

	plan := runtime.BuildRuntimeRepairPlan(root, health)
	if plan.Healthy || plan.NeedsApply() || len(plan.Targets) != 0 {
		t.Fatalf("ownership failure must not look repairable or healthy: healthy=%v apply=%v targets=%#v warnings=%#v",
			plan.Healthy, plan.NeedsApply(), plan.Targets, plan.Warnings)
	}
	joined := strings.Join(plan.Warnings, "\n")
	if !strings.Contains(joined, "hooks ownership") || !strings.Contains(joined, "skills ownership") {
		t.Fatalf("warnings = %#v", plan.Warnings)
	}
}

func TestSlice02_MissingSkillOwnership_StillRepairable(t *testing.T) {
	root, doc := slice02Project(t)
	doc.Skills.Enabled = []config.SkillPinPersist{{ID: "demo-skill", Version: "0.1.0"}}
	writeCatalogSkill(t, "demo-skill", "0.1.0")

	health := config.InspectSkillHealth(root, doc)
	if health.OwnershipError != "" {
		t.Fatalf("absent ownership file is empty ownership: %s", health.OwnershipError)
	}
	missing := false
	for _, proj := range health.Projections {
		if proj.State == "missing" && proj.Message == "projection missing" {
			missing = true
		}
	}
	if !missing {
		t.Fatalf("absent ownership with a missing projection stays visible: %#v", health.Projections)
	}
	report := doctor.Evaluate(slice02Inspection(root, doc, hookproj.HookHealth{Configured: false}, health))
	check := slice02Check(t, report, "skills projections")
	if check.Repairability != doctor.RepairAtlas {
		t.Fatalf("missing skill projection = %#v", check)
	}
}

func TestSlice02_MissingHookOwnership_StillRepairable(t *testing.T) {
	root, doc := slice02Project(t)
	health := config.InspectHookHealth(root, doc)
	if health.RepairableCount() == 0 {
		t.Fatalf("absent ownership file with a missing projection stays repairable: %#v", health.Projections)
	}
	report := doctor.Evaluate(slice02Inspection(root, doc, health, skills.SkillHealth{CatalogReadable: true}))
	check := slice02Check(t, report, "hooks projections")
	if check.Repairability != doctor.RepairAtlas {
		t.Fatalf("missing projection = %#v", check)
	}
}

func TestSlice02_InvalidProjectDocument_DoesNotLoadAsLegacy(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig("hybrid")
	cfg.Project.Mode = config.ModeExisting
	cfg.Orchestration.DefaultWorkflow = config.WorkflowNone
	cfg.SpecEngine.Enabled = false
	cfg.SpecEngine.Provider = ""
	cfg.Memory.Enabled = false
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(data), "governance:\n", "governance:\n    workflow: guided\n", 1)
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	docErr := error(nil)
	if _, docErr = config.LoadProjectDocumentAt(root); docErr == nil || errors.Is(docErr, config.ErrNotProjectDocument) {
		t.Fatalf("validation failure must stay distinct from a legacy shape: %v", docErr)
	}
	if _, err := config.LoadAt(root); err == nil {
		t.Fatal("invalid project document must not succeed as legacy config")
	}
	files, err := project.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	status := project.EvaluateAtlasStatus(root, files)
	if status.Initialized() || status.State != project.AtlasStateInvalidConfig {
		t.Fatalf("status = %#v", status)
	}
}

func TestSlice02_LegacyConfig_StillLoads(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig("legacy-only")
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), data, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.LoadAt(root)
	if err != nil {
		t.Fatalf("legacy config must still load: %v", err)
	}
	if loaded.Project.Name != "legacy-only" {
		t.Fatalf("name = %q", loaded.Project.Name)
	}
	if _, err := config.LoadProjectDocumentAt(root); !errors.Is(err, config.ErrNotProjectDocument) {
		t.Fatalf("legacy config must be rejected as a project document: %v", err)
	}
	files, err := project.DiscoverFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	if !project.EvaluateAtlasStatus(root, files).Initialized() {
		t.Fatal("legacy config remains an initialized Atlas status")
	}
}

func TestSlice02_SDDLayoutAndProviderStayDistinct(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	insp := inspect.Inspection{
		Runtime: runtime.Health{
			ConfigLoads:          true,
			DependsOnSDDContract: true,
			Document: config.ProjectDocument{
				Governance: config.GovernancePersist{OpenSpecVersion: "1.2.3"},
			},
			SDDContractPresent: true,
			SDDContractMatches: true,
		},
		SDD: sdd.Overview{
			Presence: sdd.Presence{Engine: sdd.EngineID("openspec"), Present: true, Label: "OpenSpec"},
			Active:   []sdd.ChangeRef{{ChangeID: "layout-only"}},
		},
		OpenSpecProvider: openspec.Health{
			State:   openspec.StateError,
			Project: openspec.ProjectMalformed,
			Message: "provider rejected project",
		},
		OpenSpecProviderProbed: true,
	}
	report := doctor.Evaluate(insp)
	engine := slice02Check(t, report, "sdd engine")
	if engine.Severity != doctor.SeverityPass {
		t.Fatalf("layout overview = %#v", engine)
	}
	provider := slice02Check(t, report, "openspec provider")
	if provider.Severity != doctor.SeverityFail || provider.Repairability != doctor.RepairManual {
		t.Fatalf("provider probe = %#v", provider)
	}
}

func slice02Project(t *testing.T) (string, config.ProjectDocument) {
	t.Helper()
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:         "slice02",
		ProjectMode:         "existing",
		DefaultRemote:       "origin",
		ToolCursorAvailable: true,
	})
	if !draft.ToggleMulti("adapters.selected", "cursor") {
		t.Fatal("toggle cursor")
	}
	return root, config.BuildProjectDocument(draft, config.EmptyMCPDraft())
}

func writeCatalogSkill(t *testing.T, id, version string) {
	t.Helper()
	homePath, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(homePath, "assets", "skills", id, version)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: " + id + "\ndescription: slice02 fixture\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeCorruptOwnership(t *testing.T, root, name string, pathFn func(string, string) string) {
	t.Helper()
	homePath, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	projectID, err := home.ProjectID(root, name)
	if err != nil {
		t.Fatal(err)
	}
	path := pathFn(homePath, projectID)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(":\n  - ["), 0o600); err != nil {
		t.Fatal(err)
	}
}

func slice02Inspection(root string, doc config.ProjectDocument, hooks hookproj.HookHealth, skill skills.SkillHealth) inspect.Inspection {
	return inspect.Inspection{
		RootPath: root,
		Runtime:  slice02Runtime(doc, hooks, skill),
	}
}

func slice02Runtime(doc config.ProjectDocument, hooks hookproj.HookHealth, skill skills.SkillHealth) runtime.Health {
	homePath, _ := home.Resolve()
	return runtime.Health{
		Initialized:  true,
		ConfigExists: true,
		ConfigLoads:  true,
		Document:     doc,
		AgentsExists: false,
		Hooks:        hooks,
		Skills:       skill,
		Home:         home.Status{Exists: true, Path: homePath, Writable: true, LayoutComplete: true, StateLoads: true},
	}
}

func slice02Check(t *testing.T, report doctor.Report, name string) doctor.Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.Name == name {
			return check
		}
	}
	t.Fatalf("missing check %q", name)
	return doctor.Check{}
}
