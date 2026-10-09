package context

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

// ContextIndexRel returns the Home-relative canonical index.yaml path.
func ContextIndexRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirContext, FileIndexYAML))
}

// ContextCapsuleRel returns the Home-relative canonical capsule.md path.
func ContextCapsuleRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirContext, FileCapsuleMD))
}

// ContextPackRel returns the Home-relative pack YAML path under canonical layout.
func ContextPackRel(projectID, packID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirContext, DirPacks, packID+".yaml"))
}

// LegacyContextIndexRel returns the transitional Home-relative index path.
func LegacyContextIndexRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(LegacyContextProjectsRel, projectID, FileIndexYAML))
}

// LegacyContextCapsuleRel returns the transitional Home-relative capsule path.
func LegacyContextCapsuleRel(projectID string) string {
	return filepath.ToSlash(filepath.Join(LegacyContextProjectsRel, projectID, FileCapsuleMD))
}

// InspectContextLeaf reports whether a Home-relative context file is a safe
// regular non-symlink file. Missing => present=false,nil. Unsafe => error.
func InspectContextLeaf(homePath, rel string) (present bool, err error) {
	info, err := fsafety.LstatContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("context: not a regular file: %s", rel)
	}
	return true, nil
}

// LoadIndexAt loads index.yaml under explicit Atlas Home (canonical, then legacy).
// Missing both => (zero, false, nil). Unsafe symlink/containment => error.
func LoadIndexAt(homePath, projectID string) (IndexDocument, bool, error) {
	for _, rel := range []string{ContextIndexRel(projectID), LegacyContextIndexRel(projectID)} {
		data, err := fsafety.ReadFileContained(homePath, rel)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return IndexDocument{}, false, fmt.Errorf("context index: %w", err)
		}
		var idx IndexDocument
		if err := yaml.Unmarshal(data, &idx); err != nil {
			return IndexDocument{}, false, fmt.Errorf("context index: parse %s: %w", rel, err)
		}
		return idx, true, nil
	}
	return IndexDocument{}, false, nil
}

// resolveContextRels picks canonical vs legacy Home-relative paths for inspect.
// Prefers canonical when its index leaf is present or unsafe (unsafe surfaces).
// Falls back to legacy only when canonical index is cleanly absent.
func resolveContextRels(homePath, projectID string) (indexRel, capsuleRel string, err error) {
	canonIdx := ContextIndexRel(projectID)
	present, err := InspectContextLeaf(homePath, canonIdx)
	if err != nil {
		return canonIdx, ContextCapsuleRel(projectID), err
	}
	if present {
		return canonIdx, ContextCapsuleRel(projectID), nil
	}
	legacyIdx := LegacyContextIndexRel(projectID)
	legacyPresent, legacyErr := InspectContextLeaf(homePath, legacyIdx)
	if legacyErr != nil {
		return legacyIdx, LegacyContextCapsuleRel(projectID), legacyErr
	}
	if legacyPresent {
		return legacyIdx, LegacyContextCapsuleRel(projectID), nil
	}
	return canonIdx, ContextCapsuleRel(projectID), nil
}
