package codeintel_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
