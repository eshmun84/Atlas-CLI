package context

import (
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

// Home layout for Context Economy under project-scoped Atlas Home storage.
const (
	FileIndexYAML = "index.yaml"
	FileCapsuleMD = "capsule.md"
	DirPacks      = "packs"

	// LegacyContextProjectsRel is the pre-Slice-26 Context Economy root under Atlas Home.
	// Transitional: read-only compatibility for existing payloads.
	LegacyContextProjectsRel = "context/projects"
)

// ContextRoot returns $ATLAS_HOME/projects/<project-id>/context.
func ContextRoot(homePath, projectID string) string {
	return home.ProjectContextDir(homePath, projectID)
}

// ProjectDir returns $ATLAS_HOME/projects/<project-id>/context.
func ProjectDir(homePath, projectID string) string {
	return ContextRoot(homePath, projectID)
}

// LegacyProjectDir returns the transitional $ATLAS_HOME/context/projects/<project-id>.
func LegacyProjectDir(homePath, projectID string) string {
	return filepath.Join(homePath, filepath.FromSlash(LegacyContextProjectsRel), projectID)
}

// IndexPath returns the project index.yaml path under Atlas Home (canonical layout).
func IndexPath(homePath, projectID string) string {
	return filepath.Join(ProjectDir(homePath, projectID), FileIndexYAML)
}

// CapsulePath returns the project capsule.md path under Atlas Home (canonical layout).
func CapsulePath(homePath, projectID string) string {
	return filepath.Join(ProjectDir(homePath, projectID), FileCapsuleMD)
}

// PacksDir returns the packs directory for a project under Atlas Home.
func PacksDir(homePath, projectID string) string {
	return filepath.Join(ProjectDir(homePath, projectID), DirPacks)
}

// PackPath returns one pack YAML path under Atlas Home.
func PackPath(homePath, projectID, packID string) string {
	return filepath.Join(PacksDir(homePath, projectID), packID+".yaml")
}

// ResolveContextDir picks the canonical Context Economy directory when present,
// otherwise falls back to the transitional legacy path for read-only inspection.
func ResolveContextDir(homePath, projectID string) string {
	canonical := ProjectDir(homePath, projectID)
	if fileExists(filepath.Join(canonical, FileIndexYAML)) {
		return canonical
	}
	legacy := LegacyProjectDir(homePath, projectID)
	if fileExists(filepath.Join(legacy, FileIndexYAML)) {
		return legacy
	}
	return canonical
}

// HomeRelContext returns the Home-relative context path for state refs.
func HomeRelContext(projectID string) string {
	return filepath.ToSlash(filepath.Join(home.DirProjects, projectID, home.ProjectDirContext))
}

// ResolveHome returns the Atlas Home path without creating directories.
func ResolveHome() (string, error) {
	return home.Resolve()
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
