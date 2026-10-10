package skills_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	adaptercursor "github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	adapteropencode "github.com/eshmun84/Atlas-CLI/internal/adapters/opencode"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

func TestParseSkillMD_Valid(t *testing.T) {
	body, err := skills.ReadBundledSkillMD("testing", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	fm, err := skills.ParseSkillMD(body, "testing")
	if err != nil {
		t.Fatal(err)
	}
	if fm.Name != "testing" || fm.Description == "" {
		t.Fatalf("unexpected frontmatter: %+v", fm)
	}
}

func TestParseSkillMD_MissingFrontmatter(t *testing.T) {
	_, err := skills.ParseSkillMD("# no frontmatter\n", "testing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseSkillMD_InvalidName(t *testing.T) {
	body := "---\nname: Bad Name\ndescription: x\n---\n\n# X\n"
	_, err := skills.ParseSkillMD(body, "testing")
	if err == nil {
		t.Fatal("expected invalid name error")
	}
}

func TestPackageDigest_StableAndSensitive(t *testing.T) {
	a := map[string][]byte{"SKILL.md": []byte("one"), "references/a.md": []byte("ref")}
	b := map[string][]byte{"references/a.md": []byte("ref"), "SKILL.md": []byte("one")}
	da, err := skills.DigestBytes(a)
	if err != nil {
		t.Fatal(err)
	}
	db, err := skills.DigestBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if da != db {
		t.Fatal("ordering must not change digest")
	}
	c := map[string][]byte{"SKILL.md": []byte("two"), "references/a.md": []byte("ref")}
	dc, _ := skills.DigestBytes(c)
	if da == dc {
		t.Fatal("content change must change digest")
	}
	d := map[string][]byte{"SKILL.md": []byte("one"), "references/b.md": []byte("ref")}
	dd, _ := skills.DigestBytes(d)
	if da == dd {
		t.Fatal("path change must change digest")
	}
}

func TestPackageDigest_SymlinkRejected(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: testing\ndescription: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../SKILL.md", filepath.Join(root, "references", "x.md")); err != nil {
		t.Fatal(err)
	}
	_, _, err := skills.PackageDigest(root)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink refusal, got %v", err)
	}
}

func TestPackageDigest_UnsupportedTopLevelFailClosed(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte("---\nname: testing\ndescription: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "EXTRA.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := skills.PackageDigest(root)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported entry failure, got %v", err)
	}
}

func TestExactVersion_NoRanges(t *testing.T) {
	if err := skills.ValidateExactVersion("1.0.0"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"latest", "*", "^1.0", ">=1.0", "1.0"} {
		if err := skills.ValidateExactVersion(bad); err == nil {
			t.Fatalf("expected reject %q", bad)
		}
	}
}

func TestCatalog_DeterministicBundled(t *testing.T) {
	a, err := skills.DiscoverCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := skills.DiscoverCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 4 || len(b) != 4 {
		t.Fatalf("want 4 bundled skills, got %d/%d", len(a), len(b))
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Version != b[i].Version || a[i].Digest != b[i].Digest {
			t.Fatalf("nondeterministic catalog: %+v vs %+v", a[i], b[i])
		}
	}
}

