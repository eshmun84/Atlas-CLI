package context_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestProjectID_Stable(t *testing.T) {
	root := t.TempDir()
	a, err := atlascontext.ProjectID(root, "Demo App")
	if err != nil {
		t.Fatal(err)
	}
	b, err := atlascontext.ProjectID(root, "Demo App")
	if err != nil {
		t.Fatal(err)
	}
	if a != b || !strings.HasPrefix(a, "demo-app-") {
		t.Fatalf("id unstable or unexpected: %q %q", a, b)
	}
}

func TestPaths_UnderAtlasHome(t *testing.T) {
	homePath := "/tmp/atlas-home-x"
	id := "demo-abc"
	if got := atlascontext.IndexPath(homePath, id); got != filepath.Join(homePath, "projects", id, "context", "index.yaml") {
		t.Fatalf("index path = %q", got)
	}
	if got := atlascontext.CapsulePath(homePath, id); !strings.HasSuffix(got, filepath.Join("projects", id, "context", "capsule.md")) {
		t.Fatalf("capsule path = %q", got)
	}
	if got := atlascontext.PackPath(homePath, id, "orient"); !strings.Contains(got, filepath.Join("packs", "orient.yaml")) {
		t.Fatalf("pack path = %q", got)
	}
}

func TestApplyUpdate_WritesUnderHomeProjectContext(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module demo\n")
	state := config.StateDocument{Initialized: true, ProjectName: "demo"}
	plan := atlascontext.BuildUpdatePlan(root, true, state, atlascontext.DefaultPackObjective)
	result, err := atlascontext.ApplyUpdate(root, plan.Signature(), state, atlascontext.DefaultPackObjective, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := atlascontext.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(homeDir, "projects", id, "context")
	if !strings.HasPrefix(result.CapsulePath, wantDir) {
		t.Fatalf("capsule = %q want under %q", result.CapsulePath, wantDir)
	}
	if _, err := os.Stat(filepath.Join(root, "context")); !os.IsNotExist(err) {
		t.Fatal("must not write context/ into product repo")
	}
	if _, err := os.Stat(filepath.Join(homeDir, "context", "projects", id)); !os.IsNotExist(err) {
		t.Fatal("must not write legacy context/projects layout for new updates")
	}
}

func TestBuildIndex_IgnoresNoiseAndIndexesAtlas(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module demo\n")
	write(t, filepath.Join(root, "README.md"), "# demo\n")
	write(t, filepath.Join(root, "main.go"), "package main\n")
	write(t, filepath.Join(root, "main_test.go"), "package main\n")
	write(t, filepath.Join(root, "AGENTS.md"), "# agents\n")
	write(t, filepath.Join(root, ".atlas", "contracts", "sdd-openspec.md"), "# contract\n")
	write(t, filepath.Join(root, "node_modules", "x", "index.js"), "noise\n")
	write(t, filepath.Join(root, ".git", "HEAD"), "ref\n")
	write(t, filepath.Join(root, "vendor", "pkg", "x.go"), "package x\n")
	write(t, filepath.Join(root, ".atlas", "backups", "old", "AGENTS.md"), "old\n")
	big := make([]byte, atlascontext.MaxFileBytes+10)
	writeBytes(t, filepath.Join(root, "huge.bin"), big)

	idx, err := atlascontext.BuildIndex(root, "demo", time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if idx.ProjectID == "" || idx.Fingerprint == "" || idx.IndexedAt == "" {
		t.Fatalf("incomplete index %#v", idx)
	}
	paths := map[string]bool{}
	for _, f := range idx.Files {
		paths[f.Path] = true
	}
	for _, want := range []string{"go.mod", "README.md", "main.go", "AGENTS.md", ".atlas/contracts/sdd-openspec.md"} {
		if !paths[want] {
			t.Fatalf("missing indexed path %s in %#v", want, paths)
		}
	}
	for _, banned := range []string{"node_modules/x/index.js", ".git/HEAD", "vendor/pkg/x.go", ".atlas/backups/old/AGENTS.md", "huge.bin"} {
		if paths[banned] {
			t.Fatalf("should ignore %s", banned)
		}
	}
	if len(idx.Languages) == 0 || idx.Languages[0] != "Go" {
		t.Fatalf("languages = %#v", idx.Languages)
	}
	if len(idx.Contracts) == 0 {
		t.Fatal("expected contracts")
	}
}

func TestCapsuleAndPack_Deterministic(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module demo\n")
	write(t, filepath.Join(root, "AGENTS.md"), "# agents\n")
	write(t, filepath.Join(root, ".atlas", "contracts", "sdd-openspec.md"), "# c\n")
	fixed := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	idx, err := atlascontext.BuildIndex(root, "demo", fixed)
	if err != nil {
		t.Fatal(err)
	}
	c1 := atlascontext.RenderCapsule(idx)
	c2 := atlascontext.RenderCapsule(idx)
	if c1 != c2 || !strings.Contains(c1, "Atlas Context Capsule") || !strings.Contains(c1, idx.ProjectID) {
		t.Fatalf("capsule unexpected:\n%s", c1)
	}
	p1 := atlascontext.BuildPack(idx, "implement SDD verify tests", fixed)
	p2 := atlascontext.BuildPack(idx, "implement SDD verify tests", fixed)
	if p1.PackID != p2.PackID || len(p1.Candidates) == 0 {
		t.Fatalf("pack %#v", p1)
	}
	y1, err := atlascontext.RenderPackYAML(p1)
	if err != nil {
		t.Fatal(err)
	}
	y2, err := atlascontext.RenderPackYAML(p2)
	if err != nil {
		t.Fatal(err)
	}
	if y1 != y2 || !strings.Contains(y1, "objective:") {
		t.Fatalf("pack yaml mismatch:\n%s", y1)
	}
	foundReason := false
	for _, c := range p1.Candidates {
		if c.Path == "" || c.Reason == "" {
			t.Fatalf("candidate incomplete %#v", c)
		}
		foundReason = true
	}
	if !foundReason {
		t.Fatal("no candidates")
	}
}

func TestInspect_ReadOnlyAndStale(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module demo\n")
	write(t, filepath.Join(root, "AGENTS.md"), "# a\n")

	beforeHome, _ := os.ReadDir(homeDir)
	snap := atlascontext.Inspect(atlascontext.InspectInput{
		Root: root, Initialized: true, ProjectName: "demo",
	})
	if snap.State != atlascontext.StatusMissing {
		t.Fatalf("want missing, got %#v", snap)
	}
	afterHome, _ := os.ReadDir(homeDir)
	if len(afterHome) != len(beforeHome) {
		t.Fatal("Inspect mutated Atlas Home")
	}

	state := config.StateDocument{Initialized: true, ProjectName: "demo"}
	plan := atlascontext.BuildUpdatePlan(root, true, state, atlascontext.DefaultPackObjective)
	if !plan.NeedsApply() {
		t.Fatalf("plan %#v", plan)
	}
	result, err := atlascontext.ApplyUpdate(root, plan.Signature(), state, atlascontext.DefaultPackObjective, func() time.Time {
		return time.Date(2026, 10, 6, 13, 0, 0, 0, time.UTC)
	})
	if err != nil || result.Noop {
		t.Fatalf("apply %#v err=%v", result, err)
	}
	if _, err := os.Stat(result.CapsulePath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "context")); !os.IsNotExist(err) {
		t.Fatal("must not write context/ into product repo")
	}
	stateRaw, err := os.ReadFile(filepath.Join(root, config.FileState))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(stateRaw), "context_economy_project_id:") {
		t.Fatalf("state missing refs:\n%s", stateRaw)
	}

	fresh := atlascontext.InspectFromState(root, true, config.StateDocument{
		Initialized:             true,
		ProjectName:             "demo",
		ContextEconomyProjectID: result.Index.ProjectID,
		ContextEconomyUpdatedAt: "2026-10-06T13:00:00Z",
	})
	if fresh.State != atlascontext.StatusPresent {
		t.Fatalf("want present got %#v", fresh)
	}

	write(t, filepath.Join(root, "extra.go"), "package main\n")
	stale := atlascontext.InspectFromState(root, true, config.StateDocument{
		Initialized:             true,
		ProjectName:             "demo",
		ContextEconomyProjectID: result.Index.ProjectID,
	})
	if stale.State != atlascontext.StatusStale {
		t.Fatalf("want stale got %#v", stale)
	}
}

func TestApplyUpdate_DoesNotTouchDeveloperAgents(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	write(t, filepath.Join(root, "go.mod"), "module demo\n")
	write(t, filepath.Join(root, ".cursor", "agents", "external.md"), "keep\n")
	state := config.StateDocument{Initialized: true, ProjectName: "demo", RuntimeMaterialized: true}
	plan := atlascontext.BuildUpdatePlan(root, true, state, "fix agents")
	_, err := atlascontext.ApplyUpdate(root, plan.Signature(), state, "fix agents", nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, ".cursor", "agents", "external.md"))
	if err != nil || string(got) != "keep\n" {
		t.Fatalf("developer agent touched: %q err=%v", got, err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	writeBytes(t, path, []byte(body))
}

func writeBytes(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
