package assets

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// BundledSkillEmbedFiles returns embed-relative paths for all shipped skill files.
func BundledSkillEmbedFiles() ([]string, error) {
	var out []string
	err := fs.WalkDir(Content, "skills", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		out = append(out, filepath.ToSlash(path))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("assets: list skills: %w", err)
	}
	sort.Strings(out)
	return out, nil
}

// IsSkillEmbedPath reports whether path is under skills/.
func IsSkillEmbedPath(path string) bool {
	return strings.HasPrefix(filepath.ToSlash(path), "skills/")
}
