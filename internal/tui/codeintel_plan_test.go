package tui_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
)

func TestBuildCodeIntelPlan_ProjectIDFailure(t *testing.T) {
	tui.SetCodeIntelProjectIDForTest(func(string, string) (string, error) {
		return "", errors.New("injected identity failure")
	})
	t.Cleanup(func() { tui.SetCodeIntelProjectIDForTest(nil) })

	plan := tui.BuildCodeIntelPlanForTest(inspect.Inspection{RootPath: t.TempDir()}, false)
	if !plan.Blocked {
		t.Fatalf("expected blocked plan: %#v", plan)
	}
	if len(plan.Blockers) == 0 || !strings.Contains(plan.Blockers[0], "project identity unavailable") {
		t.Fatalf("blockers = %#v", plan.Blockers)
	}
}

func TestBuildCodeIntelPlan_FingerprintFailure(t *testing.T) {
	tui.SetCodeIntelFingerprintForTest(func(string) (codeintel.SourceFingerprint, error) {
		return codeintel.SourceFingerprint{}, errors.New("injected fingerprint failure")
	})
	t.Cleanup(func() { tui.SetCodeIntelFingerprintForTest(nil) })

	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.go"), "package a\n")
	plan := tui.BuildCodeIntelPlanForTest(inspect.Inspection{RootPath: root}, false)
	if !plan.Blocked {
		t.Fatalf("expected blocked plan: %#v", plan)
	}
	if len(plan.Blockers) == 0 || !strings.Contains(plan.Blockers[0], "source fingerprint unavailable") {
		t.Fatalf("blockers = %#v", plan.Blockers)
	}
}

func TestBuildCodeIntelPlan_Healthy(t *testing.T) {
	root := t.TempDir()
	homeDir := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "a.go"), "package a\n")
	plan := tui.BuildCodeIntelPlanForTest(inspect.Inspection{
		RootPath: root,
		Runtime: runtime.Health{
			Home: home.Status{Path: homeDir, Exists: true},
			CodeIntelligence: codeintel.Snapshot{
				Provider:     codeintel.ProviderCodeGraph,
				State:        codeintel.StateAvailable,
				Version:      "3.17.0",
				GraphPresent: false,
			},
		},
	}, false)
	if plan.Blocked {
		t.Fatalf("healthy flow must not block: %#v", plan)
	}
	if plan.Fingerprint == "" || plan.ProjectID == "" || plan.Mode == "" {
		t.Fatalf("expected planned refresh: %#v", plan)
	}
}

func mustWriteFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
