package context

import (
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

// Home layout under Atlas Home for Context Economy v0.
const (
	DirContext         = "context"
	DirContextProjects = "context/projects"
	FileIndexYAML      = "index.yaml"
	FileCapsuleMD      = "capsule.md"
	DirPacks           = "packs"
)

// ContextRoot returns $ATLAS_HOME/context.
func ContextRoot(homePath string) string {
	return filepath.Join(homePath, DirContext)
}

// ProjectsRoot returns $ATLAS_HOME/context/projects.
func ProjectsRoot(homePath string) string {
	return filepath.Join(homePath, filepath.FromSlash(DirContextProjects))
}

// ProjectDir returns $ATLAS_HOME/context/projects/<project-id>.
func ProjectDir(homePath, projectID string) string {
	return filepath.Join(ProjectsRoot(homePath), projectID)
}

// IndexPath returns the project index.yaml path under Atlas Home.
func IndexPath(homePath, projectID string) string {
	return filepath.Join(ProjectDir(homePath, projectID), FileIndexYAML)
}

// CapsulePath returns the project capsule.md path under Atlas Home.
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

// ResolveHome returns the Atlas Home path without creating directories.
func ResolveHome() (string, error) {
	return home.Resolve()
}
