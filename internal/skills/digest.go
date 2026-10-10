package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// PackageDigest computes a deterministic integrity digest over a skill package.
// Includes exactly SKILL.md and references/**, scripts/**, assets/**.
// Unsupported top-level entries and symlinks fail closed.
func PackageDigest(packageRoot string) (string, []string, error) {
	root := filepath.Clean(strings.TrimSpace(packageRoot))
	if root == "" {
		return "", nil, fmt.Errorf("skills: package root is required")
	}
	info, err := os.Lstat(root)
	if err != nil {
		return "", nil, fmt.Errorf("skills: package root: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("skills: package root is a symlink")
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("skills: package root is not a directory")
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		return "", nil, err
	}
	for _, e := range entries {
		name := e.Name()
		if !IsSupportedPackageTopLevel(name) {
			return "", nil, fmt.Errorf("skills: unsupported package entry %q", name)
		}
	}

	var files []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("skills: refusing symlink in package: %s", rel)
		}
		if d.IsDir() {
			return nil
		}
		if !IsPackageFileRel(rel) {
			return fmt.Errorf("skills: unsupported package file %q", rel)
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	if len(files) == 0 {
		return "", nil, fmt.Errorf("skills: package has no digestable files")
	}
	sort.Strings(files)

	h := sha256.New()
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", nil, fmt.Errorf("skills: read %s: %w", rel, err)
		}
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(data))
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), files, nil
}

// PackageDigestContained digests a Home-contained skill package (fail closed on
// symlink parents/leaves and containment failures).
func PackageDigestContained(homePath, packageRel string) (string, []string, error) {
	packageRel = filepath.ToSlash(strings.TrimSpace(packageRel))
	if _, err := fsafety.ContainedJoin(homePath, packageRel); err != nil {
		return "", nil, fmt.Errorf("skills: package containment: %w", err)
	}
	info, err := fsafety.LstatContained(homePath, packageRel)
	if err != nil {
		return "", nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", nil, fmt.Errorf("skills: package root is a symlink")
	}
	if !info.IsDir() {
		return "", nil, fmt.Errorf("skills: package root is not a directory")
	}

	entries, err := readDirContained(homePath, packageRel)
	if err != nil {
		return "", nil, err
	}
	for _, name := range entries {
		if !IsSupportedPackageTopLevel(name) {
			return "", nil, fmt.Errorf("skills: unsupported package entry %q", name)
		}
	}

	files, err := listPackageFilesContained(homePath, packageRel)
	if err != nil {
		return "", nil, err
	}
	if len(files) == 0 {
		return "", nil, fmt.Errorf("skills: package has no digestable files")
	}
	sort.Strings(files)

	h := sha256.New()
	for _, rel := range files {
		fullRel := filepath.ToSlash(filepath.Join(packageRel, rel))
		data, err := fsafety.ReadFileContained(homePath, fullRel)
		if err != nil {
			return "", nil, fmt.Errorf("skills: read %s: %w", rel, err)
		}
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(data))
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), files, nil
}

// DigestBytes is a pure helper for tests: digest pre-ordered path→content maps.
// Rejects unsupported package paths.
func DigestBytes(files map[string][]byte) (string, error) {
	filtered, err := FilterPackageFiles(files)
	if err != nil {
		return "", err
	}
	keys := make([]string, 0, len(filtered))
	for k := range filtered {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, rel := range keys {
		data := filtered[rel]
		fmt.Fprintf(h, "%s\x00%d\x00", rel, len(data))
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readDirContained(homePath, rel string) ([]string, error) {
	abs, err := fsafety.ContainedJoin(homePath, rel)
	if err != nil {
		return nil, err
	}
	ents, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range ents {
		// Re-validate each child through ContainedJoin (symlink refusal).
		childRel := filepath.ToSlash(filepath.Join(rel, e.Name()))
		if _, err := fsafety.ContainedJoin(homePath, childRel); err != nil {
			return nil, err
		}
		info, err := fsafety.LstatContained(homePath, childRel)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("skills: refusing symlink %s", childRel)
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

func listPackageFilesContained(homePath, packageRel string) ([]string, error) {
	var files []string
	var walk func(rel string) error
	walk = func(rel string) error {
		names, err := readDirContained(homePath, rel)
		if err != nil {
			return err
		}
		for _, name := range names {
			child := filepath.ToSlash(filepath.Join(rel, name))
			info, err := fsafety.LstatContained(homePath, child)
			if err != nil {
				return err
			}
			if info.IsDir() {
				if err := walk(child); err != nil {
					return err
				}
				continue
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("skills: refusing non-regular %s", child)
			}
			pkgRel, err := filepath.Rel(packageRel, child)
			if err != nil {
				return err
			}
			pkgRel = filepath.ToSlash(pkgRel)
			if !IsPackageFileRel(pkgRel) {
				return fmt.Errorf("skills: unsupported package file %q", pkgRel)
			}
			files = append(files, pkgRel)
		}
		return nil
	}
	if err := walk(packageRel); err != nil {
		return nil, err
	}
	return files, nil
}
