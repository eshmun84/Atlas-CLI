package sdd_test

import (
	"fmt"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/sdd"
)

type stubEngine struct {
	id        sdd.EngineID
	label     string
	present   bool
	detectErr error
	active    []sdd.ChangeRef
	listIss   []sdd.Issue
	listErr   error
	listed    bool
}

func (s *stubEngine) ID() sdd.EngineID { return s.id }

func (s *stubEngine) Detect(req sdd.InspectRequest) (sdd.Presence, error) {
	if s.detectErr != nil {
		return sdd.Presence{}, s.detectErr
	}
	return sdd.Presence{
		Engine:  s.id,
		Present: s.present,
		Label:   s.label,
	}, nil
}

func (s *stubEngine) ListChanges(req sdd.InspectRequest) (active, archived []sdd.ChangeRef, issues []sdd.Issue, err error) {
	s.listed = true
	return append([]sdd.ChangeRef(nil), s.active...), nil, append([]sdd.Issue(nil), s.listIss...), s.listErr
}

func (s *stubEngine) InspectChange(req sdd.InspectRequest, changeID string) (sdd.ChangeRef, error) {
	return sdd.ChangeRef{}, fmt.Errorf("stub: not found")
}

func TestEvaluate_ZeroEnginesPresent(t *testing.T) {
	a := &stubEngine{id: "alpha", label: "Alpha", present: false}
	b := &stubEngine{id: "beta", label: "Beta", present: false}
	ov := sdd.NewService(a, b).Evaluate(sdd.InspectRequest{Root: t.TempDir()})
	if ov.HasEngine() || ov.EngineConflict {
		t.Fatalf("overview = %#v", ov)
	}
	if a.listed || b.listed {
		t.Fatal("ListChanges must not run when no engine is present")
	}
	lines := sdd.StatusLines(ov)
	if len(lines) != 1 || lines[0] != "SDD: none" {
		t.Fatalf("StatusLines = %#v", lines)
	}
}

func TestEvaluate_ExactlyOneEnginePresent(t *testing.T) {
	a := &stubEngine{
		id: "alpha", label: "Alpha", present: true,
		active: []sdd.ChangeRef{{ChangeID: "c1", Engine: "alpha", Lifecycle: sdd.LifecycleActive}},
	}
	b := &stubEngine{id: "beta", label: "Beta", present: false}
	// Reverse registration order must not matter when only one is present.
	ov := sdd.NewService(b, a).Evaluate(sdd.InspectRequest{Root: t.TempDir(), ProjectID: "p"})
	if !ov.HasEngine() || ov.EngineConflict {
		t.Fatalf("overview = %#v", ov)
	}
	if ov.Presence.Engine != "alpha" || ov.Presence.Label != "Alpha" {
		t.Fatalf("presence = %#v", ov.Presence)
	}
	if !a.listed || b.listed {
		t.Fatalf("listed: a=%v b=%v", a.listed, b.listed)
	}
	if len(ov.Active) != 1 || ov.Active[0].ChangeID != "c1" {
		t.Fatalf("active = %#v", ov.Active)
	}
}

func TestEvaluate_MultipleEnginesPresent_FailClosed(t *testing.T) {
	a := &stubEngine{
		id: "alpha", label: "Alpha", present: true,
		active: []sdd.ChangeRef{{ChangeID: "from-a"}},
	}
	b := &stubEngine{
		id: "beta", label: "Beta", present: true,
		active: []sdd.ChangeRef{{ChangeID: "from-b"}},
	}
	// Registration order must not select a winner.
	for _, engines := range [][]sdd.Engine{
		{a, b},
		{b, a},
	} {
		a.listed, b.listed = false, false
		ov := sdd.NewService(engines...).Evaluate(sdd.InspectRequest{Root: t.TempDir()})
		if ov.HasEngine() {
			t.Fatalf("must not select an engine: %#v", ov.Presence)
		}
		if !ov.EngineConflict {
			t.Fatalf("expected EngineConflict, got %#v", ov)
		}
		if ov.Presence.Present || ov.Presence.Engine != "" {
			t.Fatalf("presence must be empty on conflict: %#v", ov.Presence)
		}
		if len(ov.Active) != 0 || len(ov.Archived) != 0 {
			t.Fatalf("must not list changes on conflict: active=%#v archived=%#v", ov.Active, ov.Archived)
		}
		if a.listed || b.listed {
			t.Fatal("ListChanges must not run on multi-engine conflict")
		}
		found := false
		for _, iss := range ov.Issues {
			if iss.Kind == sdd.IssueAmbiguous {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected ambiguous issue, got %#v", ov.Issues)
		}
		lines := sdd.StatusLines(ov)
		if lines[0] != "SDD: ambiguous" {
			t.Fatalf("StatusLines = %#v", lines)
		}
	}
}

func TestReadyDoesNotImplyVerified(t *testing.T) {
	if sdd.LifecycleReady == sdd.LifecycleVerified {
		t.Fatal("ready must not equal verified")
	}
	// Semantic guard: readiness is a lifecycle phase before verification.
	if sdd.NormalizeLifecycle("ready") != sdd.LifecycleReady {
		t.Fatal("ready normalization")
	}
	if sdd.NormalizeLifecycle("verified") != sdd.LifecycleVerified {
		t.Fatal("verified normalization")
	}
}

func TestEvaluate_DetectUnsafePreservedWhenNotSelected(t *testing.T) {
	eng := &stubEngine{
		id:        "demo",
		detectErr: fmt.Errorf("fsafety: refusing symlink path component openspec"),
	}
	ov := sdd.NewService(eng).Evaluate(sdd.InspectRequest{Root: t.TempDir()})
	if ov.HasEngine() {
		t.Fatal("engine must not be selected on detect error")
	}
	if len(ov.Issues) != 1 || ov.Issues[0].Kind != sdd.IssueUnsafe {
		t.Fatalf("issues = %#v", ov.Issues)
	}
}

func TestEvaluate_ListChangesIssuesPreservedWithError(t *testing.T) {
	eng := &stubEngine{
		id:      "demo",
		label:   "Demo",
		present: true,
		active:  []sdd.ChangeRef{{ChangeID: "should-drop"}},
		listIss: []sdd.Issue{{Kind: sdd.IssueUnsafe, Message: "refusing symlink openspec/changes"}},
		listErr: fmt.Errorf("openspec: list failed after symlink"),
	}
	ov := sdd.NewService(eng).Evaluate(sdd.InspectRequest{Root: t.TempDir()})
	if !ov.HasEngine() {
		t.Fatal("engine was detected present")
	}
	if len(ov.Active) != 0 {
		t.Fatalf("partial lists must be discarded on error: %#v", ov.Active)
	}
	if len(ov.Issues) < 2 {
		t.Fatalf("expected issue + list error, got %#v", ov.Issues)
	}
	if ov.Issues[0].Kind != sdd.IssueUnsafe || ov.Issues[0].Message != "refusing symlink openspec/changes" {
		t.Fatalf("first issue lost/degraded: %#v", ov.Issues[0])
	}
	if ov.Issues[1].Kind != sdd.IssueUnsafe {
		t.Fatalf("list error must classify as unsafe, got %#v", ov.Issues[1])
	}
}
