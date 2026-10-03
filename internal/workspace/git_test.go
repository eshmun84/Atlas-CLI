package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRemotes_DeduplicatesFetchPush(t *testing.T) {
	t.Parallel()

	output := "" +
		"origin\thttps://example.com/demo.git (fetch)\n" +
		"origin\thttps://example.com/demo.git (push)\n" +
		"upstream\tgit@example.com:demo.git (fetch)\n" +
		"upstream\tgit@example.com:demo.git (push)\n"

	remotes := parseRemotes(output)
	if len(remotes) != 2 {
		t.Fatalf("got %d remotes, want 2: %#v", len(remotes), remotes)
	}
	if remotes[0].Name != "origin" || remotes[0].URL != "https://example.com/demo.git" {
		t.Fatalf("unexpected first remote: %#v", remotes[0])
	}
	if remotes[1].Name != "upstream" || remotes[1].URL != "git@example.com:demo.git" {
		t.Fatalf("unexpected second remote: %#v", remotes[1])
	}
}

func TestDiscoverGit_NonRepo(t *testing.T) {
	t.Parallel()

	info, err := DiscoverGit(t.TempDir())
	if err != nil {
		t.Fatalf("DiscoverGit: %v", err)
	}
	if info.IsRepo {
		t.Fatal("expected non-repo directory")
	}
}

func TestDiscoverGit_RepoBranchAndRemotes(t *testing.T) {
	root := initTestRepo(t, "feature/slice-3")
	runGitOk(t, root, "remote", "add", "origin", "https://example.com/atlas.git")

	info, warnings, err := discoverGit(root)
	if err != nil {
		t.Fatalf("discoverGit: %v", err)
	}
	for _, warning := range warnings {
		if !strings.Contains(warning, "default branch") {
			t.Fatalf("unexpected warning: %v", warnings)
		}
	}
	if !info.IsRepo {
		t.Fatal("expected IsRepo=true")
	}
	if info.CurrentBranch != "feature/slice-3" {
		t.Fatalf("branch = %q, want feature/slice-3", info.CurrentBranch)
	}
	if info.DefaultRemote != "origin" || info.DefaultRemoteURL != "https://example.com/atlas.git" {
		t.Fatalf("default remote = %q %q", info.DefaultRemote, info.DefaultRemoteURL)
	}
	if len(info.Remotes) != 1 {
		t.Fatalf("remotes = %#v, want 1 entry", info.Remotes)
	}
	if info.Remotes[0].Name != "origin" || info.Remotes[0].URL != "https://example.com/atlas.git" {
		t.Fatalf("unexpected remote: %#v", info.Remotes[0])
	}
}

func TestDiscover_GitRepoIntegration(t *testing.T) {
	root := initTestRepo(t, "develop")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# repo"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	result, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !result.Git.IsRepo {
		t.Fatal("expected git repo")
	}
	if result.Git.CurrentBranch != "develop" {
		t.Fatalf("branch = %q, want develop", result.Git.CurrentBranch)
	}
	if !result.Files.HasReadme {
		t.Fatal("expected README.md")
	}
}

func initTestRepo(t *testing.T, branch string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git unavailable")
	}

	root := t.TempDir()
	cmd := exec.Command("git", "-C", root, "init", "--template=", "-b", branch)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TEMPLATE_DIR=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Skipf("git init unavailable in this environment: %v\n%s", err, out)
	}
	return root
}

func runGitOk(t *testing.T, root string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", cmdArgs...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TEMPLATE_DIR=",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
