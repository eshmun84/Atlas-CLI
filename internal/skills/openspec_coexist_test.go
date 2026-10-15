package skills_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestReconcile_PreservesOpenSpecSkillsAlongsideAtlas(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "Coexist", ProjectMode: "existing",
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	fixed := time.Date(2026, 10, 14, 23, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// OpenSpec-owned skill projection (as produced by openspec init).
	osSkill := filepath.Join(root, ".cursor", "skills", "openspec-propose")
	if err := os.MkdirAll(osSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	osBody := []byte("---\nname: openspec-propose\ndescription: provider\n---\nprovider-owned\n")
	if err := os.WriteFile(filepath.Join(osSkill, "SKILL.md"), osBody, 0o644); err != nil {
		t.Fatal(err)
	}
	// User-owned skill (not an Atlas catalog id) that must coexist with OpenSpec projections.
	userSkill := filepath.Join(root, ".cursor", "skills", "user-custom-review")
	if err := os.MkdirAll(userSkill, 0o755); err != nil {
		t.Fatal(err)
	}
	userBody := []byte("---\nname: user-custom-review\ndescription: user\n---\nuser-owned\n")
	if err := os.WriteFile(filepath.Join(userSkill, "SKILL.md"), userBody, 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	res, err := config.ReconcileSkillProjections(root, doc, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocked {
		t.Fatalf("reconcile blocked: %v", res.Errors)
	}

	gotOS, err := os.ReadFile(filepath.Join(osSkill, "SKILL.md"))
	if err != nil || string(gotOS) != string(osBody) {
		t.Fatalf("OpenSpec skill mutated: %q err=%v", gotOS, err)
	}
	gotUser, err := os.ReadFile(filepath.Join(userSkill, "SKILL.md"))
	if err != nil || string(gotUser) != string(userBody) {
		t.Fatalf("user skill mutated: %q err=%v", gotUser, err)
	}
}
