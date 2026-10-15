package openspec

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// BootstrapResult summarizes OpenSpec layout ensure for Init Apply.
type BootstrapResult struct {
	Created   bool
	Paths     []string
	Footprint fsafety.TransactionFootprint
}

// LayoutAssessment is a read-only classification of openspec/ under root.
type LayoutAssessment struct {
	Absent       bool
	Valid        bool
	Incompatible bool
	Reason       string
}

// AssessLayout classifies the OpenSpec tree without mutating it.
// Valid means openspec/ and openspec/changes/ exist as real directories
// (the minimum the Atlas adapter treats as initialized, not incomplete).
func AssessLayout(root string) (LayoutAssessment, error) {
	root = filepath.Clean(root)
	if root == "" || root == "." {
		return LayoutAssessment{}, fmt.Errorf("openspec: root is required")
	}

	info, err := fsafety.LstatContained(root, rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return LayoutAssessment{Absent: true}, nil
		}
		return LayoutAssessment{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return LayoutAssessment{
			Incompatible: true,
			Reason:       "openspec root is a symlink",
		}, nil
	}
	if !info.IsDir() {
		return LayoutAssessment{
			Incompatible: true,
			Reason:       "openspec root exists but is not a directory",
		}, nil
	}

	changesInfo, err := fsafety.LstatContained(root, changesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return LayoutAssessment{
				Incompatible: true,
				Reason:       "openspec root present without changes directory",
			}, nil
		}
		return LayoutAssessment{}, err
	}
	if changesInfo.Mode()&os.ModeSymlink != 0 {
		return LayoutAssessment{
			Incompatible: true,
			Reason:       "openspec/changes is a symlink",
		}, nil
	}
	if !changesInfo.IsDir() {
		return LayoutAssessment{
			Incompatible: true,
			Reason:       "openspec/changes exists but is not a directory",
		}, nil
	}
	return LayoutAssessment{Valid: true}, nil
}

// EnsureBootstrap creates the minimum OpenSpec layout when absent.
// If a valid layout already exists, it is a no-op.
// If a partial or incompatible layout exists, it fails closed without mutation.
//
// Current/temporary: Atlas still creates empty openspec/ + openspec/changes/
// for governed SDD readiness. Block B will move openspec/ lifecycle ownership
// toward the real OpenSpec provider and stop treating Atlas mkdir as the
// long-term architecture. Do not expand bootstrap semantics here.
func EnsureBootstrap(root string) (BootstrapResult, error) {
	assess, err := AssessLayout(root)
	if err != nil {
		return BootstrapResult{}, err
	}
	switch {
	case assess.Valid:
		return BootstrapResult{}, nil
	case assess.Incompatible:
		return BootstrapResult{}, fmt.Errorf("openspec: incompatible existing layout: %s", assess.Reason)
	case !assess.Absent:
		return BootstrapResult{}, fmt.Errorf("openspec: unexpected layout state")
	}

	var result BootstrapResult
	for _, dir := range []string{rootDir, changesDir} {
		dirs, mkdirErr := fsafety.SafeMkdirAllCreated(root, dir, 0o755)
		result.Footprint.MergeDirs(dirs)
		if mkdirErr != nil {
			_ = fsafety.RestoreCreatedDirs(root, result.Footprint)
			return BootstrapResult{}, fmt.Errorf("openspec: create %s: %w", dir, mkdirErr)
		}
		result.Paths = append(result.Paths, dir)
	}
	result.Created = true

	// Re-assess to confirm adapter-compatible layout.
	after, err := AssessLayout(root)
	if err != nil || !after.Valid {
		_ = fsafety.RestoreCreatedDirs(root, result.Footprint)
		if err != nil {
			return BootstrapResult{}, err
		}
		return BootstrapResult{}, fmt.Errorf("openspec: bootstrap did not produce a valid layout")
	}
	return result, nil
}
