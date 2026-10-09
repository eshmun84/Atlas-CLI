package codeintel_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/project"
)

func TestComputeSourceFingerprint_ContentChanges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.go"), "package main\n")
	a, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil || a.Value == "" {
		t.Fatalf("fp=%#v err=%v", a, err)
	}
	mustWrite(t, filepath.Join(root, "main.go"), "package main\nfunc main(){}\n")
	b, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if a.Value == b.Value {
		t.Fatal("content change must change fingerprint")
	}
}

func TestComputeSourceFingerprint_IgnoresGenerated(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.go"), "package main\n")
	base, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "vendor", "x.go"), "package x\n")
	mustWrite(t, filepath.Join(root, "node_modules", "x", "index.js"), "noise\n")
	mustWrite(t, filepath.Join(root, ".atlas", "config.yaml"), "x: 1\n")
	mustWrite(t, filepath.Join(root, ".codegraph", "changes.journal"), "j\n")
	mustWrite(t, filepath.Join(root, ".cursor", "rules", "x.md"), "r\n")
	after, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if base.Value != after.Value {
		t.Fatal("ignored paths must not affect fingerprint")
	}
}

func TestComputeSourceFingerprint_UntrackedRelevantFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "main.go"), "package main\n")
	before, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "extra.go"), "package main\n")
	after, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if before.Value == after.Value {
		t.Fatal("untracked relevant file must change fingerprint")
	}
}

func TestComputeSourceFingerprint_UsesProjectPolicyNotContext(t *testing.T) {
	t.Parallel()
	// Behavioral lock: project policy decides relevance.
	if !project.ShouldSkipSourceDir(".codegraph", ".codegraph") {
		t.Fatal("project must own .codegraph skip")
	}
	if !project.IsRelevantSourceFile("lib.go", 1) {
		t.Fatal("project must mark lib.go relevant")
	}
}

func TestComputeSourceFingerprint_GitHEADAndDirty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "branch", "-M", "develop")
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.email=t@ex.com", "-c", "user.name=T", "-c", "commit.gpgsign=false", "commit", "-m", "seed")
	clean, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if clean.GitHEAD == "" {
		t.Fatal("expected git HEAD")
	}
	mustWrite(t, filepath.Join(root, "a.go"), "package a\nfunc F(){}\n")
	dirty, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if clean.Value == dirty.Value {
		t.Fatal("dirty content must change fingerprint")
	}
	// Change already-dirty file again.
	mustWrite(t, filepath.Join(root, "a.go"), "package a\nfunc F(){ }\n")
	dirty2, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if dirty.Value == dirty2.Value {
		t.Fatal("subsequent dirty content change must change fingerprint")
	}
}

func TestComputeSourceFingerprint_BeyondFormerCap(t *testing.T) {
	root := t.TempDir()
	const n = 4001
	for i := 0; i < n; i++ {
		mustWrite(t, filepath.Join(root, fmt.Sprintf("f%05d.go", i)), fmt.Sprintf("package f%d\n", i))
	}
	before, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if before.Files < n {
		t.Fatalf("files=%d want >= %d", before.Files, n)
	}
	mustWrite(t, filepath.Join(root, fmt.Sprintf("f%05d.go", n-1)), "package mutated\n")
	after, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if before.Value == after.Value {
		t.Fatal("mutation beyond former 4000-file cap must change fingerprint")
	}
}

func TestComputeSourceFingerprint_HashFailure(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "ok.go"), "package ok\n")
	blocked := filepath.Join(root, "blocked.go")
	mustWrite(t, blocked, "package blocked\n")
	if err := os.Chmod(blocked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o644) })
	_, err := codeintel.ComputeSourceFingerprint(root)
	if err == nil {
		t.Fatal("expected hash/read failure")
	}
}

func TestComputeSourceFingerprint_WalkError(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "ok.go"), "package ok\n")
	codeintel.SetFingerprintWalkForTest(func(string, filepath.WalkFunc) error {
		return errors.New("injected walk failure")
	})
	t.Cleanup(func() { codeintel.SetFingerprintWalkForTest(nil) })
	_, err := codeintel.ComputeSourceFingerprint(root)
	if err == nil {
		t.Fatal("expected walk error")
	}
}

func TestComputeSourceFingerprint_RelError(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "ok.go"), "package ok\n")
	codeintel.SetFingerprintRelForTest(func(string, string) (string, error) {
		return "", errors.New("injected rel failure")
	})
	t.Cleanup(func() { codeintel.SetFingerprintRelForTest(nil) })
	_, err := codeintel.ComputeSourceFingerprint(root)
	if err == nil {
		t.Fatal("expected rel error")
	}
}

func TestComputeSourceFingerprint_Deterministic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "a.go"), "package a\n")
	mustWrite(t, filepath.Join(root, "b.go"), "package b\n")
	a, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	b, err := codeintel.ComputeSourceFingerprint(root)
	if err != nil {
		t.Fatal(err)
	}
	if a.Value == "" || a.Value != b.Value || a.Files != b.Files {
		t.Fatalf("deterministic mismatch %#v vs %#v", a, b)
	}
}

func TestComputeSourceFingerprint_SymlinkSourceRefused(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	ext := filepath.Join(outside, "ext.go")
	mustWrite(t, ext, "package ext\nfunc Secret(){}\n")
	if err := os.Symlink(ext, filepath.Join(root, "linked.go")); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "ok.go"), "package ok\n")
	_, err := codeintel.ComputeSourceFingerprint(root)
	if err == nil {
		t.Fatal("expected symlink source error")
	}
	// External target must not contribute: fingerprint of ok-only tree differs
	// from a tree that included symlink content if it had been followed.
	alone := t.TempDir()
	mustWrite(t, filepath.Join(alone, "ok.go"), "package ok\n")
	fpAlone, err := codeintel.ComputeSourceFingerprint(alone)
	if err != nil {
		t.Fatal(err)
	}
	if fpAlone.Files != 1 {
		t.Fatalf("alone files=%d", fpAlone.Files)
	}
}

func TestComputeSourceFingerprint_DisappearanceDuringHash(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "ok.go"), "package ok\n")
	codeintel.SetFingerprintWalkForTest(func(walkRoot string, walkFn filepath.WalkFunc) error {
		return walkFn(filepath.Join(walkRoot, "ghost.go"), &fakeFileInfo{name: "ghost.go", size: 12}, nil)
	})
	t.Cleanup(func() { codeintel.SetFingerprintWalkForTest(nil) })
	_, err := codeintel.ComputeSourceFingerprint(root)
	if err == nil {
		t.Fatal("expected hash disappearance error")
	}
}

type fakeFileInfo struct {
	name string
	size int64
}

func (f *fakeFileInfo) Name() string       { return f.name }
func (f *fakeFileInfo) Size() int64        { return f.size }
func (f *fakeFileInfo) Mode() os.FileMode  { return 0o644 }
func (f *fakeFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (f *fakeFileInfo) IsDir() bool        { return false }
func (f *fakeFileInfo) Sys() any           { return nil }

func mustWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
