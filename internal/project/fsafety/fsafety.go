// Package fsafety provides symlink-safe path containment and atomic writes
// for Atlas project and Home filesystem mutations.
package fsafety

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultDirPerm os.FileMode = 0o755

// mkdirFn is the mkdir primitive for contained directory creation (test-overridable).
var mkdirFn = os.Mkdir

// ContainedJoin resolves root/rel for reading or writing while rejecting path
// traversal and any symlink in the chain from root through every path component.
// Missing leaf components are allowed; an existing symlink parent blocks even
// when the leaf does not yet exist. A symlink root also blocks.
func ContainedJoin(root, rel string) (string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	rel = strings.TrimSpace(rel)
	if root == "" || root == "." {
		return "", fmt.Errorf("fsafety: root is required")
	}
	if rel == "" {
		return "", fmt.Errorf("fsafety: relative path is required")
	}
	rel = filepath.ToSlash(rel)
	if filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("fsafety: absolute path refused: %s", rel)
	}
	clean := filepath.Clean(filepath.FromSlash(rel))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("fsafety: path traversal refused: %s", rel)
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("fsafety: root: %w", err)
	}
	if info, err := os.Lstat(absRoot); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("fsafety: root is a symlink")
	} else if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("fsafety: root: %w", err)
	}

	cur := absRoot
	parts := strings.Split(clean, string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("fsafety: path traversal refused: %s", rel)
		}
		next := filepath.Join(cur, part)
		info, err := os.Lstat(next)
		if os.IsNotExist(err) {
			cur = next
			continue
		}
		if err != nil {
			return "", fmt.Errorf("fsafety: lstat %s: %w", next, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			relComp := filepath.ToSlash(strings.TrimPrefix(next, absRoot+string(filepath.Separator)))
			return "", fmt.Errorf("fsafety: refusing symlink path component %s", relComp)
		}
		cur = next
	}

	sep := string(filepath.Separator)
	if cur != absRoot && !strings.HasPrefix(cur, absRoot+sep) {
		return "", fmt.Errorf("fsafety: path escapes root: %s", rel)
	}
	return cur, nil
}

// EnsureContainedDir creates parent directories for a contained relative file
// path using only real (non-symlink) components under root.
// dirPerm is applied to newly created directories (default 0o755 when zero).
func EnsureContainedDir(root, relFile string, dirPerm os.FileMode) (string, error) {
	full, _, err := EnsureContainedDirCreated(root, relFile, dirPerm)
	return full, err
}

// EnsureContainedDirCreated is EnsureContainedDir plus CreatedDir footprints for
// directories newly created by this call.
func EnsureContainedDirCreated(root, relFile string, dirPerm os.FileMode) (string, []CreatedDir, error) {
	if dirPerm == 0 {
		dirPerm = defaultDirPerm
	}
	full, err := ContainedJoin(root, relFile)
	if err != nil {
		return "", nil, err
	}
	dir := filepath.Dir(full)
	relDir, err := filepath.Rel(filepath.Clean(root), dir)
	if err != nil {
		return "", nil, err
	}
	if relDir == "." {
		return full, nil, nil
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}
	rootCreated, err := ensureRootExistsCreated(absRoot, dirPerm)
	if err != nil {
		return "", nil, err
	}
	var created []CreatedDir
	if rootCreated != nil {
		// Root creation is outside rel tree; callers track Home root separately when needed.
		_ = rootCreated
	}
	cur := absRoot
	relParts := strings.Split(relDir, string(filepath.Separator))
	built := ""
	for _, part := range relParts {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		if built == "" {
			built = part
		} else {
			built = filepath.ToSlash(filepath.Join(built, part))
		}
		info, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			if err := mkdirFn(cur, dirPerm); err != nil {
				return "", created, fmt.Errorf("fsafety: mkdir %s: %w", cur, err)
			}
			info, err = os.Lstat(cur)
			if err != nil {
				return "", created, err
			}
			created = append(created, CreatedDir{Rel: filepath.ToSlash(built), Info: info})
			continue
		}
		if err != nil {
			return "", created, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", created, fmt.Errorf("fsafety: refusing symlink directory %s", cur)
		}
		if !info.IsDir() {
			return "", created, fmt.Errorf("fsafety: parent is not a directory: %s", cur)
		}
	}
	return full, created, nil
}

func ensureRootExists(absRoot string, dirPerm os.FileMode) error {
	_, err := ensureRootExistsCreated(absRoot, dirPerm)
	return err
}

