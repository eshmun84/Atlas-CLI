package context

import (
	"path/filepath"
	"strings"
)

// MaxFileBytes skips individual files larger than this threshold (512 KiB).
const MaxFileBytes = 512 * 1024

// MaxIndexedEntries caps how many file entries the index stores.
const MaxIndexedEntries = 2000

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

// ShouldIgnoreDir reports whether a directory basename should be skipped.
func ShouldIgnoreDir(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if ignoredDirNames[n] {
		return true
	}
	return false
}

// ShouldIgnoreRel reports whether a project-relative path should be ignored.
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
