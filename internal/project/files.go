package project

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
		HasReadme:        Exists(root, "README.md"),
		HasGitignore:     Exists(root, ".gitignore"),
		HasMakefile:      Exists(root, "Makefile"),
		HasGoMod:         Exists(root, "go.mod"),
		HasPackageJSON:   Exists(root, "package.json"),
		HasComposerJSON:  Exists(root, "composer.json"),
		HasPyprojectToml: Exists(root, "pyproject.toml"),
		HasCargoToml:     Exists(root, "Cargo.toml"),
		HasDockerfile:    Exists(root, "Dockerfile"),
		HasDockerCompose: Exists(root, "docker-compose.yml") || Exists(root, "docker-compose.yaml"),
		HasAgentsFile:    Exists(root, "AGENTS.md"),
		HasAtlasDir:      Exists(root, ".atlas"),
		HasAtlasConfig:   Exists(root, filepath.Join(".atlas", "config.yaml")),
		HasOpenSpecDir:   Exists(root, "openspec"),
	}, nil
}

func Exists(root, name string) bool {
	_, err := os.Stat(filepath.Join(root, name))
	return err == nil
}
