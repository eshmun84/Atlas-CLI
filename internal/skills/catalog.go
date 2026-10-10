package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// CanonicalPackageRel returns Home-relative package root under assets/skills/.
func CanonicalPackageRel(id, version string) string {
	return filepath.ToSlash(filepath.Join("assets", "skills", id, version))
}

// CanonicalSkillMDRel returns Home-relative SKILL.md path.
func CanonicalSkillMDRel(id, version string) string {
	return CanonicalPackageRel(id, version) + "/" + FileSkillMD
}

// LoadPackageContained loads and validates a skill package under Atlas Home
// using fsafety containment (symlink parents/leaves fail closed).
func LoadPackageContained(homePath, packageRel, id, version, source string) (Package, error) {
	if err := ValidateSkillID(id); err != nil {
		return Package{}, err
	}
	if err := ValidateExactVersion(version); err != nil {
		return Package{}, err
	}
	packageRel = filepath.ToSlash(strings.TrimSpace(packageRel))
	skillRel := filepath.ToSlash(filepath.Join(packageRel, FileSkillMD))
	info, err := fsafety.LstatContained(homePath, skillRel)
	if err != nil {
		if os.IsNotExist(err) {
			return Package{}, fmt.Errorf("skills: missing SKILL.md in %s@%s", id, version)
		}
		return Package{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Package{}, fmt.Errorf("skills: SKILL.md is a symlink")
	}
	if !info.Mode().IsRegular() {
		return Package{}, fmt.Errorf("skills: SKILL.md is not a regular file")
	}
	body, err := fsafety.ReadFileContained(homePath, skillRel)
	if err != nil {
		return Package{}, err
	}
	fm, err := ParseSkillMD(string(body), id)
	if err != nil {
		return Package{}, err
	}
	digest, files, err := PackageDigestContained(homePath, packageRel)
	if err != nil {
		return Package{}, err
	}
	abs, err := fsafety.ContainedJoin(homePath, packageRel)
	if err != nil {
		return Package{}, err
	}
	meta := Metadata{
		ID:           id,
		Name:         fm.Name,
		Description:  fm.Description,
		Version:      version,
		Triggers:     fm.Triggers,
		Digest:       digest,
		Source:       source,
		CanonicalRel: CanonicalPackageRel(id, version),
		SkillMDRel:   CanonicalSkillMDRel(id, version),
	}
	return Package{Meta: meta, RootAbs: abs, SkillMD: string(body), Files: files}, nil
}

// DiscoverCatalog builds a local catalog from Atlas Home assets/skills.
// All reads inherit fsafety containment. Symlink parents/leaves, permission
// errors, and unexpected stat failures fail closed (no embed fallback).
// Embed fallback applies only when assets/skills is genuinely absent (IsNotExist).
func DiscoverCatalog(homePath string) ([]Metadata, error) {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		return nil, fmt.Errorf("skills: home path is required")
	}
	// Validate containment of assets and assets/skills before any read.
	if _, err := fsafety.ContainedJoin(homePath, "assets"); err != nil {
		return nil, fmt.Errorf("skills: catalog assets: %w", err)
	}
	if _, err := fsafety.ContainedJoin(homePath, "assets/skills"); err != nil {
		return nil, fmt.Errorf("skills: catalog: %w", err)
	}

	info, err := fsafety.LstatContained(homePath, "assets/skills")
	if os.IsNotExist(err) {
		return catalogFromEmbed()
	}
	if err != nil {
		return nil, fmt.Errorf("skills: catalog: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("skills: catalog root is a symlink")
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("skills: catalog root is not a directory")
	}

	type key struct{ id, ver string }
	seen := map[key]string{}
	var out []Metadata

	idNames, err := readDirContained(homePath, "assets/skills")
	if err != nil {
		return nil, fmt.Errorf("skills: catalog: %w", err)
	}
	for _, id := range idNames {
		idRel := filepath.ToSlash(filepath.Join("assets/skills", id))
		st, err := fsafety.LstatContained(homePath, idRel)
		if err != nil {
			return nil, fmt.Errorf("skills: catalog: %w", err)
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("skills: refusing symlink skill id path %s", id)
		}
		if !st.IsDir() {
			continue
		}
		verNames, err := readDirContained(homePath, idRel)
		if err != nil {
			return nil, fmt.Errorf("skills: catalog: %w", err)
		}
		for _, ver := range verNames {
			if err := ValidateExactVersion(ver); err != nil {
				continue
			}
			pkgRel := filepath.ToSlash(filepath.Join(idRel, ver))
			st, err := fsafety.LstatContained(homePath, pkgRel)
			if err != nil {
				return nil, fmt.Errorf("skills: catalog: %w", err)
			}
			if st.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("skills: refusing symlink package %s@%s", id, ver)
			}
			if !st.IsDir() {
				continue
			}
			k := key{id, ver}
			if prev, ok := seen[k]; ok {
				return nil, fmt.Errorf("skills: duplicate package %s@%s (also at %s)", id, ver, prev)
			}
			pkg, err := LoadPackageContained(homePath, pkgRel, id, ver, SourceHome)
			if err != nil {
				return nil, fmt.Errorf("skills: catalog %s@%s: %w", id, ver, err)
			}
			seen[k] = pkgRel
			out = append(out, pkg.Meta)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Version < out[j].Version
	})
	if len(out) == 0 {
		// Directory present but empty → still treat as absent bundled catalog.
		return catalogFromEmbed()
	}
	return out, nil
}

