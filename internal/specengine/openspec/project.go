package openspec

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// ClassifyProject classifies the OpenSpec project tree without mutating it.
//
//	absent       — no openspec/ directory
//	ready        — real OpenSpec init shape: config.yaml + specs/ + changes/ + changes/archive/
//	legacy_mkdir — Atlas-era openspec/ + openspec/changes/ without config/specs/archive
//	malformed    — partial/incompatible/ambiguous tree (including structural symlinks)
//
// .gitkeep placeholders are not required.
func ClassifyProject(root string) (ProjectInitState, string) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return ProjectMalformed, "project root is required"
	}

	info, err := fsafety.LstatContained(root, rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectAbsent, ""
		}
		return ProjectMalformed, "openspec root inspection failed: " + err.Error()
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ProjectMalformed, "openspec root is a symlink"
	}
	if !info.IsDir() {
		return ProjectMalformed, "openspec root exists but is not a directory"
	}

	hasConfig, cfgReason := inspectRegularFile(root, configFile)
	if cfgReason != "" {
		return ProjectMalformed, cfgReason
	}
	hasSpecs, specsReason := inspectRealDir(root, specsDir)
	if specsReason != "" {
		return ProjectMalformed, specsReason
	}
	hasChanges, changesReason := inspectRealDir(root, changesDir)
	if changesReason != "" {
		return ProjectMalformed, changesReason
	}
	hasArchive, archiveReason := inspectRealDir(root, archiveDir)
	if archiveReason != "" {
		return ProjectMalformed, archiveReason
	}

	switch {
	case hasConfig && hasSpecs && hasChanges && hasArchive:
		return ProjectReady, ""
	case !hasConfig && hasChanges && !hasSpecs && !hasArchive:
		// Historical Atlas EnsureBootstrap created only openspec/ + changes/.
		return ProjectLegacyMkdir, "openspec/ exists without config.yaml (legacy Atlas mkdir bootstrap)"
	case hasConfig && hasChanges && !hasSpecs && !hasArchive:
		return ProjectMalformed, "openspec/config.yaml and changes/ present without specs/ and changes/archive/ (not a complete OpenSpec init)"
	case hasConfig && hasChanges && !hasSpecs:
		return ProjectMalformed, "openspec/specs/ missing"
	case hasConfig && hasChanges && !hasArchive:
		return ProjectMalformed, "openspec/changes/archive/ missing"
	case hasConfig && !hasChanges:
		return ProjectMalformed, "openspec/config.yaml present without changes directory"
	default:
		return ProjectMalformed, "incomplete or ambiguous OpenSpec project tree"
	}
}

// AssessLayout remains for callers that need Absent/Valid/Incompatible.
// Valid means a complete real OpenSpec init shape, not mkdir-only or partial trees.
func AssessLayout(root string) (LayoutAssessment, error) {
	st, reason := ClassifyProject(root)
	switch st {
	case ProjectAbsent:
		return LayoutAssessment{Absent: true}, nil
	case ProjectReady:
		return LayoutAssessment{Valid: true}, nil
	case ProjectLegacyMkdir:
		return LayoutAssessment{Incompatible: true, Reason: reason}, nil
	default:
		return LayoutAssessment{Incompatible: true, Reason: reason}, nil
	}
}

func inspectRegularFile(root, rel string) (present bool, malformedReason string) {
	info, err := fsafety.LstatContained(root, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, ""
		}
		return false, rel + " inspection failed: " + err.Error()
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, rel + " is a symlink"
	}
	if info.IsDir() {
		return false, rel + " is not a regular file"
	}
	return true, ""
}

func inspectRealDir(root, rel string) (present bool, malformedReason string) {
	info, err := fsafety.LstatContained(root, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, ""
		}
		return false, rel + " inspection failed: " + err.Error()
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, rel + " is a symlink"
	}
	if !info.IsDir() {
		return false, rel + " is not a directory"
	}
	return true, ""
}
