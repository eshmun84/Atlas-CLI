package screens_test

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestScreenTitles(t *testing.T) {
	t.Parallel()

	if !strings.Contains(screens.Help(), "Atlas Help") {
		t.Fatal("help")
	}
	if !strings.Contains(screens.Status(workspace.DiscoveryResult{RootPath: "/tmp"}), "Atlas Status") {
		t.Fatal("status")
	}
	if !strings.Contains(screens.Doctor(doctor.Report{}), "Atlas Doctor") {
		t.Fatal("doctor")
	}
	if !strings.Contains(screens.InitPlan(initplan.Plan{ProjectName: "demo"}), "Atlas Init Plan") {
		t.Fatal("init")
	}
}
