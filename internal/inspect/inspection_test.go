package inspect_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/delivery"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestInspection_ComposesProjectAndRuntime(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if insp.RootPath == "" || !insp.Files.HasReadme {
		t.Fatalf("project facts missing: %#v", insp.ProjectSnapshot())
	}
	if insp.Runtime.ConfigExists {
		t.Fatal("uninitialized project should not report config")
	}
	if delivery.DefaultMode(insp.Git.IsRepo) != delivery.ModeNone {
		t.Fatal("no-git should seed delivery none")
	}
}

func TestInspection_GitRepoSeedsDeliveryDefault(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("branch", "-M", "develop")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("-c", "user.email=t@ex.com", "-c", "user.name=T", "-c", "commit.gpgsign=false", "commit", "-m", "seed")

	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if !insp.Git.IsRepo || insp.Git.CurrentBranch != "develop" {
		t.Fatalf("git facts = %#v", insp.Git)
	}
	if delivery.DefaultMode(insp.ProjectSnapshot().Git.IsRepo) != delivery.ModeGitLocal {
		t.Fatal("git repo must seed git_local from canonical state")
	}

	// Init draft seed uses the same Git facts — no second Git probe.
	setup := config.ProjectSetupInput{GitRepoDetected: insp.Git.IsRepo}
	if config.DefaultSourceControlMode(setup.GitRepoDetected) != string(delivery.ModeGitLocal) {
		t.Fatal("Init must consume Git from Inspection")
	}
	plan, err := initplan.Build(root, insp)
	if err != nil {
		t.Fatal(err)
	}
	if plan.RootPath == "" {
		t.Fatal("init plan empty")
	}
}

func TestInspection_SharedByStatusAndDoctor(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	ciBefore := insp.Runtime.CodeIntelligence
	before := treeSnapshot(t, root)
	report := doctor.Evaluate(insp)
	status := screens.StatusWithReport(insp, report)
	doc := screens.Doctor(report, insp)
	assertTreeUnchanged(t, root, before)
	if status == "" || doc == "" {
		t.Fatal("empty renders")
	}
	// Consumers read the same CI evidence; they must not mutate or re-probe.
	if insp.Runtime.CodeIntelligence != ciBefore {
		t.Fatal("Code Intelligence evidence changed after Status/Doctor")
	}
	if status == "" {
		t.Fatal("status empty")
	}
	_ = screens.Status(insp) // uses doctor.Evaluate on same Inspection — no Inspect call
}

func TestInspection_NoSecondCodeGraphProbeByConsumers(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	want := insp.Runtime.CodeIntelligence

	report := doctor.Evaluate(insp)
	_ = screens.StatusWithReport(insp, report)
	_ = screens.Doctor(report, insp)
	_, _ = initplan.Build(root, insp)
	_ = runtime.BuildRuntimeRepairPlan(root, insp.Runtime)

	if insp.Runtime.CodeIntelligence != want {
		t.Fatal("consumers must not alter Code Intelligence snapshot")
	}
}

func TestInspection_WorkspaceFacadeCompatible(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	viaInspect, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	viaWorkspace, err := workspace.Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if viaInspect.RootPath != viaWorkspace.RootPath {
		t.Fatal("facade root mismatch")
	}
	if viaInspect.Git.IsRepo != viaWorkspace.Git.IsRepo {
		t.Fatal("facade git mismatch")
	}
	if viaInspect.Runtime.ConfigExists != viaWorkspace.Runtime.ConfigExists {
		t.Fatal("facade runtime mismatch")
	}
	if viaInspect.Runtime.CodeIntelligence != viaWorkspace.Runtime.CodeIntelligence {
		t.Fatal("facade CI mismatch")
	}
}

func TestInspection_ProjectInspectOncePerCompose(t *testing.T) {
	// Inspect calls project.Inspect once then EvaluateHealth with that
	// snapshot's Atlas/Files — no second project.Inspect inside runtime.
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := project.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	health := runtime.EvaluateHealth(snap.RootPath, snap.Atlas, snap.Files)
	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if insp.RootPath != snap.RootPath || insp.Files.HasReadme != snap.Files.HasReadme {
		t.Fatal("composed inspection must reuse project facts shape")
	}
	if insp.Runtime.ConfigExists != health.ConfigExists ||
		insp.Runtime.ConfigLoads != health.ConfigLoads {
		t.Fatal("runtime health must evaluate the same project evidence")
	}
}

func treeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		if info.IsDir() {
			out[rel] = "dir"
			return nil
		}
		b, _ := os.ReadFile(path)
		out[rel] = string(b)
		return nil
	})
	return out
}

func assertTreeUnchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := treeSnapshot(t, root)
	if len(before) != len(after) {
		t.Fatalf("tree size %d -> %d", len(before), len(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("mutated %s", k)
		}
	}
}