// ensureRootExistsCreated ensures absRoot exists as a real directory.
// Returns non-nil FileInfo when this call created the root.
func ensureRootExistsCreated(absRoot string, dirPerm os.FileMode) (os.FileInfo, error) {
	info, err := os.Lstat(absRoot)
	if os.IsNotExist(err) {
		if err := os.MkdirAll(absRoot, dirPerm); err != nil {
			return nil, fmt.Errorf("fsafety: create root: %w", err)
		}
		info, err = os.Lstat(absRoot)
		if err != nil {
			return nil, fmt.Errorf("fsafety: root: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("fsafety: root is a symlink")
		}
		return info, nil
	} else if err != nil {
		return nil, fmt.Errorf("fsafety: root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("fsafety: root is a symlink")
	}
	return nil, nil
}

// AtomicWriteContained writes data to root/rel via temp+rename after
// symlink-safe containment checks. Temporary files live beside the destination.
// Parent directories are created with 0o755.
func AtomicWriteContained(root, rel string, data []byte, perm os.FileMode) error {
	_, _, err := AtomicWriteContainedTracked(root, rel, data, perm, defaultDirPerm, ".atlas-write-*.tmp")
	return err
}

// AtomicWriteContainedPrefix is AtomicWriteContained with a custom temp pattern.
func AtomicWriteContainedPrefix(root, rel string, data []byte, perm os.FileMode, tmpPattern string) error {
	_, _, err := AtomicWriteContainedTracked(root, rel, data, perm, defaultDirPerm, tmpPattern)
	return err
}

// AtomicWriteContainedDir writes with an explicit parent directory permission.
func AtomicWriteContainedDir(root, rel string, data []byte, filePerm, dirPerm os.FileMode, tmpPattern string) error {
	_, _, err := AtomicWriteContainedTracked(root, rel, data, filePerm, dirPerm, tmpPattern)
	return err
}

// AtomicWriteContainedTracked writes like AtomicWriteContainedDir and returns the
// written-file footprint plus parent directories created by this call.
func AtomicWriteContainedTracked(root, rel string, data []byte, filePerm, dirPerm os.FileMode, tmpPattern string) (WrittenFile, []CreatedDir, error) {
	rel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(rel)))
	full, createdDirs, err := EnsureContainedDirCreated(root, rel, dirPerm)
	if err != nil {
		// Partial mkdir progress is returned even on failure.
		return WrittenFile{}, createdDirs, err
	}
	created := false
	if _, err := os.Lstat(full); os.IsNotExist(err) {
		created = true
	} else if err != nil {
		return WrittenFile{}, createdDirs, err
	}
	if strings.TrimSpace(tmpPattern) == "" {
		tmpPattern = ".atlas-write-*.tmp"
	}
	if err := AtomicReplace(full, data, filePerm, tmpPattern); err != nil {
		// Parent dirs created before write failure remain in createdDirs.
		return WrittenFile{}, createdDirs, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return WrittenFile{}, createdDirs, err
	}
	mode := filePerm.Perm()
	if mode == 0 {
		mode = 0o644
	}
	return WrittenFile{
		Rel:      rel,
		Created:  created,
		Expected: append([]byte(nil), data...),
		Mode:     mode,
		Info:     info,
	}, createdDirs, nil
}

