package fsafety

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// RollbackConflictError reports a TOCTOU / footprint mismatch without performing
// a destructive action.
type RollbackConflictError struct {
	Rel      string
	Expected string
	Observed string
	Reason   string
}

func (e *RollbackConflictError) Error() string {
	return fmt.Sprintf("rollback conflict: %s: %s (expected %s; observed %s)", e.Rel, e.Reason, e.Expected, e.Observed)
}

func conflict(rel, reason, expected, observed string) error {
	return &RollbackConflictError{Rel: rel, Reason: reason, Expected: expected, Observed: observed}
}

// WrittenFile is one file Atlas successfully wrote during a transaction.
type WrittenFile struct {
	Rel      string
	Created  bool // true when the path was absent before this write
	Expected []byte
	Mode     os.FileMode
	Info     os.FileInfo // post-write identity for os.SameFile
}

// DeletedFile is one file Atlas deliberately removed during a transaction.
// Rollback may recreate the baseline only when this proof exists.
type DeletedFile struct {
	Rel      string
	Expected []byte      // baseline content at delete time
	Mode     os.FileMode // baseline mode at delete time
	Info     os.FileInfo // baseline identity verified before delete
}

// CreatedDir is one directory Atlas successfully created during a transaction.
type CreatedDir struct {
	Rel  string
	Info os.FileInfo // post-create identity for os.SameFile
}

// TransactionFootprint records Phase B mutations for identity-aware rollback.
type TransactionFootprint struct {
	Files   []WrittenFile
	Deleted []DeletedFile
	Dirs    []CreatedDir
}

// AddFile records or replaces a written-file footprint entry for rel.
func (fp *TransactionFootprint) AddFile(w WrittenFile) {
	if fp == nil {
		return
	}
	rel := filepath.ToSlash(filepath.Clean(w.Rel))
	w.Rel = rel
	for i := range fp.Files {
		if fp.Files[i].Rel == rel {
			fp.Files[i] = w
			return
		}
	}
	fp.Files = append(fp.Files, w)
}

// AddDir records a created directory if not already present.
func (fp *TransactionFootprint) AddDir(d CreatedDir) {
	if fp == nil {
		return
	}
	rel := filepath.ToSlash(filepath.Clean(d.Rel))
	d.Rel = rel
	for _, existing := range fp.Dirs {
		if existing.Rel == rel {
			return
		}
	}
	fp.Dirs = append(fp.Dirs, d)
}

// MergeDirs appends created dirs from another footprint.
func (fp *TransactionFootprint) MergeDirs(dirs []CreatedDir) {
	for _, d := range dirs {
		fp.AddDir(d)
	}
}

// MergeFootprint merges files/dirs/deletes from another footprint (partial progress).
func (fp *TransactionFootprint) MergeFootprint(other TransactionFootprint) {
	if fp == nil {
		return
	}
	for _, w := range other.Files {
		fp.AddFile(w)
	}
	for _, d := range other.Deleted {
		fp.AddDeleted(d)
	}
	fp.MergeDirs(other.Dirs)
}

// AddDeleted records a transactional deletion for rel.
func (fp *TransactionFootprint) AddDeleted(d DeletedFile) {
	if fp == nil {
		return
	}
	rel := filepath.ToSlash(filepath.Clean(d.Rel))
	d.Rel = rel
	for i := range fp.Deleted {
		if fp.Deleted[i].Rel == rel {
			fp.Deleted[i] = d
			return
		}
	}
	fp.Deleted = append(fp.Deleted, d)
}

