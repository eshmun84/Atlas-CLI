package context

import "github.com/eshmun84/Atlas-CLI/internal/project"

// MaxFileBytes skips individual files larger than this threshold (512 KiB).
// Canonical source-size ceiling lives in project.MaxSourceFileBytes.
const MaxFileBytes = project.MaxSourceFileBytes

// MaxIndexedEntries caps how many file entries the index stores.
const MaxIndexedEntries = 2000

// ShouldIgnoreDir reports whether a directory basename should be skipped.
// Compatibility wrapper: canonical policy is project.ShouldIgnoreDir.
func ShouldIgnoreDir(name string) bool {
	return project.ShouldIgnoreDir(name)
}

// ShouldIgnoreRel reports whether a project-relative path should be ignored.
// Compatibility wrapper: canonical policy is project.ShouldIgnoreRel.
func ShouldIgnoreRel(rel string) bool {
	return project.ShouldIgnoreRel(rel)
}
