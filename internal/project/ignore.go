package project

import (
	"path/filepath"
	"strings"
)

// MaxSourceFileBytes is the neutral size ceiling for source-relevant files
// (fingerprint / inspection walks). Context Economy may keep its own index caps.
const MaxSourceFileBytes = 512 * 1024

var ignoredDirNames = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	"build":        true,
	"target":       true,
	".next":        true,
	".nuxt":        true,
	".cache":       true,
	"coverage":     true,
	"tmp":          true,
	"temp":         true,
	"__pycache__":  true,
	".tox":         true,
	".venv":        true,
	"venv":         true,
	".idea":        true,
	".vscode":      true,
}

var ignoredExts = map[string]bool{
	".exe":   true,
	".dll":   true,
	".so":    true,
	".dylib": true,
	".a":     true,
	".o":     true,
	".bin":   true,
	".class": true,
	".jar":   true,
	".war":   true,
	".zip":   true,
	".tar":   true,
	".gz":    true,
	".tgz":   true,
	".bz2":   true,
	".xz":    true,
	".7z":    true,
	".rar":   true,
	".png":   true,
	".jpg":   true,
	".jpeg":  true,
	".gif":   true,
	".webp":  true,
	".ico":   true,
	".pdf":   true,
	".mp4":   true,
	".mp3":   true,
	".wav":   true,
	".mov":   true,
	".lock":  false, // keep lockfiles (go.sum, package-lock.json handled by name)
	".log":   true,
	".tmp":   true,
	".swp":   true,
	".swo":   true,
}

// runtimeOwnedDirNames are Atlas / adapter runtime trees that are not product source.
var runtimeOwnedDirNames = map[string]bool{
	".atlas":     true,
	".codegraph": true,
	".cursor":    true,
	".opencode":  true,
	".agents":    true,
	".claude":    true,
}

// ShouldIgnoreDir reports whether a directory basename should be skipped by
// generic project walks (shared by Context Economy indexing and source selection).
func ShouldIgnoreDir(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	return ignoredDirNames[n]
}

// ShouldIgnoreRel reports whether a project-relative path should be ignored by
// generic project walks.
func ShouldIgnoreRel(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if clean == "." || clean == "" {
		return false
	}
	if strings.HasPrefix(clean, ".atlas/backups/") || clean == ".atlas/backups" {
		return true
	}
	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if ShouldIgnoreDir(part) {
			return true
		}
	}
	base := strings.ToLower(filepath.Base(clean))
	if strings.HasSuffix(base, ".log") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(base))
	if ignoredExts[ext] {
		return true
	}
	return false
}

// ShouldSkipSourceDir reports whether a directory should be skipped when
// selecting product source files (e.g. Code Intelligence fingerprint).
// Includes generic ignores plus Atlas/adapter runtime trees.
func ShouldSkipSourceDir(name, rel string) bool {
	if ShouldIgnoreDir(name) {
		return true
	}
	if runtimeOwnedDirNames[strings.ToLower(strings.TrimSpace(name))] {
		return true
	}
	return ShouldIgnoreRel(rel)
}

// IsRelevantSourceFile reports whether a file should contribute to source
// fingerprint / source-selection walks.
func IsRelevantSourceFile(rel string, size int64) bool {
	if ShouldIgnoreRel(rel) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(rel))
	parts := strings.Split(clean, "/")
	for _, part := range parts {
		if runtimeOwnedDirNames[strings.ToLower(part)] {
			return false
		}
	}
	if size > MaxSourceFileBytes {
		return false
	}
	if strings.ToLower(filepath.Base(clean)) == "agents.md" {
		return false
	}
	return true
}
