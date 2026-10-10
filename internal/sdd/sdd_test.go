package sdd_test

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/sdd"
)

func TestNormalizeLifecycle_FailClosed(t *testing.T) {
	cases := []struct {
		in   string
		want sdd.LifecycleState
	}{
		{"active", sdd.LifecycleActive},
		{"READY", sdd.LifecycleReady},
		{"verified", sdd.LifecycleVerified},
		{"archived", sdd.LifecycleArchived},
		{"incomplete", sdd.LifecycleIncomplete},
		{"unknown", sdd.LifecycleUnknown},
		{"", sdd.LifecycleUnknown},
		{"success", sdd.LifecycleUnknown},
	}
	for _, tc := range cases {
		if got := sdd.NormalizeLifecycle(tc.in); got != tc.want {
			t.Fatalf("NormalizeLifecycle(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestStatusLines(t *testing.T) {
	cases := []struct {
		name string
		ov   sdd.Overview
		want []string
	}{
		{
			name: "none",
			ov:   sdd.Overview{},
			want: []string{"SDD: none"},
		},
		{
			name: "one active",
			ov: sdd.Overview{
				Presence: sdd.Presence{Engine: "demo", Present: true, Label: "Demo"},
				Active:   []sdd.ChangeRef{{ChangeID: "add-x"}},
			},
			want: []string{"SDD: Demo", "Active change: add-x"},
		},
		{
			name: "many active",
			ov: sdd.Overview{
				Presence: sdd.Presence{Engine: "demo", Present: true, Label: "Demo"},
				Active:   []sdd.ChangeRef{{ChangeID: "a"}, {ChangeID: "b"}},
			},
			want: []string{"SDD: Demo", "Active changes: 2"},
		},
		{
			name: "engine no active",
			ov: sdd.Overview{
				Presence: sdd.Presence{Engine: "demo", Present: true, Label: "Demo"},
			},
			want: []string{"SDD: Demo", "Active change: none"},
		},
		{
			name: "engine conflict",
			ov: sdd.Overview{
				EngineConflict: true,
				// Presence must be ignored when conflict is set.
				Presence: sdd.Presence{Engine: "demo", Present: true, Label: "Demo"},
				Active:   []sdd.ChangeRef{{ChangeID: "should-not-show"}},
			},
			want: []string{"SDD: ambiguous", "Active change: none"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sdd.StatusLines(tc.ov)
			if len(got) != len(tc.want) {
				t.Fatalf("got %#v want %#v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("line %d: got %q want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestClosedChangeFrom_AndSourceLabel(t *testing.T) {
	at := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	ref := sdd.ChangeRef{
		ProjectID:    "proj-abc",
		Engine:       "demo",
		ChangeID:     "add-oauth",
		Lifecycle:    sdd.LifecycleArchived,
		Location:     "engine://archive/2026-01-15-add-oauth",
		Archived:     true,
		ArchivedAt:   &at,
		Verification: sdd.VerificationUnknown,
		SpecRefs:     []sdd.SpecRef{{Name: "auth", Ref: "specs/auth"}},
	}
	closed, err := sdd.ClosedChangeFrom(ref)
	if err != nil {
		t.Fatal(err)
	}
	if closed.ProjectID != "proj-abc" || closed.ChangeID != "add-oauth" || closed.Engine != "demo" {
		t.Fatalf("closed = %#v", closed)
	}
	if closed.ArchivedAt == nil || !closed.ArchivedAt.Equal(at) {
		t.Fatalf("ArchivedAt = %v want %v", closed.ArchivedAt, at)
	}
	if closed.SourceLabel() != "archived demo change add-oauth" {
		t.Fatalf("SourceLabel = %q", closed.SourceLabel())
	}

	_, err = sdd.ClosedChangeFrom(sdd.ChangeRef{ChangeID: "x", Lifecycle: sdd.LifecycleActive})
	if err == nil {
		t.Fatal("expected error for non-archived")
	}
}

func TestClosedChangeFrom_FailClosedRequirements(t *testing.T) {
	valid := sdd.ChangeRef{
		Archived:     true,
		Lifecycle:    sdd.LifecycleArchived,
		Engine:       "demo",
		ChangeID:     "add-x",
		Location:     "engine://archive/add-x",
		Verification: sdd.VerificationUnknown,
	}
	if _, err := sdd.ClosedChangeFrom(valid); err != nil {
		t.Fatalf("valid archived ref: %v", err)
	}

	cases := []struct {
		name string
		ref  sdd.ChangeRef
	}{
		{
			name: "archived true lifecycle unknown",
			ref: sdd.ChangeRef{
				Archived: true, Lifecycle: sdd.LifecycleUnknown,
				Engine: "demo", ChangeID: "x", Location: "loc",
			},
		},
		{
			name: "archived ambiguous",
			ref: sdd.ChangeRef{
				Archived: true, Lifecycle: sdd.LifecycleArchived, Ambiguous: true,
				Engine: "demo", ChangeID: "x", Location: "loc",
			},
		},
		{
			name: "archived without location",
			ref: sdd.ChangeRef{
				Archived: true, Lifecycle: sdd.LifecycleArchived,
				Engine: "demo", ChangeID: "x",
			},
		},
		{
			name: "missing engine",
			ref: sdd.ChangeRef{
				Archived: true, Lifecycle: sdd.LifecycleArchived,
				ChangeID: "x", Location: "loc",
			},
		},
		{
			name: "missing change id",
			ref: sdd.ChangeRef{
				Archived: true, Lifecycle: sdd.LifecycleArchived,
				Engine: "demo", Location: "loc",
			},
		},
		{
			name: "lifecycle archived but Archived false",
			ref: sdd.ChangeRef{
				Archived: false, Lifecycle: sdd.LifecycleArchived,
				Engine: "demo", ChangeID: "x", Location: "loc",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := sdd.ClosedChangeFrom(tc.ref); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestCorePackageHasNoProviderSpecificImportsOrPaths(t *testing.T) {
	dir := "."
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	bannedImport := "github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, needle := range []string{"openspec/", ".openspec", "proposal.md", "tasks.md", "changes/archive"} {
			if strings.Contains(text, needle) {
				t.Errorf("%s contains provider-specific token %q", e.Name(), needle)
			}
		}
		file, err := parser.ParseFile(fset, path, data, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range file.Imports {
			if strings.Trim(imp.Path.Value, `"`) == bannedImport {
				t.Errorf("%s imports %s", e.Name(), bannedImport)
			}
		}
	}
}
