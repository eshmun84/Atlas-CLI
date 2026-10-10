package openspec_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/sdd"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetect_NoOpenSpec(t *testing.T) {
	root := t.TempDir()
	a := openspec.New()
	pres, err := a.Detect(sdd.InspectRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if pres.Present {
		t.Fatal("expected no engine")
	}
	active, archived, issues, err := a.ListChanges(sdd.InspectRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 || len(archived) != 0 || len(issues) != 0 {
		t.Fatalf("unexpected %#v %#v %#v", active, archived, issues)
	}
}

func TestDetect_ValidOpenSpecAndActiveChange(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"openspec/changes/add-rate-limit/proposal.md":       "# proposal\n",
		"openspec/changes/add-rate-limit/tasks.md":          "- [ ] do thing\n",
		"openspec/changes/add-rate-limit/specs/api/spec.md": "## ADDED\n",
	})
	a := openspec.New()
	pres, err := a.Detect(sdd.InspectRequest{Root: root, ProjectID: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	if !pres.Present || pres.Label != "OpenSpec" {
		t.Fatalf("presence = %#v", pres)
	}
	active, archived, issues, err := a.ListChanges(sdd.InspectRequest{Root: root, ProjectID: "p1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 0 {
		t.Fatalf("archived = %#v", archived)
	}
	if len(active) != 1 || active[0].ChangeID != "add-rate-limit" {
		t.Fatalf("active = %#v", active)
	}
	if active[0].Lifecycle != sdd.LifecycleActive {
		t.Fatalf("lifecycle = %s", active[0].Lifecycle)
	}
	if active[0].Engine != "openspec" || active[0].ProjectID != "p1" {
		t.Fatalf("ref = %#v", active[0])
	}
	if len(active[0].SpecRefs) != 1 || active[0].SpecRefs[0].Name != "api" {
		t.Fatalf("spec refs = %#v", active[0].SpecRefs)
	}
	_ = issues
}

func TestDiscover_ArchivedChange(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"openspec/changes/archive/2026-01-15-add-oauth/proposal.md": "# done\n",
		"openspec/changes/archive/2026-01-15-add-oauth/tasks.md":    "- [x] done\n",
	})
	a := openspec.New()
	active, archived, _, err := a.ListChanges(sdd.InspectRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 0 || len(archived) != 1 {
		t.Fatalf("active=%#v archived=%#v", active, archived)
	}
	ref := archived[0]
	if ref.ChangeID != "add-oauth" || !ref.Archived || ref.Lifecycle != sdd.LifecycleArchived {
		t.Fatalf("ref = %#v", ref)
	}
	if ref.ArchivedAt == nil || ref.ArchivedAt.Format("2006-01-02") != "2026-01-15" {
		t.Fatalf("ArchivedAt = %v", ref.ArchivedAt)
	}
	closed, err := sdd.ClosedChangeFrom(ref)
	if err != nil {
		t.Fatal(err)
	}
	if closed.SourceLabel() != "archived openspec change add-oauth" {
		t.Fatalf("label = %q", closed.SourceLabel())
	}
}

func TestDiscover_MissingOptionalArtifacts(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"openspec/changes/thin-change/proposal.md": "# only proposal\n",
	})
	a := openspec.New()
	active, _, _, err := a.ListChanges(sdd.InspectRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Lifecycle != sdd.LifecycleActive {
		t.Fatalf("active = %#v", active)
	}
}

func TestDiscover_MalformedEmptyChange(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", "empty-change"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := openspec.New()
	active, _, issues, err := a.ListChanges(sdd.InspectRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Lifecycle != sdd.LifecycleIncomplete {
		t.Fatalf("active = %#v", active)
	}
	found := false
	for _, iss := range issues {
		if iss.Kind == sdd.IssueIncomplete {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected incomplete issue, got %#v", issues)
	}
}

func TestDiscover_AmbiguousNestedChange(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"openspec/changes/parent/proposal.md":        "# parent\n",
		"openspec/changes/parent/nested/proposal.md": "# nested\n",
	})
	a := openspec.New()
	active, _, issues, err := a.ListChanges(sdd.InspectRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || !active[0].Ambiguous || active[0].Lifecycle != sdd.LifecycleUnknown {
		t.Fatalf("active = %#v", active)
	}
	found := false
	for _, iss := range issues {
		if iss.Kind == sdd.IssueAmbiguous {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected ambiguous issue, got %#v", issues)
	}
}

func TestDiscover_ReadyWhenTasksComplete(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"openspec/changes/done-change/proposal.md": "# p\n",
		"openspec/changes/done-change/tasks.md":    "- [x] a\n- [X] b\n",
	})
	a := openspec.New()
	active, _, _, err := a.ListChanges(sdd.InspectRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 || active[0].Lifecycle != sdd.LifecycleReady {
		t.Fatalf("active = %#v", active)
	}
	// ready ≠ verified: verification stays unknown until authoritative evidence.
	if active[0].Lifecycle == sdd.LifecycleVerified {
		t.Fatal("ready must not be mapped to verified")
	}
	if active[0].Verification != sdd.VerificationUnknown {
		t.Fatalf("ready must leave Verification unknown, got %s", active[0].Verification)
	}
	if active[0].Verification == sdd.VerificationPass {
		t.Fatal("ready must not invent verification pass")
	}
}

func TestSafety_SymlinkParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "changes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "openspec")); err != nil {
		t.Fatal(err)
	}
	a := openspec.New()
	_, err := a.Detect(sdd.InspectRequest{Root: root})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink error, got %v", err)
	}
}

