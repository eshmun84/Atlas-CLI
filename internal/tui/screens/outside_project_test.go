package screens_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

func TestStatusDoctor_OutsideProjectCopyAndReadOnly(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "missing-home")
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	beforeHomeParent, err := os.ReadDir(filepath.Dir(homeDir))
	if err != nil {
		t.Fatal(err)
	}
	beforeRoot, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}

	result, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(result)
	status := screens.StatusWithReport(result, report)
	doc := screens.Doctor(report, result)

	for _, view := range []string{status, doc} {
		for _, want := range []string{
			"Not an Atlas project",
			"Run `atlas init` to initialize this project.",
			"Run `atlas help` for available commands.",
		} {
			if !strings.Contains(view, want) {
				t.Fatalf("missing %q in:\n%s", want, view)
			}
		}
	}

	afterHomeParent, err := os.ReadDir(filepath.Dir(homeDir))
	if err != nil {
		t.Fatal(err)
	}
	if len(afterHomeParent) != len(beforeHomeParent) {
		t.Fatal("Status/Doctor must not create Atlas Home")
	}
	if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
		t.Fatal("Atlas Home created as side effect")
	}
	afterRoot, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterRoot) != len(beforeRoot) {
		t.Fatal("workspace mutated by Status/Doctor")
	}
}
