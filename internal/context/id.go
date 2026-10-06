package context

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
)

var nonID = regexp.MustCompile(`[^a-z0-9]+`)

// ProjectID returns a stable identity for a project root.
// Format: <sanitized-basename>-<sha256(absRoot)[:16]>.
func ProjectID(root, projectName string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	sum := sha256.Sum256([]byte(abs))
	hash := hex.EncodeToString(sum[:])[:16]

	base := strings.TrimSpace(projectName)
	if base == "" {
		base = filepath.Base(abs)
	}
	base = strings.ToLower(base)
	base = nonID.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "project"
	}
	if len(base) > 40 {
		base = base[:40]
		base = strings.Trim(base, "-")
	}
	return base + "-" + hash, nil
}
