package project

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
		if !strings.Contains(warning, "remote default branch unknown locally") {
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
	if info.DefaultBranch != "" {
		t.Fatalf("DefaultBranch = %q, want empty when origin/HEAD missing", info.DefaultBranch)
	}
	if len(info.Remotes) != 1 {
		t.Fatalf("remotes = %#v, want 1 entry", info.Remotes)
	}
	if info.Remotes[0].Name != "origin" || info.Remotes[0].URL != "https://example.com/atlas.git" {
		t.Fatalf("unexpected remote: %#v", info.Remotes[0])
	}
}

func TestDiscoverGit_RemoteDefaultBranchUnknownLocally(t *testing.T) {
	root := initTestRepoWithCommit(t, "feature/x")
	runGitOk(t, root, "remote", "add", "origin", "https://example.com/atlas.git")
	seedRemoteTrackingBranches(t, root, "main", "develop", "staging")
	// Intentionally leave refs/remotes/origin/HEAD unset.

	before := snapshotGitDir(t, root)
	info, warnings, err := discoverGit(root)
	if err != nil {
		t.Fatalf("discoverGit: %v", err)
	}
	assertGitDirUnchanged(t, root, before)

	if info.DefaultRemote != "origin" {
		t.Fatalf("DefaultRemote = %q, want origin", info.DefaultRemote)
	}
	if info.DefaultBranch != "" {
		t.Fatalf("DefaultBranch = %q, want empty (unknown locally)", info.DefaultBranch)
	}
	found := false
	for _, warning := range warnings {
		if strings.Contains(warning, "refs/remotes/origin/HEAD is not configured") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected unknown-locally warning, got %#v", warnings)
	}
}

