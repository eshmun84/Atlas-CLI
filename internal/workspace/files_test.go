package workspace_test

import (
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestDiscoverFiles_KnownMarkers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	markers := []string{
		"README.md",
		".gitignore",
		"Makefile",
		"go.mod",
		"package.json",
		"composer.json",
		"pyproject.toml",
		"Cargo.toml",
		"Dockerfile",
		"docker-compose.yaml",
		"AGENTS.md",
		"openspec/.keep",
		".atlas/config.yaml",
	}
	for _, marker := range markers {
		writeFile(t, filepath.Join(root, marker), "x")
	}

	info, err := workspace.DiscoverFiles(root)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}

	checks := map[string]bool{
		"README.md":           info.HasReadme,
		".gitignore":          info.HasGitignore,
		"Makefile":            info.HasMakefile,
		"go.mod":              info.HasGoMod,
		"package.json":        info.HasPackageJSON,
		"composer.json":       info.HasComposerJSON,
		"pyproject.toml":      info.HasPyprojectToml,
		"Cargo.toml":          info.HasCargoToml,
		"Dockerfile":          info.HasDockerfile,
		"docker-compose.yaml": info.HasDockerCompose,
		"AGENTS.md":           info.HasAgentsFile,
		".atlas/":             info.HasAtlasDir,
		".atlas/config.yaml":  info.HasAtlasConfig,
		"openspec/":           info.HasOpenSpecDir,
	}
	for name, ok := range checks {
		if !ok {
			t.Fatalf("expected detection for %s", name)
		}
	}
}

func TestDiscoverFiles_EmptyDir(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	info, err := workspace.DiscoverFiles(root)
	if err != nil {
		t.Fatalf("DiscoverFiles: %v", err)
	}
	if info.HasReadme || info.HasGoMod || info.HasAtlasConfig {
		t.Fatalf("expected no markers, got %#v", info)
	}
}