// FileByRel returns the written-file footprint for rel, if any.
func (fp *TransactionFootprint) FileByRel(rel string) *WrittenFile {
	if fp == nil {
		return nil
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	for i := range fp.Files {
		if fp.Files[i].Rel == rel {
			return &fp.Files[i]
		}
	}
	return nil
}

// DeletedByRel returns the deletion footprint for rel, if any.
func (fp *TransactionFootprint) DeletedByRel(rel string) *DeletedFile {
	if fp == nil {
		return nil
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	for i := range fp.Deleted {
		if fp.Deleted[i].Rel == rel {
			return &fp.Deleted[i]
		}
	}
	return nil
}

// SortedFiles returns written files sorted by Rel (stable rollback order).
func (fp TransactionFootprint) SortedFiles() []WrittenFile {
	out := append([]WrittenFile(nil), fp.Files...)
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out
}

// SortedDirsDeepestFirst returns created dirs deepest-first, then Rel ascending.
func (fp TransactionFootprint) SortedDirsDeepestFirst() []CreatedDir {
	out := append([]CreatedDir(nil), fp.Dirs...)
	sort.SliceStable(out, func(i, j int) bool {
		di, dj := pathDepth(out[i].Rel), pathDepth(out[j].Rel)
		if di != dj {
			return di > dj
		}
		return out[i].Rel < out[j].Rel
	})
	return out
}

func pathDepth(rel string) int {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" || rel == "." {
		return 0
	}
	return strings.Count(rel, "/") + 1
}

// SameFileIdentity reports whether a and b refer to the same filesystem object.
func SameFileIdentity(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return false
	}
	return os.SameFile(a, b)
}

// VerifyDirIdentity checks that abs is still the same real non-symlink directory
// as baselineInfo (os.SameFile). Used before transactional mode restore.
func VerifyDirIdentity(abs string, baselineInfo os.FileInfo) (os.FileInfo, error) {
	abs = filepath.Clean(strings.TrimSpace(abs))
	if abs == "" || abs == "." {
		return nil, fmt.Errorf("fsafety dir identity: path is required")
	}
	if baselineInfo == nil {
		return nil, conflict(abs, "missing baseline directory identity", "baseline directory info", "nil")
	}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		return nil, conflict(abs, "directory missing at mode restore", "baseline directory", "missing")
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, conflict(abs, "symlink where directory expected", "real directory", "symlink")
	}
	if !info.IsDir() {
		return nil, conflict(abs, "not a directory", "directory", info.Mode().String())
	}
	if !SameFileIdentity(baselineInfo, info) {
		return nil, conflict(abs, "directory identity mismatch", "baseline directory object", "different filesystem object")
	}
	return info, nil
}

// RestoreDirMode restores perm on an absolute directory only when VerifyDirIdentity passes.
func RestoreDirMode(abs string, baselineInfo os.FileInfo, mode os.FileMode) error {
	if mode == 0 {
		return fmt.Errorf("fsafety restore dir mode: mode is required for %s", abs)
	}
	info, err := VerifyDirIdentity(abs, baselineInfo)
	if err != nil {
		return err
	}
	if info.Mode().Perm() == mode.Perm() {
		return nil
	}
	if err := os.Chmod(abs, mode); err != nil {
		return fmt.Errorf("fsafety restore dir mode: chmod %s: %w", abs, err)
	}
	return nil
}

// removeFn is the remove primitive for RemoveRegularFileTracked (test-overridable).
var removeFn = os.Remove

// RemoveRegularFileTracked deletes a contained regular file after proving it still
// matches the captured baseline identity/content/mode. Returns deletion footprint
// only when this call's remove succeeds.
func RemoveRegularFileTracked(root string, baseline FileSnapshot) (DeletedFile, error) {
	rel := filepath.ToSlash(filepath.Clean(baseline.Rel))
	if !baseline.Exists || !baseline.WasFile {
		return DeletedFile{}, fmt.Errorf("fsafety delete: baseline must be an existing regular file for %s", rel)
	}
	full, err := ContainedJoin(root, rel)
	if err != nil {
		return DeletedFile{}, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return DeletedFile{}, conflict(rel, "object missing before delete", "baseline regular file", "missing")
		}
		return DeletedFile{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return DeletedFile{}, conflict(rel, "symlink where regular file expected", "regular file", "symlink")
	}
	if !info.Mode().IsRegular() {
		return DeletedFile{}, conflict(rel, "not a regular file", "regular file", info.Mode().String())
	}
	if baseline.Info != nil && !SameFileIdentity(baseline.Info, info) {
		return DeletedFile{}, conflict(rel, "object identity mismatch", "baseline object", "different filesystem object")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return DeletedFile{}, err
	}
	if !bytes.Equal(data, baseline.Data) {
		return DeletedFile{}, conflict(rel, "content mismatch", fmt.Sprintf("%d bytes", len(baseline.Data)), fmt.Sprintf("%d bytes", len(data)))
	}
	wantMode := baseline.Mode.Perm()
	if wantMode == 0 {
		wantMode = 0o644
	}
	if info.Mode().Perm() != wantMode {
		return DeletedFile{}, conflict(rel, "mode mismatch", fmt.Sprintf("%04o", wantMode), fmt.Sprintf("%04o", info.Mode().Perm()))
	}
	if err := removeFn(full); err != nil {
		if os.IsNotExist(err) {
			// Another actor removed the object between verify and remove — no deletion proof.
			return DeletedFile{}, conflict(rel, "object missing at remove", "atlas-removed regular file", "missing")
		}
		return DeletedFile{}, err
	}
	return DeletedFile{
		Rel:      rel,
		Expected: append([]byte(nil), baseline.Data...),
		Mode:     wantMode,
		Info:     info,
	}, nil
}

