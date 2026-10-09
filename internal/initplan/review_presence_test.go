package initplan_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
)

func TestBuildReview_RuntimeSymlinkNotOrdinaryReplace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ext := filepath.Join(outside, "atlas.mdc")
	if err := os.WriteFile(ext, []byte("EXTERNAL_REVIEW\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, filepath.Join(root, filepath.FromSlash(config.FileCursorAtlasMDC))); err != nil {
		t.Fatal(err)
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "review-sym", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	plan := initplan.BuildReview(initplan.ReviewInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
	})

	for _, repl := range plan.Replacements {
		if repl.Path == config.FileCursorAtlasMDC {
			t.Fatalf("symlink must not be ordinary replace target: %#v", plan.Replacements)
		}
	}
	found := false
	for _, b := range plan.Blockers {
		if strings.Contains(b.Message, config.FileCursorAtlasMDC) &&
			(strings.Contains(b.Message, "unsafe") || strings.Contains(b.Message, "symlink")) {
			found = true
			break
		}
	}
	for _, w := range plan.Warnings {
		if strings.Contains(w.Message, config.FileCursorAtlasMDC) &&
			(strings.Contains(w.Message, "unsafe") || strings.Contains(w.Message, "symlink")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected warning/block for unsafe runtime path: blockers=%#v warnings=%#v", plan.Blockers, plan.Warnings)
	}
	got, err := os.ReadFile(ext)
	if err != nil || string(got) != "EXTERNAL_REVIEW\n" {
		t.Fatalf("external mutated: %q err=%v", got, err)
	}
}