// AtomicReplace writes data to an absolute path via temp+rename, refusing
// to write through a leaf symlink.
func AtomicReplace(path string, data []byte, perm os.FileMode, tmpPattern string) error {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return fmt.Errorf("fsafety write: path must be absolute")
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("fsafety write: refusing to write through symlink %s", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("fsafety write: lstat: %w", err)
	}
	dir := filepath.Dir(path)
	if strings.TrimSpace(tmpPattern) == "" {
		tmpPattern = ".atlas-write-*.tmp"
	}
	tmp, err := os.CreateTemp(dir, tmpPattern)
	if err != nil {
		return fmt.Errorf("fsafety write: temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsafety write: temp write: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("fsafety write: chmod: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("fsafety write: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("fsafety write: rename: %w", err)
	}
	cleanup = false
	return nil
}

// SafeMkdirAll creates root/rel as a directory tree with symlink refusal on
// every existing component under root. Newly created directories use perm
// (default 0o755 when zero). Umask still applies as usual for os.Mkdir.
func SafeMkdirAll(root, rel string, perm os.FileMode) error {
	_, err := SafeMkdirAllCreated(root, rel, perm)
	return err
}

// SafeMkdirAllCreated is SafeMkdirAll plus CreatedDir footprints for dirs this call created.
func SafeMkdirAllCreated(root, rel string, perm os.FileMode) ([]CreatedDir, error) {
	if perm == 0 {
		perm = defaultDirPerm
	}
	rel = strings.TrimSpace(rel)
	absRoot, err := filepath.Abs(filepath.Clean(strings.TrimSpace(root)))
	if err != nil {
		return nil, err
	}
	var created []CreatedDir
	rootInfo, err := ensureRootExistsCreated(absRoot, perm)
	if err != nil {
		return nil, err
	}
	if rootInfo != nil && (rel == "" || rel == ".") {
		// Caller may track "." specially for brand-new roots.
		created = append(created, CreatedDir{Rel: ".", Info: rootInfo})
	}
	if rel == "" || rel == "." {
		return created, nil
	}
	marker := filepath.ToSlash(filepath.Join(filepath.FromSlash(rel), ".atlas-fsafety-keep"))
	_, dirs, err := EnsureContainedDirCreated(root, marker, perm)
	created = append(created, dirs...)
	if err != nil {
		return created, err
	}
	return created, nil
}

// LstatContained resolves root/rel with symlink-safe containment and Lstats the leaf.
// Symlink parents, symlink leaf, and path escapes are rejected.
func LstatContained(root, rel string) (os.FileInfo, error) {
	full, err := ContainedJoin(root, rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("fsafety: refusing symlink leaf %s", filepath.ToSlash(rel))
	}
	return info, nil
}

// ReadFileContained reads a regular non-symlink file under root/rel.
// Every path component beneath root is validated; symlink targets are never followed.
func ReadFileContained(root, rel string) ([]byte, error) {
	full, err := ContainedJoin(root, rel)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("fsafety: refusing symlink leaf %s", filepath.ToSlash(rel))
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("fsafety: not a regular file: %s", filepath.ToSlash(rel))
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// CaptureFileSnapshot reads an existing file under root/rel for later restore.
// Exists is false when the path is absent.
func CaptureFileSnapshot(root, rel string) (FileSnapshot, error) {
	full, err := ContainedJoin(root, rel)
	if err != nil {
		return FileSnapshot{Rel: rel}, err
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return FileSnapshot{Rel: rel, Exists: false}, nil
	}
	if err != nil {
		return FileSnapshot{Rel: rel}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return FileSnapshot{Rel: rel}, fmt.Errorf("fsafety: refusing symlink leaf %s", filepath.ToSlash(rel))
	}
	if info.IsDir() {
		return FileSnapshot{Rel: rel, Exists: true, WasFile: false, Mode: info.Mode().Perm(), Info: info}, nil
	}
	if !info.Mode().IsRegular() {
		return FileSnapshot{Rel: rel}, fmt.Errorf("fsafety: not a regular file: %s", filepath.ToSlash(rel))
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return FileSnapshot{Rel: rel}, err
	}
	return FileSnapshot{
		Rel:     rel,
		Exists:  true,
		Data:    append([]byte(nil), data...),
		Mode:    info.Mode().Perm(),
		WasFile: true,
		Info:    info,
	}, nil
}

// FileSnapshot is a restoreable view of one relative path under a root.
type FileSnapshot struct {
	Rel     string
	Exists  bool
	Data    []byte
	Mode    os.FileMode
	WasFile bool
	Info    os.FileInfo // baseline identity when Exists (for delete proofs)
}

// RestoreFileSnapshot restores a previously captured file snapshot under root.
//
// Exact rollback claim covers path existence, type, bytes, permission mode, and
// transaction object identity — not ACLs, xattrs, UID/GID, or timestamps.
//
// Destructive remove/overwrite requires wrote footprint proving the current object
// is the Atlas post-write object (os.SameFile + content + mode).
// Recreating a missing baseline file requires deleted footprint proving Atlas
// removed it during the transaction.
// wrote/deleted may be nil only for non-destructive verification (already at baseline).
func RestoreFileSnapshot(root string, snap FileSnapshot, wrote *WrittenFile, deleted *DeletedFile) error {
	full, err := ContainedJoin(root, snap.Rel)
	if err != nil {
		return err
	}
	if !snap.Exists {
		info, err := os.Lstat(full)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if wrote == nil {
			return conflict(snap.Rel, "cannot remove without transaction footprint", "absent or atlas-written file", "present without footprint")
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return conflict(snap.Rel, "unexpected type on remove", "regular file", info.Mode().String())
		}
		if err := VerifyWrittenFile(root, *wrote); err != nil {
			return err
		}
		if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if !snap.WasFile {
		return fmt.Errorf("fsafety restore: directory restore not supported for %s", snap.Rel)
	}
	mode := snap.Mode
	if mode == 0 {
		mode = 0o644
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		if deleted == nil {
			return conflict(snap.Rel, "cannot recreate without transaction deletion footprint", "atlas-deleted baseline file", "missing without deletion proof")
		}
		if filepath.ToSlash(filepath.Clean(deleted.Rel)) != filepath.ToSlash(filepath.Clean(snap.Rel)) {
			return conflict(snap.Rel, "deletion footprint path mismatch", snap.Rel, deleted.Rel)
		}
		_, _, werr := AtomicWriteContainedTracked(root, snap.Rel, snap.Data, mode, defaultDirPerm, ".atlas-restore-*.tmp")
		return werr
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return conflict(snap.Rel, "unexpected type on restore", "regular file", info.Mode().String())
	}
	cur, err := os.ReadFile(full)
	if err != nil {
		return err
	}
	if bytes.Equal(cur, snap.Data) && info.Mode().Perm() == mode {
		return nil
	}
	if wrote == nil {
		return conflict(snap.Rel, "cannot overwrite without transaction footprint", "baseline or atlas-written object", "modified without footprint")
	}
	if err := VerifyWrittenFile(root, *wrote); err != nil {
		return err
	}
	_, _, werr := AtomicWriteContainedTracked(root, snap.Rel, snap.Data, mode, defaultDirPerm, ".atlas-restore-*.tmp")
	return werr
}
