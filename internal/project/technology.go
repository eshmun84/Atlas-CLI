package project

// Confidence levels for technology detection.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Technology is a detected workspace technology evidence signal.
// It is not permanent project technology configuration and does not
// materialize skills/agents. Task Capability Discovery (internal/capability)
// consumes these signals as secondary evidence only — never as permanent
// stack→skill selection (DOGFOOD-015).
type Technology struct {
	Name       string
	Source     string
	Confidence string
}

// DiscoverTechnologies maps discovered files to technology evidence signals.
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
