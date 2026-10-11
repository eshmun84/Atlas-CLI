package hookproj

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// MutationState retains projection + ownership evidence for outer rollback.
//
// Touched is set when any filesystem mutation evidence is recorded (baselines /
// footprints), independent of overall reconcile success.
// Applied is set only after the full reconcile commits successfully.
// Rollback restores when Touched || Applied.
type MutationState struct {
	Root      string
	HomePath  string
	ProjectID string
	Touched   bool
	Applied   bool

	ProjectBaselines []fsafety.FileSnapshot
	ProjectFootprint fsafety.TransactionFootprint

	OwnershipExists bool
	OwnershipRaw    []byte
	OwnershipMode   os.FileMode
	HomeFootprint   fsafety.TransactionFootprint
}

// noteTouch records that mutation evidence exists and rollback may be required.
func (m *MutationState) noteTouch() {
	if m != nil {
		m.Touched = true
	}
}

// Rollback restores project hook projections and hooks ownership baseline.
// Safe and idempotent: a second call after a successful restore is a no-op.
func (m *MutationState) Rollback() error {
	if m == nil || (!m.Touched && !m.Applied) {
		return nil
	}
	var errs []string

	bases := append([]fsafety.FileSnapshot(nil), m.ProjectBaselines...)
	sort.Slice(bases, func(i, j int) bool { return bases[i].Rel < bases[j].Rel })
	for _, snap := range bases {
		wrote := m.ProjectFootprint.FileByRel(snap.Rel)
		deleted := m.ProjectFootprint.DeletedByRel(snap.Rel)
		if err := fsafety.RestoreFileSnapshot(m.Root, snap, wrote, deleted); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", snap.Rel, err))
		}
	}
	if err := fsafety.RestoreCreatedDirs(m.Root, m.ProjectFootprint); err != nil {
		errs = append(errs, err.Error())
	}

	if m.HomePath != "" && m.ProjectID != "" {
		if err := restoreOwnershipBaseline(m.HomePath, m.ProjectID, m.OwnershipExists, m.OwnershipRaw, m.OwnershipMode, &m.HomeFootprint); err != nil {
			errs = append(errs, err.Error())
		}
		if err := fsafety.RestoreCreatedDirs(m.HomePath, m.HomeFootprint); err != nil {
			errs = append(errs, err.Error())
		}
	}

	if len(errs) == 0 {
		m.Touched = false
		m.Applied = false
		return nil
	}
	return fmt.Errorf("hooks rollback: %s", strings.Join(errs, "; "))
}

func captureOwnershipBaseline(homePath, projectID string) (exists bool, raw []byte, mode os.FileMode, err error) {
	if homePath == "" || projectID == "" {
		return false, nil, 0, nil
	}
	rel := OwnershipRelPath(projectID)
	info, err := fsafety.LstatContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil, 0, nil
		}
		return false, nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return false, nil, 0, fmt.Errorf("hooks ownership: not a regular file: %s", rel)
	}
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		return false, nil, 0, err
	}
	return true, append([]byte(nil), data...), info.Mode().Perm(), nil
}

func restoreOwnershipBaseline(homePath, projectID string, exists bool, raw []byte, mode os.FileMode, fp *fsafety.TransactionFootprint) error {
	if homePath == "" || projectID == "" {
		return nil
	}
	rel := OwnershipRelPath(projectID)
	baseline := fsafety.FileSnapshot{
		Rel:     rel,
		Exists:  exists,
		Data:    append([]byte(nil), raw...),
		Mode:    mode,
		WasFile: true,
	}
	var wrote *fsafety.WrittenFile
	var deleted *fsafety.DeletedFile
	if fp != nil {
		wrote = fp.FileByRel(rel)
		deleted = fp.DeletedByRel(rel)
	}
	if err := fsafety.RestoreFileSnapshot(homePath, baseline, wrote, deleted); err != nil {
		return fmt.Errorf("hooks ownership restore: %w", err)
	}
	return nil
}

func mergeBaseline(list *[]fsafety.FileSnapshot, snap fsafety.FileSnapshot) {
	rel := filepath.ToSlash(filepath.Clean(snap.Rel))
	snap.Rel = rel
	for i := range *list {
		if (*list)[i].Rel == rel {
			return
		}
	}
	*list = append(*list, snap)
}
