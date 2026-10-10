package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

func TestApplyConfig_SkillReconcileThenLaterFailure_RestoresProjectionsAndOwnership_AbsentBaseline(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	root := t.TempDir()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:    "skills-tx-absent",
		ProjectMode:    "new",
		DefaultRemote:  "origin",
		CursorDetected: true,
	})
	if !draft.SetValue("adapters.selected", "cursor") {
		t.Fatal("adapters")
	}

	var sawProjection bool
	config.SetAfterSkillsReconcileHookForTest(func() error {
		skillPath := filepath.Join(root, ".cursor", "skills", "testing", "SKILL.md")
		if _, err := os.Lstat(skillPath); err != nil {
			t.Fatalf("expected projection before later failure: %v", err)
		}
		sawProjection = true
		ownRel := skills.OwnershipRelPath(mustProjectID(t, root, "skills-tx-absent"))
		if _, err := os.Lstat(filepath.Join(homeDir, filepath.FromSlash(ownRel))); err != nil {
			t.Fatalf("expected ownership after skill reconcile: %v", err)
		}
		return fmt.Errorf("injected post-skills failure")
	})
	t.Cleanup(func() { config.SetAfterSkillsReconcileHookForTest(nil) })

	fixed := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	_, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed },
	})
	if err == nil {
		t.Fatal("expected rollback")
	}
	if !sawProjection {
		t.Fatal("hook did not observe skill projection")
	}
	if !strings.Contains(err.Error(), "injected post-skills failure") {
		t.Fatalf("missing cause: %v", err)
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected outer rollback: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".cursor", "skills", "testing", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("projection must be restored absent, err=%v", err)
	}
	ownPath := filepath.Join(homeDir, filepath.FromSlash(skills.OwnershipRelPath(mustProjectID(t, root, "skills-tx-absent"))))
	if _, err := os.Lstat(ownPath); !os.IsNotExist(err) {
		t.Fatalf("ownership must be restored absent, err=%v", err)
	}
}

func TestPersistConfigure_SkillReconcileThenLaterFailure_RestoresProjectionsAndOwnership_ExistingBaseline(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	root := t.TempDir()

	baseDraft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:    "skills-tx-exist",
		ProjectMode:    "new",
		DefaultRemote:  "origin",
		CursorDetected: true,
	})
	if !baseDraft.SetValue("adapters.selected", "cursor") {
		t.Fatal("adapters")
	}
	fixed := time.Date(2026, 10, 9, 18, 10, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: baseDraft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed },
	}); err != nil {
		t.Fatal(err)
	}

	projectID := mustProjectID(t, root, "skills-tx-exist")
	ownRel := skills.OwnershipRelPath(projectID)
	ownPath := filepath.Join(homeDir, filepath.FromSlash(ownRel))
	beforeOwn, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".cursor", "skills", "testing", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "skills", "testing", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatal("opencode skills must be absent before configure")
	}

	cfgDraft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName:      "skills-tx-exist",
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: true,
	})
	if !cfgDraft.SetValue("adapters.selected", "cursor,opencode") {
		t.Fatal("adapters")
	}

	config.SetAfterSkillsReconcileHookForTest(func() error {
		if _, err := os.Lstat(filepath.Join(root, ".opencode", "skills", "testing", "SKILL.md")); err != nil {
			t.Fatalf("expected opencode projection before failure: %v", err)
		}
		return fmt.Errorf("injected configure post-skills failure")
	})
	t.Cleanup(func() { config.SetAfterSkillsReconcileHookForTest(nil) })

	_, err = config.PersistConfigure(config.ApplyInput{
		Root:  root,
		Draft: cfgDraft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed.Add(time.Minute) },
	})
	if err == nil {
		t.Fatal("expected configure rollback")
	}
	if !strings.Contains(err.Error(), "injected configure post-skills failure") {
		t.Fatalf("missing cause: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "skills", "testing", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("opencode projection must be rolled back, err=%v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".cursor", "skills", "testing", "SKILL.md")); err != nil {
		t.Fatalf("cursor projection must remain: %v", err)
	}
	afterOwn, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterOwn) != string(beforeOwn) {
		t.Fatalf("ownership not restored exactly\nbefore=%s\nafter=%s", beforeOwn, afterOwn)
	}
}

func mustProjectID(t *testing.T, root, name string) string {
	t.Helper()
	id, err := home.ProjectID(root, name)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
