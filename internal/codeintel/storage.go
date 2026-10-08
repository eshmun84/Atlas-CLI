package codeintel

import (
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

const (
	// FileGraphDB is the Atlas-owned CodeGraph database filename under Home.
	FileGraphDB = "graph.db"
	// FileMetadata is Atlas-owned provider metadata (written only by explicit lifecycle).
	FileMetadata = "metadata.json"
)

// ProviderStorageDir returns $ATLAS_HOME/projects/<project-id>/<provider>/.
// Read-only helper: does not create directories.
func ProviderStorageDir(homePath, projectID string, provider ProviderID) string {
	homePath = strings.TrimSpace(homePath)
	projectID = strings.TrimSpace(projectID)
	id := strings.TrimSpace(string(provider))
	if homePath == "" || projectID == "" || id == "" {
		return ""
	}
	return filepath.Join(home.ProjectRoot(homePath, projectID), id)
}

// GraphDBPath returns the Atlas-owned graph database path for a provider.
func GraphDBPath(homePath, projectID string, provider ProviderID) string {
	dir := ProviderStorageDir(homePath, projectID, provider)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, FileGraphDB)
}

// MetadataPath returns the Atlas-owned metadata path for a provider.
func MetadataPath(homePath, projectID string, provider ProviderID) string {
	dir := ProviderStorageDir(homePath, projectID, provider)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, FileMetadata)
}
