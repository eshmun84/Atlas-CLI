package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// DefaultInitialBranch is the deterministic initial branch for Atlas Git bootstrap.
const DefaultInitialBranch = "main"

// GitMissingHint is the actionable message when Git bootstrap is required but
// git is not available on PATH.
const GitMissingHint = "Git is required to initialize a local repository but was not found on PATH. Install Git and retry, or change source control mode to none."

// lookPathGit is the LookPath seam for locating the git binary. Tests may override
// via SetLookPathGitForTest; gitLookPathTestMu serializes those overrides across
// packages so stubbed and real-git tests do not race.
var (
	lookPathGit       = exec.LookPath
	gitLookPathTestMu sync.Mutex
	gitLookPathHeld   bool
)

// SetLookPathGitForTest installs a test-only LookPath seam for git bootstrap.
// Passing a non-nil fn acquires an exclusive test lock; passing nil restores
// exec.LookPath and releases the lock. Callers must Cleanup with nil.
func SetLookPathGitForTest(fn func(file string) (string, error)) {
	if fn == nil {
		lookPathGit = exec.LookPath
		if gitLookPathHeld {
			gitLookPathHeld = false
			gitLookPathTestMu.Unlock()
		}
		return
	}
	if !gitLookPathHeld {
		gitLookPathTestMu.Lock()
		gitLookPathHeld = true
	}
	lookPathGit = fn
}

// GitBootstrapResult describes an Apply-time local Git initialization.
type GitBootstrapResult struct {
	Created       bool
	InitialBranch string
}

// NeedsGitBootstrap reports whether source_control implies a local Git repo
// and the workspace is not already inside one.
func NeedsGitBootstrap(root, sourceControlMode string) (bool, error) {
	if !NeedsGitBootstrapMode(ProjectDocument{
		SourceControl: SourceControlPersist{Mode: strings.TrimSpace(sourceControlMode)},
	}) {
		return false, nil
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return false, fmt.Errorf("git bootstrap: workspace root is required")
	}
	inside, err := isGitWorkTree(root)
	if err != nil {
		return false, err
	}
	if inside {
		return false, nil
	}
	// Preserve an existing .git marker even when git is absent from PATH so
	// Apply does not demand Git availability for a re-init that will not run.
	if hasLocalGitDir(root) {
		return false, nil
	}
	return true, nil
}

// RequireGitForBootstrap fails closed when Git bootstrap is required but git
// cannot be resolved on PATH. Read-only: never mutates the workspace.
func RequireGitForBootstrap(root, sourceControlMode string) error {
	need, err := NeedsGitBootstrap(root, sourceControlMode)
	if err != nil {
		return err
	}
	if !need {
		return nil
	}
	if _, err := lookPathGit("git"); err != nil {
		return fmt.Errorf("%s", GitMissingHint)
	}
	return nil
}

// BootstrapGitRepo initializes a local Git repository with HEAD on main.
// It does not create commits, remotes, tags, or push. If a repo already exists,
// it is a no-op that preserves branch/remotes/config.
func BootstrapGitRepo(root string) (GitBootstrapResult, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return GitBootstrapResult{}, fmt.Errorf("git bootstrap: workspace root is required")
	}

	inside, err := isGitWorkTree(root)
	if err != nil {
		return GitBootstrapResult{}, err
	}
	if inside {
		branch, _ := currentGitBranch(root)
		return GitBootstrapResult{Created: false, InitialBranch: branch}, nil
	}
	if hasLocalGitDir(root) {
		return GitBootstrapResult{Created: false}, nil
	}

	gitPath, err := lookPathGit("git")
	if err != nil {
		return GitBootstrapResult{}, fmt.Errorf("%s", GitMissingHint)
	}

	gitDir := filepath.Join(root, ".git")
	if out, initErr := runGitAt(gitPath, root, "init"); initErr != nil {
		return GitBootstrapResult{}, fmt.Errorf("git bootstrap: git init: %w (%s)", initErr, strings.TrimSpace(out))
	}
	if out, refErr := runGitAt(gitPath, root, "symbolic-ref", "HEAD", "refs/heads/"+DefaultInitialBranch); refErr != nil {
		_ = os.RemoveAll(gitDir)
		return GitBootstrapResult{}, fmt.Errorf("git bootstrap: set initial branch: %w (%s)", refErr, strings.TrimSpace(out))
	}

	inside, err = isGitWorkTree(root)
	if err != nil || !inside {
		_ = os.RemoveAll(gitDir)
		if err != nil {
			return GitBootstrapResult{}, err
		}
		return GitBootstrapResult{}, fmt.Errorf("git bootstrap: repository not detected after init")
	}
	return GitBootstrapResult{Created: true, InitialBranch: DefaultInitialBranch}, nil
}

// RemoveBootstrappedGit removes a .git directory Atlas created during a failed Apply.
func RemoveBootstrappedGit(root string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	gitDir := filepath.Join(root, ".git")
	info, err := os.Lstat(gitDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("git bootstrap rollback: refusing to remove symlink .git")
	}
	return os.RemoveAll(gitDir)
}

func hasLocalGitDir(root string) bool {
	_, err := os.Lstat(filepath.Join(root, ".git"))
	return err == nil
}

func isGitWorkTree(root string) (bool, error) {
	gitPath, err := lookPathGit("git")
	if err != nil {
		return false, nil
	}
	out, err := runGitAt(gitPath, root, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(out) == "true", nil
}

func currentGitBranch(root string) (string, error) {
	gitPath, err := lookPathGit("git")
	if err != nil {
		return "", err
	}
	out, err := runGitAt(gitPath, root, "branch", "--show-current")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func runGitAt(gitPath, root string, args ...string) (string, error) {
	cmd := exec.Command(gitPath, args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	return string(out), err
}
