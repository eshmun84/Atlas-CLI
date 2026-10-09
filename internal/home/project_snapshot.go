package home

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// ProjectLayoutSnapshot captures project-scoped Home layout/identity/local state
// before a mutating flow (Init/Repair/Context) creates or replaces them.
// Footprint records Phase B writes/dirs for identity-aware rollback.
type ProjectLayoutSnapshot struct {
	HomePath           string
	ProjectID          string
	ProjectRootExisted bool
	ProjectRootMode    os.FileMode
	ProjectRootInfo    os.FileInfo     // baseline identity when ProjectRootExisted
	DirsExisted        map[string]bool // relative to projects/<id>/
	DirModes           map[string]os.FileMode
	DirInfos           map[string]os.FileInfo // baseline identity when DirsExisted
	Identity           fsafety.FileSnapshot
	LocalState         fsafety.FileSnapshot
	Footprint          fsafety.TransactionFootprint
}

// Test seams (nil in production).
var (
	captureFileSnapshotFn = fsafety.CaptureFileSnapshot
	lstatFn               = os.Lstat
	chmodFn               = os.Chmod
)

// CaptureProjectLayoutSnapshot records project Home existence before mutation.
// Read-only: never creates directories.
// Fail-closed: unexpected Lstat/read errors abort; only os.IsNotExist means absent.
func CaptureProjectLayoutSnapshot(homePath, projectID string) (ProjectLayoutSnapshot, error) {
	snap := ProjectLayoutSnapshot{
		HomePath:    homePath,
		ProjectID:   projectID,
		DirsExisted: map[string]bool{},
		DirModes:    map[string]os.FileMode{},
		DirInfos:    map[string]os.FileInfo{},
	}
	if homePath == "" || projectID == "" {
		return snap, nil
	}
	root := ProjectRoot(homePath, projectID)
	existed, mode, info, err := inspectRealDir(root)
	if err != nil {
		return snap, fmt.Errorf("atlas home: project root: %w", err)
	}
	snap.ProjectRootExisted = existed
	snap.ProjectRootMode = mode
	snap.ProjectRootInfo = info

	dirs := append([]string{}, ProjectLayoutDirectories...)
	dirs = append(dirs, "codegraph", "mcp")
	for _, dir := range dirs {
		abs := filepath.Join(root, dir)
		existed, mode, info, err := inspectRealDir(abs)
		if err != nil {
			return snap, fmt.Errorf("atlas home: project dir %s: %w", dir, err)
		}
		snap.DirsExisted[dir] = existed
		if existed {
			snap.DirModes[dir] = mode
			snap.DirInfos[dir] = info
		}
	}
	idRel := filepath.ToSlash(filepath.Join(DirProjects, projectID, FileProjectIdentity))
	idSnap, err := captureFileSnapshotFn(homePath, idRel)
	if err != nil {
		return snap, fmt.Errorf("atlas home: identity snapshot: %w", err)
	}
	snap.Identity = idSnap
	localRel := filepath.ToSlash(filepath.Join(DirProjects, projectID, FileProjectLocal))
	localSnap, err := captureFileSnapshotFn(homePath, localRel)
	if err != nil {
		return snap, fmt.Errorf("atlas home: local state snapshot: %w", err)
	}
	snap.LocalState = localSnap
	return snap, nil
}

// inspectRealDir classifies a path that must be a real directory when present.
// missing → false,0,nil,nil; real dir → true,mode,info,nil; symlink/file/other → error.
func inspectRealDir(abs string) (existed bool, mode os.FileMode, info os.FileInfo, err error) {
	info, err = lstatFn(abs)
	if os.IsNotExist(err) {
		return false, 0, nil, nil
	}
	if err != nil {
		return false, 0, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return false, 0, nil, fmt.Errorf("symlink where real directory required: %s", abs)
	}
	if !info.IsDir() {
		return false, 0, nil, fmt.Errorf("not a directory: %s", abs)
	}
	return true, info.Mode().Perm(), info, nil
}

// RestoreProjectLayoutSnapshot restores identity/local state and removes only
// empty directories recorded in Footprint.createdDirs (identity-aware).
// Never uses os.RemoveAll. Prior directory modes are restored when baseline existed.
func RestoreProjectLayoutSnapshot(snap ProjectLayoutSnapshot) error {
	return restoreProjectLayoutSnapshot(snap, true)
}

// RestoreProjectLayoutSnapshotFiles restores footprint files and prior dir modes
// without removing createdDirs. Used when attempt-backup stamps must remain for
// RemoveContainedRel (SAFE_TO_KEEP) before empty-dir teardown.
func RestoreProjectLayoutSnapshotFiles(snap ProjectLayoutSnapshot) error {
	return restoreProjectLayoutSnapshot(snap, false)
}

