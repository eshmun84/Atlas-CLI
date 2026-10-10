package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// InspectInput drives read-only skill health evaluation.
type InspectInput struct {
	Root             string
	HomePath         string
	ProjectID        string
	Enabled          []Pin
	Adapters         []string
	Projectors       map[adapters.ID]Projector
	Ownership        OwnershipDocument
	ExpectedRegistry string // expected .atlas/skill-registry.md content (optional)
	RegistryRel      string
}

// Inspect evaluates skill catalog/pin/projection health without mutation.
func Inspect(in InspectInput) SkillHealth {
	h := SkillHealth{
		Enabled:   append([]Pin(nil), in.Enabled...),
		CheckedAt: time.Now().UTC(),
	}
	catalog, err := DiscoverCatalog(in.HomePath)
	if err != nil {
		h.CatalogReadable = false
		h.CatalogError = err.Error()
		return h
	}
	h.CatalogReadable = true
	h.Available = catalog

	if in.RegistryRel != "" {
		data, err := fsafety.ReadFileContained(in.Root, in.RegistryRel)
		switch {
		case err == nil:
			h.RegistryPresent = true
			if in.ExpectedRegistry != "" {
				h.RegistryMatches = string(data) == in.ExpectedRegistry
			}
		case os.IsNotExist(err):
			h.RegistryPresent = false
		default:
			h.RegistryPresent = false
			h.CatalogError = err.Error()
		}
	}

	for _, adapter := range in.Adapters {
		id := adapters.ID(adapter)
		proj, ok := in.Projectors[id]
		if !ok || proj == nil || !proj.SupportsSkills() {
			for _, pin := range in.Enabled {
				h.Projections = append(h.Projections, ProjectionStatus{
					Adapter: adapter,
					SkillID: pin.ID,
					Version: pin.Version,
					State:   "unsupported",
					Message: "adapter does not support Skills",
				})
			}
			continue
		}
		for _, pin := range in.Enabled {
			st := inspectOne(in.Root, in.HomePath, catalog, in.Ownership, adapter, proj, pin)
			h.Projections = append(h.Projections, st)
		}
	}
	return h
}

func inspectOne(root, homePath string, catalog []Metadata, own OwnershipDocument, adapter string, proj Projector, pin Pin) ProjectionStatus {
	st := ProjectionStatus{
		Adapter: adapter,
		SkillID: pin.ID,
		Version: pin.Version,
		RootRel: proj.PackageRootRel(pin.ID),
	}
	meta, err := FindExact(catalog, pin.ID, pin.Version)
	if err != nil {
		st.State = "missing"
		st.Message = err.Error()
		return st
	}
	st.Digest = meta.Digest
	owned, hasOwn := own.Lookup(adapter, pin.ID)
	st.Owned = hasOwn && owned.RootRel == st.RootRel

	abs, err := fsafety.ContainedJoin(root, st.RootRel)
	if err != nil {
		st.State = "invalid"
		st.Message = err.Error()
		return st
	}
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		st.State = "missing"
		st.Message = "projection missing"
		return st
	}
	if err != nil {
		st.State = "invalid"
		st.Message = err.Error()
		return st
	}
	if info.Mode()&os.ModeSymlink != 0 {
		st.State = "invalid"
		st.Message = "projection is a symlink"
		return st
	}
	if !info.IsDir() {
		st.State = "invalid"
		st.Message = "projection is not a directory"
		return st
	}
	skillMD := filepath.Join(abs, FileSkillMD)
	si, err := os.Lstat(skillMD)
	if err != nil || si.Mode()&os.ModeSymlink != 0 || !si.Mode().IsRegular() {
		st.State = "invalid"
		st.Message = "SKILL.md missing or unsafe"
		return st
	}
	digest, _, err := PackageDigest(abs)
	if err != nil {
		st.State = "invalid"
		st.Message = err.Error()
		return st
	}
	if !hasOwn {
		st.State = "conflict"
		st.Message = "projection exists but is not Atlas-owned"
		return st
	}
	if owned.Digest != meta.Digest {
		st.State = "stale"
		st.Message = fmt.Sprintf("ownership digest %s != catalog %s", owned.Digest, meta.Digest)
		return st
	}
	if digest != meta.Digest {
		st.State = "drifted"
		st.Message = "projection digest differs from catalog"
		return st
	}
	_ = homePath
	st.State = "ready"
	st.Message = "ok"
	return st
}

// Ready reports whether all enabled projections are ready.
func (h SkillHealth) Ready() bool {
	if !h.CatalogReadable || len(h.Enabled) == 0 {
		return h.CatalogReadable
	}
	if len(h.Projections) == 0 {
		return true
	}
	for _, p := range h.Projections {
		if p.State != "ready" && p.State != "unsupported" {
			return false
		}
		if p.State == "unsupported" {
			return false
		}
	}
	return true
}

// SummaryCounts returns counts by projection state.
func (h SkillHealth) SummaryCounts() map[string]int {
	out := map[string]int{}
	for _, p := range h.Projections {
		out[p.State]++
	}
	return out
}

// HasConflict reports any conflict projection.
func (h SkillHealth) HasConflict() bool {
	for _, p := range h.Projections {
		if p.State == "conflict" {
			return true
		}
	}
	return false
}

// NormalizePins returns DefaultPins when enabled is empty (init default).
func NormalizePins(enabled []Pin) []Pin {
	if len(enabled) == 0 {
		return DefaultPins()
	}
	out := make([]Pin, 0, len(enabled))
	for _, p := range enabled {
		out = append(out, Pin{ID: strings.TrimSpace(p.ID), Version: strings.TrimSpace(p.Version)})
	}
	return out
}
