package hookproj

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
	dirHooksMeta  = "hooks"
	fileOwnership = "ownership.yaml"
)

// OwnershipRelPath returns projects/<id>/hooks/ownership.yaml relative to Home.
func OwnershipRelPath(projectID string) string {
	return filepath.ToSlash(filepath.Join("projects", projectID, dirHooksMeta, fileOwnership))
}

// OwnershipPath returns the absolute ownership file path.
func OwnershipPath(homePath, projectID string) string {
	return filepath.Join(homePath, "projects", projectID, dirHooksMeta, fileOwnership)
}

// LoadOwnership loads machine-local hooks ownership metadata.
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
		return OwnershipDocument{}, fmt.Errorf("hooks ownership: read: %w", err)
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return OwnershipDocument{}, fmt.Errorf("hooks ownership: parse: %w", err)
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = OwnershipSchemaVersion
	}
	doc.ProjectID = projectID
	return doc, nil
}

// SaveOwnershipTracked writes ownership metadata and returns footprints for rollback.
func SaveOwnershipTracked(homePath, projectID string, doc OwnershipDocument) (fsafety.TransactionFootprint, error) {
	var fp fsafety.TransactionFootprint
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fp, fmt.Errorf("hooks ownership: home path and project id are required")
	}
	relDir := filepath.ToSlash(filepath.Join("projects", projectID, dirHooksMeta))
	dirs, err := fsafety.SafeMkdirAllCreated(homePath, relDir, 0o700)
	fp.MergeDirs(dirs)
	if err != nil {
		return fp, fmt.Errorf("hooks ownership: mkdir: %w", err)
	}
	doc.SchemaVersion = OwnershipSchemaVersion
	doc.ProjectID = projectID
	if doc.UpdatedAt == "" {
		doc.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return fp, fmt.Errorf("hooks ownership: marshal: %w", err)
	}
	rel := OwnershipRelPath(projectID)
	wrote, writeDirs, err := fsafety.AtomicWriteContainedTracked(homePath, rel, data, 0o600, 0o700, ".atlas-hooks-own-*.tmp")
	fp.MergeDirs(writeDirs)
	if err != nil {
		return fp, fmt.Errorf("hooks ownership: write: %w", err)
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

// LookupKind returns the owned entry for kind under adapter, if any.
func (d OwnershipDocument) LookupKind(adapter, kind string) (OwnedEntry, bool) {
	for _, e := range d.AdapterEntries(adapter) {
		if e.Kind == kind {
			return e, true
		}
	}
	return OwnedEntry{}, false
}

func cloneOwnership(in OwnershipDocument) OwnershipDocument {
	out := OwnershipDocument{
		SchemaVersion: in.SchemaVersion,
		ProjectID:     in.ProjectID,
		UpdatedAt:     in.UpdatedAt,
	}
	for _, a := range in.Adapters {
		entries := make([]OwnedEntry, len(a.Entries))
		copy(entries, a.Entries)
		for i := range entries {
			if len(entries[i].Events) > 0 {
				ev := make([]string, len(entries[i].Events))
				copy(ev, entries[i].Events)
				entries[i].Events = ev
			}
		}
		out.Adapters = append(out.Adapters, AdapterOwnership{
			Adapter: a.Adapter,
			Entries: entries,
		})
	}
	return out
}
