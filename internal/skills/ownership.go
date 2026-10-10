package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

const (
	dirSkillsMeta = "skills"
	fileOwnership = "ownership.yaml"
)

// OwnershipRelPath returns projects/<id>/skills/ownership.yaml relative to Home.
func OwnershipRelPath(projectID string) string {
	return filepath.ToSlash(filepath.Join("projects", projectID, dirSkillsMeta, fileOwnership))
}

// OwnershipPath returns the absolute ownership file path.
func OwnershipPath(homePath, projectID string) string {
	return filepath.Join(homePath, "projects", projectID, dirSkillsMeta, fileOwnership)
}

// LoadOwnership loads machine-local skill ownership metadata.
func LoadOwnership(homePath, projectID string) (OwnershipDocument, error) {
	projectID = strings.TrimSpace(projectID)
	doc := OwnershipDocument{
		SchemaVersion: OwnershipSchemaVersion,
		ProjectID:     projectID,
	}
	if homePath == "" || projectID == "" {
		return doc, nil
	}
	data, err := fsafety.ReadFileContained(homePath, OwnershipRelPath(projectID))
	if err != nil {
		if os.IsNotExist(err) {
			return doc, nil
		}
		return OwnershipDocument{}, fmt.Errorf("skills ownership: read: %w", err)
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return OwnershipDocument{}, fmt.Errorf("skills ownership: parse: %w", err)
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = OwnershipSchemaVersion
	}
	doc.ProjectID = projectID
	return doc, nil
}

// SaveOwnership writes ownership metadata with restrictive permissions.
func SaveOwnership(homePath, projectID string, doc OwnershipDocument) error {
	_, err := SaveOwnershipTracked(homePath, projectID, doc)
	return err
}

// SaveOwnershipTracked is SaveOwnership plus mkdir/write footprints for rollback.
func SaveOwnershipTracked(homePath, projectID string, doc OwnershipDocument) (fsafety.TransactionFootprint, error) {
	var fp fsafety.TransactionFootprint
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fp, fmt.Errorf("skills ownership: home path and project id are required")
	}
	relDir := filepath.ToSlash(filepath.Join("projects", projectID, dirSkillsMeta))
	dirs, err := fsafety.SafeMkdirAllCreated(homePath, relDir, 0o700)
	fp.MergeDirs(dirs)
	if err != nil {
		return fp, fmt.Errorf("skills ownership: mkdir: %w", err)
	}
	doc.SchemaVersion = OwnershipSchemaVersion
	doc.ProjectID = projectID
	if doc.UpdatedAt == "" {
		doc.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return fp, fmt.Errorf("skills ownership: marshal: %w", err)
	}
	rel := OwnershipRelPath(projectID)
	wrote, writeDirs, err := fsafety.AtomicWriteContainedTracked(homePath, rel, data, 0o600, 0o700, ".atlas-skill-own-*.tmp")
	fp.MergeDirs(writeDirs)
	if err != nil {
		return fp, fmt.Errorf("skills ownership: write: %w", err)
	}
	fp.AddFile(wrote)
	return fp, nil
}

// AdapterEntries returns ownership entries for one adapter.
func (d OwnershipDocument) AdapterEntries(adapter string) []OwnedEntry {
	for _, a := range d.Adapters {
		if a.Adapter == adapter {
			return append([]OwnedEntry(nil), a.Entries...)
		}
	}
	return nil
}

// SetAdapterEntries replaces ownership entries for one adapter.
func (d *OwnershipDocument) SetAdapterEntries(adapter string, entries []OwnedEntry) {
	if d == nil {
		return
	}
	for i := range d.Adapters {
		if d.Adapters[i].Adapter == adapter {
			d.Adapters[i].Entries = append([]OwnedEntry(nil), entries...)
			return
		}
	}
	d.Adapters = append(d.Adapters, AdapterOwnership{
		Adapter: adapter,
		Entries: append([]OwnedEntry(nil), entries...),
	})
}

// ClearAdapter removes ownership for one adapter.
func (d *OwnershipDocument) ClearAdapter(adapter string) {
	if d == nil {
		return
	}
	out := d.Adapters[:0]
	for _, a := range d.Adapters {
		if a.Adapter == adapter {
			continue
		}
		out = append(out, a)
	}
	d.Adapters = out
}

// Lookup returns the owned entry for skill id under adapter, if any.
func (d OwnershipDocument) Lookup(adapter, skillID string) (OwnedEntry, bool) {
	for _, e := range d.AdapterEntries(adapter) {
		if e.SkillID == skillID {
			return e, true
		}
	}
	return OwnedEntry{}, false
}
