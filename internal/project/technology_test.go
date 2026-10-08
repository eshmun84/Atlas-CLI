package project_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project"
)

func TestDiscoverTechnologies_MapsKnownFiles(t *testing.T) {
	t.Parallel()

	files := project.FileInfo{
		HasGoMod:         true,
		HasPackageJSON:   true,
		HasComposerJSON:  true,
		HasPyprojectToml: true,
		HasCargoToml:     true,
		HasDockerfile:    true,
		HasDockerCompose: true,
		HasAgentsFile:    true,
		HasAtlasConfig:   true,
		HasOpenSpecDir:   true,
	}

	techs := project.DiscoverTechnologies(files)
	got := map[string]project.Technology{}
	for _, tech := range techs {
		got[tech.Name] = tech
	}

	want := map[string]string{
		"Go":                 "go.mod",
		"Go modules":         "go.mod",
		"Node.js":            "package.json",
		"PHP":                "composer.json",
		"Python":             "pyproject.toml",
		"Rust":               "Cargo.toml",
		"Docker":             "Dockerfile",
		"Docker Compose":     "docker-compose.yml|docker-compose.yaml",
		"Agent Instructions": "AGENTS.md",
		"Atlas":              ".atlas/config.yaml",
		"OpenSpec":           "openspec/",
	}

	if len(got) != len(want) {
		t.Fatalf("got %d technologies, want %d: %#v", len(got), len(want), got)
	}

	for name, source := range want {
		tech, ok := got[name]
		if !ok {
			t.Fatalf("missing technology %q", name)
		}
		if tech.Source != source {
			t.Fatalf("%s source = %q, want %q", name, tech.Source, source)
		}
		if tech.Confidence != project.ConfidenceHigh {
			t.Fatalf("%s confidence = %q, want %q", name, tech.Confidence, project.ConfidenceHigh)
		}
	}
}
