package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

const (
	// DirMCP is the machine-local MCP metadata directory under a Home project.
	DirMCP = "mcp"
	// FileOwnership is ownership/materialization metadata (never repo-owned).
	FileOwnership = "ownership.yaml"
)

// OwnershipRelPath returns projects/<id>/mcp/ownership.yaml relative to Home.
func OwnershipRelPath(projectID string) string {
	return filepath.ToSlash(filepath.Join("projects", projectID, DirMCP, FileOwnership))
}

// OwnershipPath returns the absolute ownership file path.
func OwnershipPath(homePath, projectID string) string {
	return filepath.Join(homePath, "projects", projectID, DirMCP, FileOwnership)
}

// LoadOwnership loads machine-local ownership metadata via symlink-safe contained
// reads under explicit Atlas Home. Missing file => empty doc.
// Symlink parents/leaves, directories, and containment failures return error —
// external forged ownership documents are never trusted.
func LoadOwnership(homePath, projectID string) (OwnershipDocument, error) {
	projectID = strings.TrimSpace(projectID)
	doc := OwnershipDocument{
		SchemaVersion: OwnershipSchemaVersion,
		ProjectID:     projectID,
	}
	if homePath == "" || projectID == "" {
		return doc, nil
	}
	rel := OwnershipRelPath(projectID)
	data, err := fsafety.ReadFileContained(homePath, rel)
	if err != nil {
		if os.IsNotExist(err) {
			return doc, nil
		}
		return doc, fmt.Errorf("mcp ownership: read: %w", err)
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return OwnershipDocument{}, fmt.Errorf("mcp ownership: parse: %w", err)
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = OwnershipSchemaVersion
	}
	doc.ProjectID = projectID
	return doc, nil
}

// SaveOwnership writes ownership metadata with restrictive permissions.
// Refuses symlink path components under Atlas Home.
func SaveOwnership(homePath, projectID string, doc OwnershipDocument) error {
	projectID = strings.TrimSpace(projectID)
	if homePath == "" || projectID == "" {
		return fmt.Errorf("mcp ownership: home path and project id are required")
	}
	mcpRel := filepath.ToSlash(filepath.Join("projects", projectID, DirMCP))
	if err := fsafety.SafeMkdirAll(homePath, mcpRel, 0o700); err != nil {
		return fmt.Errorf("mcp ownership: mkdir: %w", err)
	}
	doc.SchemaVersion = OwnershipSchemaVersion
	doc.ProjectID = projectID
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("mcp ownership: marshal: %w", err)
	}
	rel := OwnershipRelPath(projectID)
	if err := fsafety.AtomicWriteContainedDir(homePath, rel, data, 0o600, 0o700, ".atlas-own-*.tmp"); err != nil {
		return fmt.Errorf("mcp ownership: write: %w", err)
	}
	return nil
}

// AdapterEntries returns ownership entries for one adapter.
func (d OwnershipDocument) AdapterEntries(adapter AdapterID) []OwnedEntry {
	for _, a := range d.Adapters {
		if a.Adapter == adapter {
			return append([]OwnedEntry(nil), a.Entries...)
		}
	}
	return nil
}

// SetAdapterEntries replaces ownership entries for one adapter.
func (d *OwnershipDocument) SetAdapterEntries(adapter AdapterID, entries []OwnedEntry) {
	if d == nil {
		return
	}
	found := false
	for i := range d.Adapters {
		if d.Adapters[i].Adapter == adapter {
			d.Adapters[i].Entries = append([]OwnedEntry(nil), entries...)
			found = true
			break
		}
	}
	if !found {
		d.Adapters = append(d.Adapters, AdapterOwnership{
			Adapter: adapter,
			Entries: append([]OwnedEntry(nil), entries...),
		})
	}
}

// ClearAdapter removes ownership for one adapter (after managed projection removal).
func (d *OwnershipDocument) ClearAdapter(adapter AdapterID) {
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