// VerifyWrittenFile checks current path matches the transaction write footprint.
func VerifyWrittenFile(root string, w WrittenFile) error {
	full, err := ContainedJoin(root, w.Rel)
	if err != nil {
		return err
	}
	info, err := os.Lstat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return conflict(w.Rel, "object missing", "atlas-written regular file", "missing")
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return conflict(w.Rel, "symlink where regular file expected", "regular file", "symlink")
	}
	if !info.Mode().IsRegular() {
		return conflict(w.Rel, "not a regular file", "regular file", info.Mode().String())
	}
	if !SameFileIdentity(w.Info, info) {
		return conflict(w.Rel, "object identity mismatch", "atlas post-write object", "different filesystem object")
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, w.Expected) {
		return conflict(w.Rel, "content mismatch", fmt.Sprintf("%d bytes", len(w.Expected)), fmt.Sprintf("%d bytes", len(data)))
	}
	wantMode := w.Mode.Perm()
	if wantMode == 0 {
		wantMode = 0o644
	}
	if info.Mode().Perm() != wantMode {
		return conflict(w.Rel, "mode mismatch", fmt.Sprintf("%04o", wantMode), fmt.Sprintf("%04o", info.Mode().Perm()))
	}
	return nil
}

// RemoveEmptyCreatedDir removes a transaction-created empty directory after identity checks.
func RemoveEmptyCreatedDir(root string, d CreatedDir) error {
	var full string
	rel := filepath.ToSlash(filepath.Clean(d.Rel))
	if rel == "." {
		// Home/workspace root created by this transaction (Trusted root itself).
		absRoot, err := filepath.Abs(filepath.Clean(strings.TrimSpace(root)))
		if err != nil {
			return err
		}
		full = absRoot
	} else {
		var err error
		full, err = ContainedJoin(root, d.Rel)
		if err != nil {
			return err
		}
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return conflict(d.Rel, "symlink where directory expected", "real directory", "symlink")
	}
	if !info.IsDir() {
		return conflict(d.Rel, "not a directory", "directory", info.Mode().String())
	}
	if !SameFileIdentity(d.Info, info) {
		return conflict(d.Rel, "directory identity mismatch", "atlas post-create directory", "different filesystem object")
	}
	entries, err := os.ReadDir(full)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		sort.Strings(names)
		return conflict(d.Rel, "directory not empty", "empty atlas-created directory", fmt.Sprintf("%d entries: %v", len(entries), names))
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RestoreCreatedDirs removes empty created directories deepest-first.
// On conflict for a child, ancestors are not removed; errors are aggregated.
func RestoreCreatedDirs(root string, fp TransactionFootprint) error {
	var errs []string
	var failed []string
	for _, d := range fp.SortedDirsDeepestFirst() {
		skip := false
		for _, f := range failed {
			if strings.HasPrefix(f, d.Rel+"/") {
				skip = true
				break
			}
		}
		if skip {
			failed = append(failed, d.Rel)
			errs = append(errs, fmt.Sprintf("%s: skipped destructive remove (child conflict)", d.Rel))
			continue
		}
		if err := RemoveEmptyCreatedDir(root, d); err != nil {
			failed = append(failed, d.Rel)
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}
