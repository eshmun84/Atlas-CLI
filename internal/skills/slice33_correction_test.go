package skills_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	adaptercursor "github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

func TestReconcile_ObsoleteReferenceRemovedAndDigestMatches(t *testing.T) {
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
		t.Fatalf("seed: err=%v blocked=%v %v", err, res.Blocked, res.Errors)
	}
	pkg := filepath.Join(root, ".cursor", "skills", "testing")
	oldRef := filepath.Join(pkg, "references", "old.md")
	if err := os.MkdirAll(filepath.Dir(oldRef), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldRef, []byte("obsolete"), 0o644); err != nil {
		t.Fatal(err)
	}
	in.Ownership = res.Ownership
	res2, err := skills.Reconcile(in)
	if err != nil || res2.Blocked {
		t.Fatalf("update: err=%v blocked=%v %v", err, res2.Blocked, res2.Errors)
	}
	if _, err := os.Lstat(oldRef); !os.IsNotExist(err) {
		t.Fatalf("obsolete reference must be removed, err=%v", err)
	}
	digest, _, err := skills.PackageDigest(pkg)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := skills.FindExact(cat, "testing", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if digest != meta.Digest {
		t.Fatalf("projected digest %s != catalog %s", digest, meta.Digest)
	}
}

func TestReconcile_FailureAfterObsoleteRemoval_RestoresFile(t *testing.T) {
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
		t.Fatalf("seed: %v %v", err, res.Errors)
	}
	oldRef := filepath.Join(root, ".cursor", "skills", "testing", "references", "old.md")
	if err := os.MkdirAll(filepath.Dir(oldRef), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldRef, []byte("keep-me"), 0o644); err != nil {
		t.Fatal(err)
	}
	in.Ownership = res.Ownership
	skills.SetReconcileAfterWriteHookForTest(func() error {
		return fmt.Errorf("injected after obsolete removal")
	})
	t.Cleanup(func() { skills.SetReconcileAfterWriteHookForTest(nil) })

	_, err = skills.Reconcile(in)
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(err.Error(), "injected after obsolete removal") {
		t.Fatalf("missing mutation cause: %v", err)
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rolled-back wording: %v", err)
	}
	data, readErr := os.ReadFile(oldRef)
	if readErr != nil || string(data) != "keep-me" {
		t.Fatalf("obsolete file not restored: %q err=%v", data, readErr)
	}
}

func TestReconcile_RollbackFailure_Observable(t *testing.T) {
	skills.RegisterDefaultProjector(adaptercursor.NewSkillsProjector())
	root := t.TempDir()
	homePath := t.TempDir()
	cat, err := skills.DiscoverCatalog(homePath)
	if err != nil {
		t.Fatal(err)
	}
	skillMD := filepath.Join(root, ".cursor", "skills", "testing", "SKILL.md")
	skills.SetReconcileAfterWriteHookForTest(func() error {
		if err := os.Remove(skillMD); err != nil {
			return err
		}
		if err := os.Mkdir(skillMD, 0o755); err != nil {
			return err
		}
		return fmt.Errorf("injected mutation failure")
	})
	t.Cleanup(func() { skills.SetReconcileAfterWriteHookForTest(nil) })

	_, err = skills.Reconcile(skills.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  "proj",
		Enabled:    []skills.Pin{{ID: "testing", Version: "1.0.0"}},
		Adapters:   []string{"cursor"},
		Projectors: skills.DefaultProjectors(),
		Catalog:    cat,
	})
	if err == nil {
		t.Fatal("expected combined failure")
	}
	msg := err.Error()
	if !strings.Contains(msg, "injected mutation failure") {
		t.Fatalf("missing original failure: %v", err)
	}
	if !strings.Contains(msg, "rollback failed") {
		t.Fatalf("rollback failure must be observable (not ordinary rolled-back): %v", err)
	}
	if strings.Contains(msg, "(rolled back)") && !strings.Contains(msg, "rollback failed") {
		t.Fatalf("must not report ordinary rolled-back when rollback incomplete: %v", err)
	}
}