func TestDiscoverGit_RemoteDefaultBranchFromOriginHEAD(t *testing.T) {
	root := initTestRepoWithCommit(t, "feature/x")
	runGitOk(t, root, "remote", "add", "origin", "https://example.com/atlas.git")
	seedRemoteTrackingBranches(t, root, "main", "develop", "staging")
	runGitOk(t, root, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")

	before := snapshotGitDir(t, root)
	info, err := DiscoverGit(root)
	if err != nil {
		t.Fatalf("DiscoverGit: %v", err)
	}
	assertGitDirUnchanged(t, root, before)

	if info.DefaultBranch != "main" {
		t.Fatalf("DefaultBranch = %q, want main", info.DefaultBranch)
	}
}

func TestDiscoverGit_NoRemoteDefaultBranchNone(t *testing.T) {
	root := initTestRepoWithCommit(t, "main")

	before := snapshotGitDir(t, root)
	info, warnings, err := discoverGit(root)
	if err != nil {
		t.Fatalf("discoverGit: %v", err)
	}
	assertGitDirUnchanged(t, root, before)

	if info.DefaultRemote != "" || info.DefaultRemoteURL != "" {
		t.Fatalf("default remote = %q %q, want empty", info.DefaultRemote, info.DefaultRemoteURL)
	}
	if info.DefaultBranch != "" {
		t.Fatalf("DefaultBranch = %q, want empty", info.DefaultBranch)
	}
	for _, warning := range warnings {
		if strings.Contains(warning, "default branch") || strings.Contains(warning, "remote default") {
			t.Fatalf("unexpected default-branch warning without remote: %#v", warnings)
		}
	}
}

func TestDiscover_GitRepoIntegration(t *testing.T) {
	root := initTestRepo(t, "develop")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# repo"), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	result, err := Inspect(root)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
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

func initTestRepoWithCommit(t *testing.T, branch string) string {
	t.Helper()
	root := initTestRepo(t, branch)
	runGitOk(t, root, "config", "user.email", "atlas@example.com")
	runGitOk(t, root, "config", "user.name", "Atlas Test")
	runGitOk(t, root, "commit", "--allow-empty", "-m", "init")
	return root
}

func seedRemoteTrackingBranches(t *testing.T, root string, branches ...string) {
	t.Helper()
	for _, branch := range branches {
		runGitOk(t, root, "update-ref", "refs/remotes/origin/"+branch, "HEAD")
	}
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

func snapshotGitDir(t *testing.T, root string) map[string]string {
	t.Helper()
	gitDir := filepath.Join(root, ".git")
	out := map[string]string{}
	err := filepath.Walk(gitDir, func(path string, info os.FileInfo, err error) error {
		rel, relErr := filepath.Rel(gitDir, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		// Git may create/remove transient lock files (e.g. objects/maintenance.lock)
		// while we fingerprint. Treat missing lock paths as ignorable, not mutations.
		if err != nil {
			if os.IsNotExist(err) && isTransientGitLock(rel) {
				return nil
			}
			return err
		}
		if isTransientGitLock(rel) {
			return nil
		}
		if info.IsDir() {
			out[rel+"/"] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) && isTransientGitLock(rel) {
				return nil
			}
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

// isTransientGitLock reports whether rel (slash-separated, relative to .git) is a
// Git-managed lock file that may appear or disappear without DiscoverGit mutating
// persistent repository metadata.
func isTransientGitLock(rel string) bool {
	if rel == "" || strings.HasSuffix(rel, "/") {
		return false
	}
	base := filepath.Base(rel)
	return strings.HasSuffix(base, ".lock")
}

func assertGitDirUnchanged(t *testing.T, root string, before map[string]string) {
	t.Helper()
	after := snapshotGitDir(t, root)
	if len(before) != len(after) {
		t.Fatalf(".git tree size changed: before=%d after=%d", len(before), len(after))
	}
	for path, content := range before {
		got, ok := after[path]
		if !ok {
			t.Fatalf(".git path removed: %s", path)
		}
		if got != content {
			t.Fatalf(".git path mutated: %s", path)
		}
	}
}

func TestSnapshotGitDir_IgnoresTransientLockFiles(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	objects := filepath.Join(gitDir, "objects")
	if err := os.MkdirAll(objects, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(objects, "maintenance.lock"), []byte("pid"), 0o644); err != nil {
		t.Fatalf("write maintenance.lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "index.lock"), []byte("pid"), 0o644); err != nil {
		t.Fatalf("write index.lock: %v", err)
	}

	snap := snapshotGitDir(t, root)
	for path := range snap {
		if isTransientGitLock(strings.TrimSuffix(path, "/")) || strings.HasSuffix(path, ".lock") {
			t.Fatalf("snapshot included transient lock path %q", path)
		}
	}
	if _, ok := snap["HEAD"]; !ok {
		t.Fatal("expected HEAD in snapshot")
	}
	if _, ok := snap["objects/"]; !ok {
		t.Fatal("expected objects/ directory in snapshot")
	}
}

func TestSnapshotGitDir_DetectsPersistentMetadataMutation(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	gitDir := filepath.Join(root, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs/heads"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "description"), []byte("test repo\n"), 0o644); err != nil {
		t.Fatalf("write description: %v", err)
	}

	before := snapshotGitDir(t, root)
	if err := os.WriteFile(filepath.Join(gitDir, "refs/heads/main"), []byte("abc123\n"), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}
	after := snapshotGitDir(t, root)
	if len(after) <= len(before) {
		t.Fatalf("expected persistent ref to enlarge snapshot: before=%d after=%d", len(before), len(after))
	}
	if _, ok := after["refs/heads/main"]; !ok {
		t.Fatal("expected refs/heads/main in after snapshot")
	}

	// Lock files appearing between snapshots must not count as mutations.
	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0o755); err != nil {
		t.Fatalf("mkdir objects: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "objects/maintenance.lock"), []byte("pid"), 0o644); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	withLock := snapshotGitDir(t, root)
	// objects/ dir may be new; lock file itself must still be excluded.
	if _, ok := withLock["objects/maintenance.lock"]; ok {
		t.Fatal("objects/maintenance.lock must be ignored by fingerprint")
	}
	for path := range withLock {
		if path == "objects/" {
			continue
		}
		if _, ok := after[path]; !ok && !strings.HasPrefix(path, "objects/") {
			t.Fatalf("unexpected non-lock path in withLock: %s", path)
		}
	}
	if got, want := withLock["refs/heads/main"], after["refs/heads/main"]; got != want {
		t.Fatalf("persistent ref content drifted when lock appeared")
	}
}
