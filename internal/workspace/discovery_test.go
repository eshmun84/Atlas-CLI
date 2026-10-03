package workspace_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func TestDiscover_RejectsEmptyRoot(t *testing.T) {
	t.Parallel()

	if _, err := workspace.Discover(""); err == nil {
		t.Fatal("expected error for empty root")
	}
}

func TestDiscover_RejectsMissingRoot(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	if _, err := workspace.Discover(missing); err == nil {
		t.Fatal("expected error for missing root")
	}
}

func TestDiscover_RejectsFileRoot(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir.txt")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := workspace.Discover(filePath); err == nil {
		t.Fatal("expected error when root is a file")
	}
}

func TestDiscover_DetectsMarkers(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# demo")
	writeFile(t, filepath.Join(root, "go.mod"), "module demo")
	writeFile(t, filepath.Join(root, "package.json"), `{"name":"demo"}`)
	writeFile(t, filepath.Join(root, "composer.json"), `{"name":"demo/app"}`)
	writeFile(t, filepath.Join(root, "Dockerfile"), "FROM scratch")
	writeFile(t, filepath.Join(root, "docker-compose.yml"), "services: {}")
	writeFile(t, filepath.Join(root, "AGENTS.md"), "# agents")
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatalf("mkdir .atlas: %v", err)
	}
	writeFile(t, filepath.Join(root, ".atlas", "config.yaml"), "atlas: {}\n")

	result, err := workspace.Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if !result.Files.HasReadme {
		t.Fatal("expected README.md")
	}
	if !result.Files.HasGoMod {
		t.Fatal("expected go.mod")
	}
	if !result.Files.HasPackageJSON {
		t.Fatal("expected package.json")
	}
	if !result.Files.HasComposerJSON {
		t.Fatal("expected composer.json")
	}
	if !result.Files.HasDockerfile {
		t.Fatal("expected Dockerfile")
	}
	if !result.Files.HasDockerCompose {
		t.Fatal("expected docker-compose.yml")
	}
	if !result.Files.HasAgentsFile {
		t.Fatal("expected AGENTS.md")
	}
	if !result.Files.HasAtlasDir {
		t.Fatal("expected .atlas/")
	}
	if !result.Files.HasAtlasConfig {
		t.Fatal("expected .atlas/config.yaml")
	}
	if result.Git.IsRepo {
		t.Fatal("temp fixture should not be a git repo")
	}
}

func TestDiscover_DockerComposeYAML(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeFile(t, filepath.Join(root, "docker-compose.yaml"), "services: {}")

	result, err := workspace.Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !result.Files.HasDockerCompose {
		t.Fatal("expected docker-compose.yaml detection")
	}
}

func TestDiscover_NonGitDirectoryDoesNotFail(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result, err := workspace.Discover(root)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if result.Git.IsRepo {
		t.Fatal("expected IsRepo=false")
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
