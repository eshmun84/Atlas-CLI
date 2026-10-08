package project

// Confidence levels for technology detection.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Technology is a detected workspace technology signal.
type Technology struct {
	Name       string
	Source     string
	Confidence string
}

// DiscoverTechnologies maps discovered files to technologies.
func DiscoverTechnologies(files FileInfo) []Technology {
	var techs []Technology

	add := func(name, source string) {
		techs = append(techs, Technology{
			Name:       name,
			Source:     source,
			Confidence: ConfidenceHigh,
		})
	}

	if files.HasGoMod {
		add("Go", "go.mod")
		add("Go modules", "go.mod")
	}
	if files.HasPackageJSON {
		add("Node.js", "package.json")
	}
	if files.HasComposerJSON {
		add("PHP", "composer.json")
	}
	if files.HasPyprojectToml {
		add("Python", "pyproject.toml")
	}
	if files.HasCargoToml {
		add("Rust", "Cargo.toml")
	}
	if files.HasDockerfile {
		add("Docker", "Dockerfile")
	}
	if files.HasDockerCompose {
		add("Docker Compose", "docker-compose.yml|docker-compose.yaml")
	}
	if files.HasAgentsFile {
		add("Agent Instructions", "AGENTS.md")
	}
	if files.HasAtlasConfig {
		add("Atlas", ".atlas/config.yaml")
	}
	if files.HasOpenSpecDir {
		add("OpenSpec", "openspec/")
	}

	return techs
}
