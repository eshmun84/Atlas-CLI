package config

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	adaptercursor "github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	adapteropencode "github.com/eshmun84/Atlas-CLI/internal/adapters/opencode"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

func init() {
	skills.RegisterDefaultProjector(adaptercursor.NewSkillsProjector())
	skills.RegisterDefaultProjector(adapteropencode.NewSkillsProjector())
}

// SkillProjectors returns registered skill projectors.
func SkillProjectors() map[adapters.ID]skills.Projector {
	return skills.DefaultProjectors()
}

// RenderSkillRegistry builds the project skill registry markdown.
func RenderSkillRegistry(doc ProjectDocument, homePath string) (string, error) {
	catalog, err := skills.DiscoverCatalog(homePath)
	if err != nil {
		return "", err
	}
	enabled, err := skills.ResolvePins(catalog, doc.SkillPins())
	if err != nil {
		return "", err
	}
	adaptersSelected := normalizeSelectedAdapters(doc.Adapters.Selected)
	return skills.RenderSkillRegistry(doc.Project.Name, enabled, adaptersSelected), nil
}

// ReconcileSkillProjections materializes/removes Atlas-owned skill projections.
// Must run under mutatelock with Home+Workspace already held by the caller.
// reclaimUnowned is true only after an explicit Init Home reset (ownership wiped).
func ReconcileSkillProjections(root string, doc ProjectDocument, reclaimUnowned bool) (skills.ReconcileResult, error) {
	homePath, err := home.Resolve()
	if err != nil {
		return skills.ReconcileResult{}, fmt.Errorf("skills reconcile: %w", err)
	}
	projectID, err := home.ProjectID(root, doc.Project.Name)
	if err != nil {
		return skills.ReconcileResult{}, fmt.Errorf("skills reconcile: project id: %w", err)
	}
	catalog, err := skills.DiscoverCatalog(homePath)
	if err != nil {
		return skills.ReconcileResult{}, err
	}
	own, err := skills.LoadOwnership(homePath, projectID)
	if err != nil {
		return skills.ReconcileResult{}, err
	}
	adaptersSelected := normalizeSelectedAdapters(doc.Adapters.Selected)
	return skills.Reconcile(skills.ReconcileInput{
		Root:           root,
		HomePath:       homePath,
		ProjectID:      projectID,
		Enabled:        doc.SkillPins(),
		Adapters:       adaptersSelected,
		Projectors:     SkillProjectors(),
		Ownership:      own,
		Catalog:        catalog,
		ReclaimUnowned: reclaimUnowned,
	})
}

// InspectSkillHealth returns read-only skill health for Status/Doctor.
func InspectSkillHealth(root string, doc ProjectDocument) skills.SkillHealth {
	homePath, err := home.Resolve()
	if err != nil {
		return skills.SkillHealth{CatalogReadable: false, CatalogError: err.Error()}
	}
	projectID, _ := home.ProjectID(root, doc.Project.Name)
	own, _ := skills.LoadOwnership(homePath, projectID)
	expected, _ := RenderSkillRegistry(doc, homePath)
	return skills.Inspect(skills.InspectInput{
		Root:             root,
		HomePath:         homePath,
		ProjectID:        projectID,
		Enabled:          doc.SkillPins(),
		Adapters:         normalizeSelectedAdapters(doc.Adapters.Selected),
		Projectors:       SkillProjectors(),
		Ownership:        own,
		ExpectedRegistry: expected,
		RegistryRel:      FileSkillRegistry,
	})
}

// AtlasOwnedSkillPaths returns Atlas-owned skill projection roots for an adapter
// from ownership metadata (empty when unknown).
func AtlasOwnedSkillPaths(homePath, projectID, adapter string) []string {
	own, err := skills.LoadOwnership(homePath, projectID)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range own.AdapterEntries(adapter) {
		if strings.TrimSpace(e.RootRel) != "" {
			out = append(out, e.RootRel+"/"+skills.FileSkillMD)
		}
	}
	return out
}

func skillPinsEqual(a, b []skills.Pin) bool {
	if len(a) != len(b) {
		return false
	}
	am := map[string]string{}
	for _, p := range a {
		am[p.ID] = p.Version
	}
	for _, p := range b {
		if am[p.ID] != p.Version {
			return false
		}
	}
	return true
}
