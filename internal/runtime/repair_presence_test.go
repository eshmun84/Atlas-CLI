package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

func TestBuildRuntimeRepairPlan_UnselectedAdapterSymlinkNotQuarantine(t *testing.T) {
	root := materializeProject(t, []string{"opencode"}, true)
	outside := t.TempDir()
	ext := filepath.Join(outside, "atlas.mdc")
	if err := os.WriteFile(ext, []byte("EXTERNAL_CURSOR_RULE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, filepath.FromSlash(config.FileCursorAtlasMDC))
	if err := os.Symlink(ext, link); err != nil {
		t.Fatal(err)
	}

	plan := runtime.BuildRuntimeRepairPlan(root, mustDiscover(t, root).Runtime)
	for _, q := range plan.Quarantines {
		if q == config.FileCursorAtlasMDC {
			t.Fatalf("symlink must not be ordinary quarantine target: %#v", plan.Quarantines)
		}
	}
	if !plan.Blocked {
		t.Fatalf("expected blocked plan for unsafe Atlas-owned path: %#v", plan)
	}
	found := false
	for _, b := range plan.Blockers {
		if strings.Contains(b, config.FileCursorAtlasMDC) &&
			(strings.Contains(b, "unsafe") || strings.Contains(b, "symlink")) {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unsafe/blocking diagnostic, blockers=%#v", plan.Blockers)
	}
	got, err := os.ReadFile(ext)
	if err != nil || string(got) != "EXTERNAL_CURSOR_RULE\n" {
		t.Fatalf("external target mutated: %q err=%v", got, err)
	}
}
