package codeintel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project"
)

// Testable seams; production uses filepath.Walk / filepath.Rel.
var (
	fingerprintWalkFn = filepath.Walk
	fingerprintRelFn  = filepath.Rel
)

// SourceFingerprint is a deterministic content-based hash of relevant project sources.
type SourceFingerprint struct {
	Value   string
	GitHEAD string
	Files   int
}

// ComputeSourceFingerprint hashes every relevant working-tree file.
// Git HEAD (when present) is included so clean commits are distinct; dirty and
// untracked content still change the hash because file bodies are hashed from
// the working tree. Ignores vendor/generated/runtime paths via project policy.
// Relevant symlinks are refused. Unexpected Walk/Rel/hash errors and disappearing
// encountered entries fail closed — no incomplete freshness evidence.
func ComputeSourceFingerprint(root string) (SourceFingerprint, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return SourceFingerprint{}, fmt.Errorf("codeintel: fingerprint root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return SourceFingerprint{}, fmt.Errorf("codeintel: resolve root: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return SourceFingerprint{}, fmt.Errorf("codeintel: stat root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return SourceFingerprint{}, fmt.Errorf("codeintel: fingerprint root is a symlink")
	}
	if !info.IsDir() {
		return SourceFingerprint{}, fmt.Errorf("codeintel: root is not a directory")
	}

	h := sha256.New()
	head := gitHEAD(abs)
	fmt.Fprintf(h, "git-head\x00%s\n", head)

	type entry struct {
		rel  string
		hash string
	}
	var entries []entry
	err = fingerprintWalkFn(abs, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := fingerprintRelFn(abs, path)
		if relErr != nil {
			return fmt.Errorf("rel %s: %w", path, relErr)
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		base := filepath.Base(path)
		if fi.IsDir() {
			if project.ShouldSkipSourceDir(base, rel) {
				return filepath.SkipDir
			}
			return nil
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			if project.IsRelevantSourceFile(rel, fi.Size()) {
				return fmt.Errorf("codeintel: refusing symlink source %s", rel)
			}
			return nil
		}
		if !project.IsRelevantSourceFile(rel, fi.Size()) {
			return nil
		}
		sum, hashErr := hashFile(path)
		if hashErr != nil {
			return fmt.Errorf("hash %s: %w", rel, hashErr)
		}
		entries = append(entries, entry{rel: rel, hash: sum})
		return nil
	})
	if err != nil {
		return SourceFingerprint{}, fmt.Errorf("codeintel: walk sources: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].rel < entries[j].rel })
	for _, e := range entries {
		fmt.Fprintf(h, "f\x00%s\x00%s\n", e.rel, e.hash)
	}
	return SourceFingerprint{
		Value:   hex.EncodeToString(h.Sum(nil)),
		GitHEAD: head,
		Files:   len(entries),
	}, nil
}

func hashFile(path string) (string, error) {
	// Re-check leaf is still a regular non-symlink file before open.
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing symlink")
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, project.MaxSourceFileBytes+1)); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func gitHEAD(root string) string {
	git, err := exec.LookPath("git")
	if err != nil {
		return ""
	}
	cmd := exec.Command(git, "rev-parse", "HEAD")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
