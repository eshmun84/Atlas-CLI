package project_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project"
)

func TestInspect_GitRepoSeedsSnapshot(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}
	root := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("branch", "-M", "develop")
	writeFile(t, filepath.Join(root, "README.md"), "# demo\n")
	run("add", "README.md")
	run("-c", "user.email=t@example.com", "-c", "user.name=T", "commit", "-m", "seed")

	snap, err := project.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.Git.IsRepo {
		t.Fatal("expected git repo")
	}
	if snap.Git.CurrentBranch != "develop" {
		t.Fatalf("branch = %q", snap.Git.CurrentBranch)
	}
	if !snap.Files.HasReadme {
		t.Fatal("expected readme")
	}
	if snap.RootPath == "" {
		t.Fatal("expected absolute root")
	}
}

func TestInspect_NoGit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# x\n")
	snap, err := project.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Git.IsRepo {
		t.Fatal("expected no git repo")
	}
}