func TestReconcile_UserOwnedCollisionPreserved(t *testing.T) {
	skills.RegisterDefaultProjector(adaptercursor.NewSkillsProjector())
	root := t.TempDir()
	homePath := t.TempDir()
	cat, err := skills.DiscoverCatalog(homePath)
	if err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(root, ".cursor", "skills", "testing")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(foreign, "SKILL.md"), []byte("---\nname: testing\ndescription: user\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := skills.Reconcile(skills.ReconcileInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  "proj",
		Enabled:    []skills.Pin{{ID: "testing", Version: "1.0.0"}},
		Adapters:   []string{"cursor"},
		Projectors: skills.DefaultProjectors(),
		Catalog:    cat,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Blocked {
		t.Fatal("expected conflict block")
	}
	data, _ := os.ReadFile(filepath.Join(foreign, "SKILL.md"))
	if !strings.Contains(string(data), "user") {
		t.Fatal("user-owned content must be preserved")
	}
}

func TestDiscoverCatalog_AssetsSymlinkFailClosed(t *testing.T) {
	homePath := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(homePath, "assets")); err != nil {
		t.Fatal(err)
	}
	_, err := skills.DiscoverCatalog(homePath)
	if err == nil {
		t.Fatal("expected fail closed for assets symlink")
	}
	if strings.Contains(err.Error(), "bundled") {
		t.Fatalf("must not silently fall back to embed: %v", err)
	}
}

func TestDiscoverCatalog_SkillsRootSymlinkFailClosed(t *testing.T) {
	homePath := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(homePath, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(homePath, "assets", "skills")); err != nil {
		t.Fatal(err)
	}
	_, err := skills.DiscoverCatalog(homePath)
	if err == nil {
		t.Fatal("expected fail closed for skills root symlink")
	}
}

func TestDiscoverCatalog_PackageSymlinkFailClosed(t *testing.T) {
	homePath := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte("---\nname: testing\ndescription: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idDir := filepath.Join(homePath, "assets", "skills", "testing")
	if err := os.MkdirAll(idDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(idDir, "1.0.0")); err != nil {
		t.Fatal(err)
	}
	_, err := skills.DiscoverCatalog(homePath)
	if err == nil {
		t.Fatal("expected fail closed for package symlink")
	}
}

func TestDiscoverCatalog_UnexpectedReadError_NoEmbedFallback(t *testing.T) {
	homePath := t.TempDir()
	skillsPath := filepath.Join(homePath, "assets", "skills")
	if err := os.MkdirAll(filepath.Dir(skillsPath), 0o755); err != nil {
		t.Fatal(err)
	}
	// Regular file where directory expected → unexpected catalog shape, fail closed.
	if err := os.WriteFile(skillsPath, []byte("not-a-dir"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := skills.DiscoverCatalog(homePath)
	if err == nil {
		t.Fatal("expected fail closed")
	}
	// Must not return a successful embed catalog.
	if err == nil {
		t.Fatal("unreachable")
	}
}

func TestDiscoverCatalog_GenuineAbsence_EmbedFallback(t *testing.T) {
	cat, err := skills.DiscoverCatalog(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(cat) != 4 {
		t.Fatalf("want bundled embed catalog, got %d", len(cat))
	}
	for _, m := range cat {
		if m.Source != skills.SourceBundled {
			t.Fatalf("expected bundled source, got %+v", m)
		}
	}
}

func TestLoadPackageFiles_UnsupportedHomePackageFailClosed(t *testing.T) {
	homePath := t.TempDir()
	pkgRel := "assets/skills/testing/1.0.0"
	pkg := filepath.Join(homePath, filepath.FromSlash(pkgRel))
	if err := os.MkdirAll(pkg, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: testing\ndescription: x\n---\n\n# testing\n"
	if err := os.WriteFile(filepath.Join(pkg, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkg, "EXTRA.txt"), []byte("nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := skills.LoadPackageContained(homePath, pkgRel, "testing", "1.0.0", skills.SourceHome)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported fail closed, got %v", err)
	}
}
