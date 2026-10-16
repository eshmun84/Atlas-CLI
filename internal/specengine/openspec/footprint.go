package openspec

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// ProviderPathPrefixes are workspace-relative roots OpenSpec may create/update.
// Include adapter roots so empty .cursor/.opencode can be removed on Init rollback.
var ProviderPathPrefixes = []string{
	"openspec",
	".cursor",
	".opencode",
}

// InventoryProviderPaths returns a set of existing relative paths under OpenSpec-owned prefixes.
func InventoryProviderPaths(root string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return out, nil
	}
	for _, prefix := range ProviderPathPrefixes {
		if err := walkProviderPrefix(root, prefix, func(rel string) {
			out[rel] = struct{}{}
		}); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// CaptureProviderFileSnapshots snapshots existing OpenSpec-owned regular files
// under provider prefixes so Configure/Init rollback can restore content that
// the provider later modifies or deletes. Snapshotting does not imply Atlas ownership.
// Symlinked provider roots fail closed.
func CaptureProviderFileSnapshots(root string) ([]fsafety.FileSnapshot, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return nil, nil
	}
	seen := map[string]struct{}{}
	for _, prefix := range ProviderPathPrefixes {
		info, err := fsafety.LstatContained(root, prefix)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("openspec provider snapshot: %s: %w", prefix, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("openspec provider snapshot: refusing symlink %s", prefix)
		}
		if err := walkProviderPrefix(root, prefix, func(rel string) {
			if !providerOwnedRel(rel) {
				return
			}
			full := filepath.Join(root, filepath.FromSlash(rel))
			st, lerr := os.Lstat(full)
			if lerr != nil || st.IsDir() || st.Mode()&os.ModeSymlink != 0 || !st.Mode().IsRegular() {
				return
			}
			seen[rel] = struct{}{}
		}); err != nil {
			return nil, err
		}
	}
	rels := make([]string, 0, len(seen))
	for rel := range seen {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	out := make([]fsafety.FileSnapshot, 0, len(rels))
	for _, rel := range rels {
		fs, err := fsafety.CaptureFileSnapshot(root, rel)
		if err != nil {
			return nil, fmt.Errorf("openspec provider snapshot %s: %w", rel, err)
		}
		if fs.Exists && fs.WasFile {
			out = append(out, fs)
		}
	}
	return out, nil
}

// RecordProviderMutationFootprint absorbs new provider paths and records
// modifications/deletions against baseline snapshots so rollback can restore
// exact pre-mutation provider-owned file state.
func RecordProviderMutationFootprint(fp *fsafety.TransactionFootprint, root string, before map[string]struct{}, baseline []fsafety.FileSnapshot) error {
	if fp == nil {
		return nil
	}
	if err := AbsorbNewProviderPaths(fp, root, before); err != nil {
		return err
	}
	root = filepath.Clean(strings.TrimSpace(root))
	for _, snap := range baseline {
		if !snap.Exists || !snap.WasFile || !providerOwnedRel(snap.Rel) {
			continue
		}
		info, err := fsafety.LstatContained(root, snap.Rel)
		if err != nil {
			if os.IsNotExist(err) {
				fp.AddDeleted(fsafety.DeletedFile{
					Rel:      snap.Rel,
					Expected: append([]byte(nil), snap.Data...),
					Mode:     snap.Mode,
					Info:     snap.Info,
				})
				continue
			}
			return fmt.Errorf("openspec provider footprint: %s: %w", snap.Rel, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("openspec provider footprint: %s: unexpected type after provider mutation", snap.Rel)
		}
		cur, err := fsafety.ReadFileContained(root, snap.Rel)
		if err != nil {
			return fmt.Errorf("openspec provider footprint: read %s: %w", snap.Rel, err)
		}
		mode := info.Mode().Perm()
		if bytes.Equal(cur, snap.Data) && mode == snap.Mode {
			continue
		}
		fp.AddFile(fsafety.WrittenFile{
			Rel:      snap.Rel,
			Created:  false,
			Expected: append([]byte(nil), cur...),
			Mode:     mode,
			Info:     info,
		})
	}
	return nil
}

// AbsorbNewProviderPaths records files/dirs created after before into fp for rollback.
// Only OpenSpec-owned names under command/skill trees are absorbed (opsx-*, openspec-*).
func AbsorbNewProviderPaths(fp *fsafety.TransactionFootprint, root string, before map[string]struct{}) error {
	if fp == nil {
		return nil
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" {
		return nil
	}
	for _, prefix := range ProviderPathPrefixes {
		if err := walkProviderPrefix(root, prefix, func(rel string) {
			if _, existed := before[rel]; existed {
				return
			}
			if !providerOwnedRel(rel) {
				return
			}
			full := filepath.Join(root, filepath.FromSlash(rel))
			info, err := os.Lstat(full)
			if err != nil {
				return
			}
			if info.IsDir() {
				fp.AddDir(fsafety.CreatedDir{Rel: rel, Info: info})
				return
			}
			if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
				return
			}
			data, err := os.ReadFile(full)
			if err != nil {
				return
			}
			fp.AddFile(fsafety.WrittenFile{
				Rel:      rel,
				Created:  true,
				Expected: data,
				Mode:     info.Mode().Perm(),
				Info:     info,
			})
		}); err != nil {
			return err
		}
	}
	return nil
}

func walkProviderPrefix(root, prefix string, fn func(rel string)) error {
	full := filepath.Join(root, filepath.FromSlash(prefix))
	info, err := os.Lstat(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}
	return filepath.Walk(full, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		fn(rel)
		return nil
	})
}

// ProviderOwnedRel reports whether rel is an OpenSpec-owned provider path
// (openspec/ tree or opsx-/openspec-* adapter projections). Atlas-owned
// surfaces under .cursor/.opencode are excluded.
func ProviderOwnedRel(rel string) bool {
	return providerOwnedRel(rel)
}

func providerOwnedRel(rel string) bool {
	rel = filepath.ToSlash(rel)
	switch {
	case rel == "openspec" || strings.HasPrefix(rel, "openspec/"):
		return true
	case rel == ".cursor" || rel == ".opencode":
		return true
	case strings.HasPrefix(rel, ".cursor/commands") || strings.HasPrefix(rel, ".opencode/commands"):
		// Only OpenSpec opsx commands (and their parent command dirs).
		base := filepath.Base(rel)
		if rel == ".cursor/commands" || rel == ".opencode/commands" {
			return true
		}
		return strings.HasPrefix(base, "opsx-")
	case strings.HasPrefix(rel, ".cursor/skills/") || strings.HasPrefix(rel, ".opencode/skills/"):
		// openspec-* skill packages only (not Atlas/user skills).
		rest := strings.TrimPrefix(rel, ".cursor/skills/")
		rest = strings.TrimPrefix(rest, ".opencode/skills/")
		skillID := strings.SplitN(rest, "/", 2)[0]
		return strings.HasPrefix(skillID, "openspec-")
	case rel == ".cursor/skills" || rel == ".opencode/skills":
		return true
	default:
		return false
	}
}
