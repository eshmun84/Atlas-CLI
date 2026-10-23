package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestApplyConfig_OpenSpecCLIAbsent_ZeroMutation(t *testing.T) {
	withTempAtlasHome(t)
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty-bin"))
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "NoCLI", ProjectMode: "new", ToolCursorAvailable: true,
	})
	if !draft.ToggleMulti("adapters.selected", "cursor") {
		t.Fatal("select cursor")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "openspec executable not found on PATH") {
		t.Fatalf("expected absent CLI, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(statErr) {
		t.Fatalf(".atlas must stay absent: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "openspec")); !os.IsNotExist(statErr) {
		t.Fatalf("openspec/ must stay absent: %v", statErr)
	}
	readme, readErr := os.ReadFile(filepath.Join(root, "README.md"))
	if readErr != nil || string(readme) != "keep\n" {
		t.Fatalf("readme = %q err=%v", readme, readErr)
	}
}
