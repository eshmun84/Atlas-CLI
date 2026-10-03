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

	if !strings.Contains(screens.Home(0), "Init / Setup") {
		t.Fatal("home")
	}
	if !strings.Contains(screens.Help(), "atlas start") {
		t.Fatal("help")
	}
	if !strings.Contains(screens.Error("start"), "start") {
		t.Fatal("error")
	}
	if !strings.Contains(screens.Status(workspace.DiscoveryResult{RootPath: "/tmp"}), "Workspace") {
		t.Fatal("status")
	}
	if !strings.Contains(screens.Doctor(doctor.Report{}), "Checks") {
		t.Fatal("doctor")
	}
	if !strings.Contains(screens.InitPlan(initplan.Plan{ProjectName: "demo"}), "Project") {
		t.Fatal("init")
	}
}