func TestSafety_SymlinkLeaf(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	outsideFile := filepath.Join(outside, "proposal.md")
	if err := os.WriteFile(outsideFile, []byte("secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changeDir := filepath.Join(root, "openspec", "changes", "sym-change")
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(changeDir, "proposal.md")); err != nil {
		t.Fatal(err)
	}
	a := openspec.New()
	active, _, issues, err := a.ListChanges(sdd.InspectRequest{Root: root})
	if err != nil {
		// List may return error or issues depending on first failure point.
		if !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("expected symlink err, got %v", err)
		}
		return
	}
	unsafe := false
	for _, iss := range issues {
		if iss.Kind == sdd.IssueUnsafe || strings.Contains(iss.Message, "symlink") {
			unsafe = true
		}
	}
	for _, ref := range active {
		if ref.Ambiguous || strings.Contains(strings.Join(ref.Issues, " "), "symlink") {
			unsafe = true
		}
	}
	if !unsafe {
		t.Fatalf("expected unsafe symlink finding, active=%#v issues=%#v", active, issues)
	}
}

func TestSafety_PathTraversal(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"openspec/changes/ok/proposal.md": "# ok\n",
	})
	a := openspec.New()
	_, err := a.InspectChange(sdd.InspectRequest{Root: root}, "../escape")
	if err == nil {
		t.Fatal("expected error for traversal change id")
	}
	// ContainedJoin refuses ".." in relative paths used as locations.
	_, err = a.Detect(sdd.InspectRequest{Root: root + string(filepath.Separator) + ".." + string(filepath.Separator) + filepath.Base(root)})
	// Detect with odd root may still work after Clean Abs — ensure ContainedJoin traversal refused via direct list with crafted id.
	ref, err := a.InspectChange(sdd.InspectRequest{Root: root}, "ok/../../etc")
	if err == nil && ref.ChangeID != "" && !ref.Ambiguous {
		// If somehow resolved, must not escape — ContainedJoin should have failed.
		t.Fatalf("unexpected success: %#v", ref)
	}
}

func TestMapping_OpenSpecToNeutral(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"openspec/changes/map-me/proposal.md": "# p\n",
		"openspec/changes/map-me/design.md":   "# d\n",
		"openspec/changes/map-me/tasks.md":    "- [ ] t\n",
	})
	a := openspec.New()
	ref, err := a.InspectChange(sdd.InspectRequest{Root: root, ProjectID: "pid"}, "map-me")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Engine != "openspec" {
		t.Fatalf("engine = %s", ref.Engine)
	}
	if ref.Lifecycle != sdd.LifecycleActive {
		t.Fatalf("lifecycle = %s", ref.Lifecycle)
	}
	if ref.Verification != sdd.VerificationUnknown {
		t.Fatalf("verification must be fail-closed unknown, got %s", ref.Verification)
	}
	if !strings.Contains(ref.Location, "map-me") {
		t.Fatalf("location = %s", ref.Location)
	}
}

func TestService_EvaluateWiresAdapter(t *testing.T) {
	root := t.TempDir()
	writeTree(t, root, map[string]string{
		"openspec/changes/svc/proposal.md": "# p\n",
	})
	ov := sdd.NewService(openspec.New()).Evaluate(sdd.InspectRequest{Root: root})
	if !ov.HasEngine() || len(ov.Active) != 1 {
		t.Fatalf("overview = %#v", ov)
	}
	lines := sdd.StatusLines(ov)
	if lines[0] != "SDD: OpenSpec" || !strings.Contains(lines[1], "svc") {
		t.Fatalf("lines = %#v", lines)
	}
}

func TestOpenSpecRootWithoutChangesDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := openspec.New()
	pres, err := a.Detect(sdd.InspectRequest{Root: root})
	if err != nil || !pres.Present {
		t.Fatalf("presence = %#v err=%v", pres, err)
	}
	_, _, issues, err := a.ListChanges(sdd.InspectRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) == 0 {
		t.Fatal("expected incomplete issues")
	}
}