func catalogFromEmbed() ([]Metadata, error) {
	var out []Metadata
	for _, b := range BundledPackages() {
		files, err := loadEmbedPackageFiles(b.ID, b.Version)
		if err != nil {
			return nil, err
		}
		fm, err := ParseSkillMD(string(files[FileSkillMD]), b.ID)
		if err != nil {
			return nil, err
		}
		digest, err := DigestBytes(files)
		if err != nil {
			return nil, err
		}
		out = append(out, Metadata{
			ID:           b.ID,
			Name:         fm.Name,
			Description:  fm.Description,
			Version:      b.Version,
			Triggers:     fm.Triggers,
			Digest:       digest,
			Source:       SourceBundled,
			CanonicalRel: CanonicalPackageRel(b.ID, b.Version),
			SkillMDRel:   CanonicalSkillMDRel(b.ID, b.Version),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Version < out[j].Version
	})
	return out, nil
}

func fsWalkEmbed(prefix string, into map[string][]byte) error {
	return fsWalk(assets.Content, strings.TrimSuffix(prefix, "/"), into)
}

func fsWalk(fsys fs.FS, root string, into map[string][]byte) error {
	return fs.WalkDir(fsys, root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !IsPackageFileRel(rel) {
			return fmt.Errorf("skills: unsupported bundled package file %q", rel)
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		into[rel] = data
		return nil
	})
}

// FindExact locates an exact id@version in the catalog. No newest fallback.
func FindExact(catalog []Metadata, id, version string) (Metadata, error) {
	if err := ValidatePin(Pin{ID: id, Version: version}); err != nil {
		return Metadata{}, err
	}
	for _, m := range catalog {
		if m.ID == id && m.Version == version {
			return m, nil
		}
	}
	return Metadata{}, fmt.Errorf("skills: exact version %s@%s not found", id, version)
}

// ResolvePins validates pins against the catalog with exact versions only.
func ResolvePins(catalog []Metadata, pins []Pin) ([]Metadata, error) {
	out := make([]Metadata, 0, len(pins))
	seen := map[string]struct{}{}
	for _, p := range pins {
		if err := ValidatePin(p); err != nil {
			return nil, err
		}
		if _, dup := seen[p.ID]; dup {
			return nil, fmt.Errorf("skills: duplicate pin for %s", p.ID)
		}
		seen[p.ID] = struct{}{}
		m, err := FindExact(catalog, p.ID, p.Version)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}
