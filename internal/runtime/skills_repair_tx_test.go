package runtime_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

func TestApplyRuntimeRepair_SkillReconcileThenLaterFailure_RestoresProjectionsAndOwnership(t *testing.T) {
	root := materializeProject(t, []string{"cursor"}, true)
	homePath, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	projectID, err := home.ProjectID(root, "demo")
	if err != nil {
		t.Fatal(err)
	}
	ownPath := filepath.Join(homePath, filepath.FromSlash(skills.OwnershipRelPath(projectID)))
	beforeOwn, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}

	skillRoot := filepath.Join(root, ".cursor", "skills", "testing")
	skillMD := filepath.Join(skillRoot, "SKILL.md")
	if _, err := os.Lstat(skillMD); err != nil {
		t.Fatal(err)
	}
	// Remove whole package so health reports missing (not invalid).
	if err := os.RemoveAll(skillRoot); err != nil {
		t.Fatal(err)
	}

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	var sawSkillTarget bool
	for _, target := range plan.Targets {
		if target.Kind == runtime.RepairKindSkill {
			sawSkillTarget = true
			break
		}
	}
	if !sawSkillTarget {
		t.Fatalf("expected skill repair target: %#v", plan.Targets)
	}

	runtime.SetRepairAfterSkillsHookForTest(func() error {
		if _, err := os.Lstat(skillMD); err != nil {
			t.Fatalf("expected skill restored by reconcile before later failure: %v", err)
		}
		return fmt.Errorf("injected repair post-skills failure")
	})
	t.Cleanup(func() { runtime.SetRepairAfterSkillsHookForTest(nil) })

	fixed := time.Date(2026, 10, 9, 19, 0, 0, 0, time.UTC)
	_, err = runtime.ApplyRuntimeRepair(root, plan.Signature(), func() time.Time { return fixed })
	if err == nil {
		t.Fatal("expected repair rollback")
	}
	if !strings.Contains(err.Error(), "injected repair post-skills failure") {
		t.Fatalf("missing cause: %v", err)
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected outer rollback: %v", err)
	}

	// Pre-repair baseline: skill file was deleted. Reconcile recreated it; outer
	// rollback must restore absence.
	if _, err := os.Lstat(skillMD); !os.IsNotExist(err) {
		t.Fatalf("skill projection must be absent after rollback, err=%v", err)
	}

	afterOwn, err := os.ReadFile(ownPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(afterOwn) != string(beforeOwn) {
		t.Fatalf("skills ownership not restored exactly\nbefore=%s\nafter=%s", beforeOwn, afterOwn)
	}
}
