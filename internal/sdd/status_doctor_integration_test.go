package sdd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/sdd"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

func fingerprintTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			out[rel] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestStatusAndDoctor_ReadOnlyWithOpenSpec(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	change := filepath.Join(root, "openspec", "changes", "slice34")
	if err := os.MkdirAll(change, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(change, "proposal.md"), []byte("# p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := fingerprintTree(t, root)

	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if !insp.SDD.HasEngine() || len(insp.SDD.Active) != 1 {
		t.Fatalf("SDD = %#v", insp.SDD)
	}

	report := doctor.Evaluate(insp)
	status := screens.StatusWithReport(insp, report)
	docView := screens.Doctor(report, insp)

	if !strings.Contains(status, "SDD: OpenSpec") || !strings.Contains(status, "Active change: slice34") {
		t.Fatalf("status missing SDD lines:\n%s", status)
	}
	if !strings.Contains(docView, "SDD") || !strings.Contains(docView, "sdd engine") {
		t.Fatalf("doctor missing SDD section:\n%s", docView)
	}

	after := fingerprintTree(t, root)
	if len(before) != len(after) {
		t.Fatalf("filesystem mutated: before=%d after=%d", len(before), len(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("mutated %s", k)
		}
	}
}

func TestStatus_NoOpenSpec(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	status := screens.Status(insp)
	if !strings.Contains(status, "SDD: none") {
		t.Fatalf("status:\n%s", status)
	}
	report := doctor.Evaluate(insp)
	found := false
	for _, c := range report.Checks {
		if c.Name == "sdd engine" && strings.Contains(c.Message, "none") {
			found = true
		}
	}
	if !found {
		t.Fatalf("doctor checks missing sdd none: %#v", report.Checks)
	}
}

func TestDoctor_MalformedAndUnsafe(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	report := doctor.Evaluate(insp)
	foundIncomplete := false
	for _, c := range report.Checks {
		if strings.Contains(c.Name, "sdd") && (c.Severity == doctor.SeverityWarn || c.Severity == doctor.SeverityFail) {
			foundIncomplete = true
		}
	}
	if !foundIncomplete {
		t.Fatalf("expected sdd warn/fail for empty change, checks=%#v", report.Checks)
	}
}

func TestDoctor_OpenSpecRootSymlink_FailUnsafe(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "changes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "openspec")); err != nil {
		t.Fatal(err)
	}

	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if insp.SDD.HasEngine() {
		t.Fatalf("engine must not be selected for symlink root: %#v", insp.SDD)
	}
	if len(insp.SDD.Issues) == 0 || insp.SDD.Issues[0].Kind != sdd.IssueUnsafe {
		t.Fatalf("expected unsafe issue on overview: %#v", insp.SDD.Issues)
	}

	report := doctor.Evaluate(insp)
	if !hasDoctorCheck(report, doctor.SeverityFail, "sdd unsafe path") {
		t.Fatalf("expected Doctor FAIL sdd unsafe path, checks=%#v", report.Checks)
	}
}

func TestDoctor_OpenSpecChangesSymlink_FailUnsafe(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(outside, "add-x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "openspec", "changes")); err != nil {
		t.Fatal(err)
	}

	insp, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	foundUnsafe := false
	for _, iss := range insp.SDD.Issues {
		if iss.Kind == sdd.IssueUnsafe {
			foundUnsafe = true
		}
	}
	if !foundUnsafe {
		t.Fatalf("expected unsafe issue for changes symlink: %#v", insp.SDD)
	}

	report := doctor.Evaluate(insp)
	if !hasDoctorCheck(report, doctor.SeverityFail, "sdd unsafe path") {
		t.Fatalf("expected Doctor FAIL sdd unsafe path, checks=%#v", report.Checks)
	}
}

func hasDoctorCheck(report doctor.Report, sev doctor.Severity, name string) bool {
	for _, c := range report.Checks {
		if c.Severity == sev && c.Name == name {
			return true
		}
	}
	return false
}
