package openspec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// Known adapter roots that OpenSpec may project tools into.
var adapterToolRoots = map[string]string{
	"cursor":   ".cursor",
	"opencode": ".opencode",
}

// HasStaleToolProjections reports whether provider-owned tool files exist for
// adapters that are not in the selected set.
func HasStaleToolProjections(root string, selected []string) bool {
	files, err := ListStaleToolProjectionFiles(root, selected)
	return err == nil && len(files) > 0
}

// ListStaleToolProjectionFiles returns OpenSpec-owned regular files under
// adapter trees that are not currently selected. Atlas-owned and user-owned
// paths are never listed (ProviderOwnedRel gate).
func ListStaleToolProjectionFiles(root string, selected []string) ([]string, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return nil, nil
	}
	selectedSet := map[string]bool{}
	for _, a := range selected {
		id := strings.ToLower(strings.TrimSpace(a))
		if id != "" {
			selectedSet[id] = true
		}
	}
	seen := map[string]struct{}{}
	var out []string
	for adapter, prefix := range adapterToolRoots {
		if selectedSet[adapter] {
			continue
		}
		if err := walkProviderPrefix(root, prefix, func(rel string) {
			if !providerOwnedRel(rel) {
				return
			}
			full := filepath.Join(root, filepath.FromSlash(rel))
			info, err := os.Lstat(full)
			if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return
			}
			if _, ok := seen[rel]; ok {
				return
			}
			seen[rel] = struct{}{}
			out = append(out, rel)
		}); err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

// RemoveStaleToolProjections deletes OpenSpec-owned tool projections for
// deselected adapters and records deletions on the footprint for rollback.
// Only ProviderOwnedRel paths are touched; user/Atlas content is preserved.
// Empty provider skill/command directories are pruned best-effort afterward.
func RemoveStaleToolProjections(fp *fsafety.TransactionFootprint, root string, selected []string) error {
	files, err := ListStaleToolProjectionFiles(root, selected)
	if err != nil {
		return fmt.Errorf("openspec stale projections: %w", err)
	}
	// Delete deepest paths first so parent dirs can be pruned.
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	for _, rel := range files {
		if err := removeTrackedProviderFile(fp, root, rel); err != nil {
			return err
		}
	}
	// Prune empty provider-owned directories under deselected adapter roots.
	selectedSet := map[string]bool{}
	for _, a := range selected {
		selectedSet[strings.ToLower(strings.TrimSpace(a))] = true
	}
	for adapter, prefix := range adapterToolRoots {
		if selectedSet[adapter] {
			continue
		}
		if err := pruneEmptyProviderDirs(root, prefix); err != nil {
			return err
		}
	}
	return nil
}

func removeTrackedProviderFile(fp *fsafety.TransactionFootprint, root, rel string) error {
	rel = filepath.ToSlash(filepath.Clean(rel))
	if !providerOwnedRel(rel) {
		return fmt.Errorf("openspec: refused non-provider path %s", rel)
	}
	full, err := fsafety.ContainedJoin(root, rel)
	if err != nil {
		return err
	}
	info, err := os.Lstat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("openspec remove %s: %w", rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("openspec remove %s: refusing symlink", rel)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("openspec remove %s: not a regular file", rel)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return fmt.Errorf("openspec remove %s: read: %w", rel, err)
	}
	if err := os.Remove(full); err != nil {
		return fmt.Errorf("openspec remove %s: %w", rel, err)
	}
	if fp != nil {
		fp.AddDeleted(fsafety.DeletedFile{
			Rel:      rel,
			Expected: data,
			Mode:     info.Mode(),
			Info:     info,
		})
	}
	return nil
}

// pruneEmptyProviderDirs removes empty directories under prefix when they are
// provider-owned (opsx/openspec skill trees and command dirs). Never removes
// the adapter root itself here — caller may prune that after Atlas cleanup.
func pruneEmptyProviderDirs(root, prefix string) error {
	root = filepath.Clean(strings.TrimSpace(root))
	prefix = filepath.ToSlash(filepath.Clean(prefix))
	abs, err := fsafety.ContainedJoin(root, prefix)
	if err != nil {
		return nil
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil
	}
	var dirs []string
	_ = filepath.WalkDir(abs, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || !d.IsDir() {
			return walkErr
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == prefix {
			return nil // adapter root handled elsewhere
		}
		if !providerOwnedRel(rel) {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})
	sort.Sort(sort.Reverse(sort.StringSlice(dirs)))
	for _, d := range dirs {
		_ = os.Remove(d) // empty only
	}
	return nil
}