func restoreProjectLayoutSnapshot(snap ProjectLayoutSnapshot, removeCreatedDirs bool) error {
	if snap.HomePath == "" || snap.ProjectID == "" {
		return nil
	}
	var errs []string

	// Files first (stable Rel): baseline identity/local, or remove transaction-created files.
	for _, w := range snap.Footprint.SortedFiles() {
		baseline := fsafety.FileSnapshot{Rel: w.Rel, Exists: false}
		switch w.Rel {
		case snap.Identity.Rel:
			baseline = snap.Identity
		case snap.LocalState.Rel:
			baseline = snap.LocalState
		}
		ww := w
		if err := fsafety.RestoreFileSnapshot(snap.HomePath, baseline, &ww, snap.Footprint.DeletedByRel(w.Rel)); err != nil {
			errs = append(errs, err.Error())
		}
	}
	// Also restore identity/local when present at baseline but not rewritten (no footprint entry).
	if snap.Identity.Rel != "" && snap.Footprint.FileByRel(snap.Identity.Rel) == nil {
		if err := fsafety.RestoreFileSnapshot(snap.HomePath, snap.Identity, nil, snap.Footprint.DeletedByRel(snap.Identity.Rel)); err != nil {
			errs = append(errs, fmt.Sprintf("identity: %v", err))
		}
	}
	if snap.LocalState.Rel != "" && snap.Footprint.FileByRel(snap.LocalState.Rel) == nil {
		if err := fsafety.RestoreFileSnapshot(snap.HomePath, snap.LocalState, nil, snap.Footprint.DeletedByRel(snap.LocalState.Rel)); err != nil {
			errs = append(errs, fmt.Sprintf("local state: %v", err))
		}
	}

	if removeCreatedDirs {
		if err := fsafety.RestoreCreatedDirs(snap.HomePath, snap.Footprint); err != nil {
			errs = append(errs, err.Error())
		}
	}

	root := ProjectRoot(snap.HomePath, snap.ProjectID)
	keys := make([]string, 0, len(snap.DirsExisted))
	for k := range snap.DirsExisted {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, dir := range keys {
		if !snap.DirsExisted[dir] {
			continue
		}
		mode, ok := snap.DirModes[dir]
		if !ok || mode == 0 {
			continue
		}
		abs := filepath.Join(root, dir)
		if err := restoreDirModeIdentified(abs, snap.DirInfos[dir], mode); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", dir, err))
		}
	}
	if snap.ProjectRootExisted && snap.ProjectRootMode != 0 {
		if err := restoreDirModeIdentified(root, snap.ProjectRootInfo, snap.ProjectRootMode); err != nil {
			errs = append(errs, fmt.Sprintf("project root: %v", err))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("project layout restore: %s", strings.Join(errs, "; "))
}

// restoreDirModeIdentified chmods only after SameFile identity with baseline.
func restoreDirModeIdentified(abs string, baselineInfo os.FileInfo, mode os.FileMode) error {
	info, err := fsafety.VerifyDirIdentity(abs, baselineInfo)
	if err != nil {
		return err
	}
	if mode == 0 {
		return fmt.Errorf("mode is required")
	}
	if info.Mode().Perm() == mode.Perm() {
		return nil
	}
	if err := chmodFn(abs, mode); err != nil {
		return fmt.Errorf("chmod %04o: %w", mode, err)
	}
	return nil
}

// RemoveContainedRel removes homePath/rel after ContainedJoin when the relative
// path is under projects/<id>/backups/ or projects/<id>/mcp/backups/.
// Used to discard a backup directory created solely by a failed mutation attempt
// after a successful rollback.
//
// Classification: SAFE_TO_KEEP (allowlisted attempt-backup stamp only).
func RemoveContainedRel(homePath, rel string) error {
	homePath = strings.TrimSpace(homePath)
	rel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	if homePath == "" || rel == "" || rel == "." {
		return fmt.Errorf("atlas home: remove requires home path and relative path")
	}
	if !strings.HasPrefix(rel, DirProjects+"/") {
		return fmt.Errorf("atlas home: refused remove outside projects/: %s", rel)
	}
	parts := strings.Split(rel, "/")
	// projects/<id>/backups/<stamp> or projects/<id>/mcp/backups/<stamp>[/...]
	if len(parts) < 4 {
		return fmt.Errorf("atlas home: refused shallow remove: %s", rel)
	}
	ok := (parts[2] == ProjectDirBackups) ||
		(len(parts) >= 5 && parts[2] == "mcp" && parts[3] == "backups")
	if !ok {
		return fmt.Errorf("atlas home: refused non-backup remove: %s", rel)
	}
	full, err := fsafety.ContainedJoin(homePath, rel)
	if err != nil {
		return err
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("atlas home: refused remove symlink %s", rel)
	}
	if info.IsDir() {
		return os.RemoveAll(full)
	}
	return os.Remove(full)
}
