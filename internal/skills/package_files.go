package skills

import (
	"fmt"
	"path/filepath"
	"strings"
)

// IsPackageFileRel reports whether rel (package-relative, slash-separated) is
// part of the canonical integrity/projection set:
// SKILL.md, references/**, scripts/**, assets/**
func IsPackageFileRel(rel string) bool {
	rel = filepath.ToSlash(strings.TrimSpace(rel))
	if rel == "" || rel == "." || strings.Contains(rel, "..") {
		return false
	}
	if rel == FileSkillMD {
		return true
	}
	top, _, ok := strings.Cut(rel, "/")
	if !ok {
		return false // other top-level file
	}
	switch top {
	case DirReferences, DirScripts, DirAssets:
		return true
	default:
		return false
	}
}

// IsSupportedPackageTopLevel reports whether a top-level name is allowed.
func IsSupportedPackageTopLevel(name string) bool {
	switch name {
	case FileSkillMD, DirReferences, DirScripts, DirAssets:
		return true
	default:
		return false
	}
}

// FilterPackageFiles keeps only canonical package file entries from a map.
func FilterPackageFiles(files map[string][]byte) (map[string][]byte, error) {
	out := make(map[string][]byte, len(files))
	for rel, data := range files {
		rel = filepath.ToSlash(rel)
		if !IsPackageFileRel(rel) {
			return nil, fmt.Errorf("skills: unsupported package file %q", rel)
		}
		out[rel] = data
	}
	if _, ok := out[FileSkillMD]; !ok {
		return nil, fmt.Errorf("skills: package missing %s", FileSkillMD)
	}
	return out, nil
}
