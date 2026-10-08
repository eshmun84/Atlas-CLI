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

const fingerprintMaxFiles = 4000

// SourceFingerprint is a deterministic content-based hash of relevant project sources.
type SourceFingerprint struct {
	Value   string
	GitHEAD string
	Files   int
}

// ComputeSourceFingerprint hashes relevant working-tree file contents.
// Git HEAD (when present) is included so clean commits are distinct; dirty and
// untracked content still change the hash because file bodies are hashed from
// the working tree. Ignores vendor/generated/runtime paths via project policy.
func ComputeSourceFingerprint(root string) (SourceFingerprint, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return SourceFingerprint{}, fmt.Errorf("codeintel: fingerprint root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return SourceFingerprint{}, fmt.Errorf("codeintel: resolve root: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return SourceFingerprint{}, fmt.Errorf("codeintel: stat root: %w", err)
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
	fileCount := 0
	err = filepath.Walk(abs, func(path string, fi os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(abs, path)
		if relErr != nil {
			return nil
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
		if !project.IsRelevantSourceFile(rel, fi.Size()) {
			return nil
		}
		sum, hashErr := hashFile(path)
		if hashErr != nil {
			return nil
		}
		entries = append(entries, entry{rel: rel, hash: sum})
		fileCount++
		if fileCount >= fingerprintMaxFiles {
			return io.EOF
		}
		return nil
	})
	if err != nil && err != io.EOF {
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
