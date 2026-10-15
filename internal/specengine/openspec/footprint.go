package openspec

import (
	"os"
	"path/filepath"
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
