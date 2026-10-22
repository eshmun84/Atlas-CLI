package hookactions

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
)

// OpenCode session identity (installed OpenCode 2.0.20):
//
//   - One OpenCode session belongs to one Atlas project.
//   - session.created carries Location.Ref at data.location.directory
//     (and the event envelope may repeat location.directory).
//     SessionV1 still carries info.directory. The plugin sends one
//     verified directory. Conflicting directories are not a match.
//   - tool and session hooks carry sessionID, not a directory. The
//     plugin resolves session.get(...).location.directory before dispatch.
//   - Membership is realpath equality with the project root Atlas will
//     mutate. The plugin install directory alone is not membership.
//   - A missing, unresolvable, or different directory does not admit
//     Phase B or Phase C. Cursor events leave SessionDirectory empty.
func openCodeSessionAllowed(adapter adapters.ID, projectRoot, sessionDirectory string) error {
	if adapter != adapters.OpenCode {
		return nil
	}
	if err := SessionDirectoryMatches(projectRoot, sessionDirectory); err != nil {
		return fmt.Errorf("%w: %v", errAdmitProject, err)
	}
	return nil
}

// SessionDirectoryMatches reports whether sessionDirectory is the same
// directory as projectRoot after absolute clean and symlink resolution.
func SessionDirectoryMatches(projectRoot, sessionDirectory string) error {
	root, err := canonicalExistingDir(projectRoot)
	if err != nil {
		return fmt.Errorf("project root: %w", err)
	}
	session, err := canonicalExistingDir(sessionDirectory)
	if err != nil {
		return fmt.Errorf("session directory: %w", err)
	}
	if root != session {
		return fmt.Errorf("session directory is not the project root")
	}
	return nil
}

func canonicalExistingDir(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "\x00") {
		return "", fmt.Errorf("missing")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