func TestPin_ExactResolvesMissingBlocks(t *testing.T) {
	cat, err := skills.DiscoverCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := skills.FindExact(cat, "testing", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if _, err := skills.FindExact(cat, "testing", "9.9.9"); err == nil {
		t.Fatal("missing exact version must block")
	}
	// No implicit newest fallback when asking for wrong version.
	_, err = skills.ResolvePins(cat, []skills.Pin{{ID: "testing", Version: "0.0.1"}})
	if err == nil {
		t.Fatal("expected missing pin error")
	}
}

func TestResolve_BoundedDeterministic(t *testing.T) {
	cat, err := skills.DiscoverCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	out, err := skills.Resolve(skills.ResolveInput{
		Intent:     "security review of auth changes",
		Enabled:    skills.DefaultPins(),
		Catalog:    cat,
		AdapterIDs: []string{"cursor"},
		MaxResults: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 || len(out) > 2 {
		t.Fatalf("bounded result expected, got %d", len(out))
	}
	if out[0].SkillMDRel == "" || out[0].Digest == "" {
		t.Fatal("must return exact references, not rewritten content")
	}
	// Irrelevant intent should not load everything.
	none, err := skills.Resolve(skills.ResolveInput{
		Intent:     "unrelated chocolate cake recipe",
		Enabled:    skills.DefaultPins(),
		Catalog:    cat,
		AdapterIDs: []string{"cursor"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("irrelevant skills must not be selected, got %v", none)
	}
}

func TestProjectors_CursorOpenCode(t *testing.T) {
	skills.RegisterDefaultProjector(adaptercursor.NewSkillsProjector())
	skills.RegisterDefaultProjector(adapteropencode.NewSkillsProjector())
	projs := skills.DefaultProjectors()
	c := projs[adapters.Cursor]
	o := projs[adapters.OpenCode]
	if c == nil || !c.SupportsSkills() || c.SkillsRootRel() == "" {
		t.Fatal("cursor projector incomplete")
	}
	if o == nil || !o.SupportsSkills() || o.SkillsRootRel() == "" {
		t.Fatal("opencode projector incomplete")
	}
	if c.PackageRootRel("testing") == o.PackageRootRel("testing") {
		t.Fatal("adapter roots should remain adapter-owned (distinct paths)")
	}
}

func TestReconcile_IdempotentAndConflict(t *testing.T) {
	skills.RegisterDefaultProjector(adaptercursor.NewSkillsProjector())
	root := t.TempDir()
	homePath := t.TempDir()
	cat, err := skills.DiscoverCatalog(homePath)
	if err != nil {
		t.Fatal(err)
	}
	in := skills.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  "proj",
		Enabled:    []skills.Pin{{ID: "testing", Version: "1.0.0"}},
		Adapters:   []string{"cursor"},
		Projectors: skills.DefaultProjectors(),
		Catalog:    cat,
	}
	res, err := skills.Reconcile(in)
	if err != nil || res.Blocked {
		t.Fatalf("first apply: err=%v blocked=%v errors=%v", err, res.Blocked, res.Errors)
	}
	skillPath := filepath.Join(root, ".cursor", "skills", "testing", "SKILL.md")
	if _, err := os.Lstat(skillPath); err != nil {
		t.Fatal(err)
	}
	in.Ownership = res.Ownership
	res2, err := skills.Reconcile(in)
	if err != nil || res2.Blocked {
		t.Fatalf("idempotent: err=%v blocked=%v", err, res2.Blocked)
	}

	// User-owned collision: create unknown skill dir without ownership.
	foreign := filepath.Join(root, ".cursor", "skills", "code-review")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "SKILL.md"), []byte("---\nname: code-review\ndescription: user\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	in.Enabled = skills.DefaultPins()
	in.Ownership = res2.Ownership
	res3, err := skills.Reconcile(in)
	if err != nil {
		t.Fatal(err)
	}
	if !res3.Blocked {
		t.Fatal("expected conflict block for user-owned projection")
	}
	// User content preserved.
	data, _ := os.ReadFile(filepath.Join(foreign, "SKILL.md"))
	if !strings.Contains(string(data), "user") {
		t.Fatal("user-owned content must be preserved")
	}
}

func TestAgentSkillRefs_NoProviderPaths(t *testing.T) {
	refs := skills.AgentSkillRefs("atlas-orchestrator")
	if len(refs) == 0 {
		t.Fatal("expected skill ids")
	}
	for _, r := range refs {
		if strings.Contains(r, "/") || strings.Contains(r, ".cursor") || strings.Contains(r, ".opencode") {
			t.Fatalf("provider path leaked: %s", r)
		}
	}
}
