package workspace

import (
	"os"
	"path/filepath"
)

// FileInfo reports presence of important workspace files and directories.
type FileInfo struct {
	HasReadme        bool
	HasGitignore     bool
	HasMakefile      bool
	HasGoMod         bool
	HasPackageJSON   bool
	HasComposerJSON  bool
	HasPyprojectToml bool
	HasCargoToml     bool
	HasDockerfile    bool
	HasDockerCompose bool
	HasAgentsFile    bool
	HasAtlasDir      bool
	HasAtlasConfig   bool
	HasOpenSpecDir   bool
}

// DiscoverFiles inspects root for known project markers. Read-only only.
func DiscoverFiles(root string) (FileInfo, error) {
	return FileInfo{
		HasReadme:        exists(root, "README.md"),
		HasGitignore:     exists(root, ".gitignore"),
		HasMakefile:      exists(root, "Makefile"),
		HasGoMod:         exists(root, "go.mod"),
		HasPackageJSON:   exists(root, "package.json"),
		HasComposerJSON:  exists(root, "composer.json"),
		HasPyprojectToml: exists(root, "pyproject.toml"),
		HasCargoToml:     exists(root, "Cargo.toml"),
		HasDockerfile:    exists(root, "Dockerfile"),
		HasDockerCompose: exists(root, "docker-compose.yml") || exists(root, "docker-compose.yaml"),
		HasAgentsFile:    exists(root, "AGENTS.md"),
		HasAtlasDir:      exists(root, ".atlas"),
		HasAtlasConfig:   exists(root, filepath.Join(".atlas", "config.yaml")),
		HasOpenSpecDir:   exists(root, "openspec"),
	}, nil
}

func exists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}
