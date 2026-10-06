package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"gopkg.in/yaml.v3"
)

// RuntimeManifestDocument is .atlas/runtime-manifest.yaml.
type RuntimeManifestDocument struct {
	SchemaVersion int                    `yaml:"schema_version"`
	GeneratedBy   string                 `yaml:"generated_by"`
	ProjectName   string                 `yaml:"project_name"`
	Adapters      []string               `yaml:"adapters"`
	Entrypoints   []string               `yaml:"entrypoints"`
	Registry      string                 `yaml:"registry"`
	SkillsPolicy  string                 `yaml:"skills_policy"`
	Agents        []RuntimeManifestAgent `yaml:"agents"`
}

// RuntimeManifestAgent records one Atlas agent and its materialized paths.
type RuntimeManifestAgent struct {
	ID    string   `yaml:"id"`
	Kind  string   `yaml:"kind"`
	Files []string `yaml:"files"`
}

// AtlasAgentRuntimePath returns the materialized path for an Atlas agent under an adapter.
func AtlasAgentRuntimePath(adapter, filename string) string {
	switch adapter {
	case "cursor":
		return filepath.ToSlash(filepath.Join(DirCursorAgents, filename))
	case "opencode":
		return filepath.ToSlash(filepath.Join(DirOpenCodeAgents, filename))
	default:
		return ""
	}
}

// AtlasAgentRuntimePaths returns all Atlas agent paths for selected adapters.
func AtlasAgentRuntimePaths(selected []string) []string {
	adapters := normalizeSelectedAdapters(selected)
	var out []string
	for _, adapter := range adapters {
		for _, name := range assets.AtlasAgentFilenames {
			if path := AtlasAgentRuntimePath(adapter, name); path != "" {
				out = append(out, path)
			}
		}
	}
	return out
}

// IsAtlasAgentRuntimePath reports whether rel is an Atlas-owned agent file path.
func IsAtlasAgentRuntimePath(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(rel))
	dir := filepath.ToSlash(filepath.Dir(clean))
	base := filepath.Base(clean)
	if !assets.IsAtlasAgentFilename(base) {
		return false
	}
	return dir == DirCursorAgents || dir == DirOpenCodeAgents
}

// IsDeveloperAgentRuntimePath reports whether rel is a non-Atlas agent under an adapter agents dir.
func IsDeveloperAgentRuntimePath(rel string) bool {
	clean := filepath.ToSlash(filepath.Clean(rel))
	dir := filepath.ToSlash(filepath.Dir(clean))
	base := filepath.Base(clean)
	if dir != DirCursorAgents && dir != DirOpenCodeAgents {
		return false
	}
	if !strings.HasSuffix(base, ".md") {
		return false
	}
	return !assets.IsAtlasAgentFilename(base)
}

// RenderAtlasAgent returns agent content for materialization.
// Prefers Atlas Home canonical copy, then embedded bundled fallback.
func RenderAtlasAgent(filename string) (string, error) {
	name := strings.TrimSpace(filename)
	if !assets.IsAtlasAgentFilename(name) {
		return "", fmt.Errorf("unknown atlas agent %q", filename)
	}
	data, err := home.ReadCanonical("agents/runtime/" + name)
	if err != nil {
		body, readErr := assets.ReadRuntimeAgent(name)
		if readErr != nil {
			return "", err
		}
		return strings.TrimSuffix(body, "\n") + "\n", nil
	}
	return strings.TrimSuffix(string(data), "\n") + "\n", nil
}

// RenderAgentRegistry builds .atlas/agent-registry.md for selected adapters.
// When homePath is set, entries include Atlas Home source paths.
func RenderAgentRegistry(projectName string, selected []string, homePath string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	adapters := normalizeSelectedAdapters(selected)

	var b strings.Builder
	fmt.Fprintf(&b, "# Atlas Agent Registry\n\n")
	fmt.Fprintf(&b, "Project: **%s**\n\n", name)
	if homePath != "" {
		fmt.Fprintf(&b, "Atlas Home: `%s`\n\n", homePath)
	}
	fmt.Fprintf(&b, "This registry lists Atlas-owned runtime agents. Skills remain registry-first via `%s` when present and are not copied into adapter skill folders in this slice.\n\n", FileSkillRegistry)
	fmt.Fprintf(&b, "`AGENTS.md` remains the project authority. Adapter agent files are execution surfaces only.\n\n")

	if len(adapters) == 0 {
		fmt.Fprintf(&b, "## Status\n\n")
		fmt.Fprintf(&b, "No Cursor/OpenCode adapters are selected. Atlas agent files are not materialized under `.cursor/agents/` or `.opencode/agents/`.\n")
		return b.String()
	}

	fmt.Fprintf(&b, "## Selected adapters\n\n")
	for _, adapter := range adapters {
		fmt.Fprintf(&b, "- `%s`\n", adapter)
	}
	fmt.Fprintf(&b, "\n## Agents\n\n")
	for _, filename := range assets.AtlasAgentFilenames {
		id := strings.TrimSuffix(filename, ".md")
		kind := agentKind(id)
		fmt.Fprintf(&b, "### `%s`\n\n", id)
		fmt.Fprintf(&b, "- Kind: `%s`\n", kind)
		fmt.Fprintf(&b, "- Primary conductor: `%v`\n", id == "atlas-orchestrator")
		fmt.Fprintf(&b, "- Source: `bundled`\n")
		if homePath != "" {
			homeAsset := home.Asset{
				Family:    "agents",
				ID:        "agents/runtime/" + filename,
				EmbedPath: "agents/runtime/" + filename,
			}
			fmt.Fprintf(&b, "- Atlas Home: `%s`\n", filepath.ToSlash(home.AssetHomePath(homePath, homeAsset)))
		}
		for _, adapter := range adapters {
			fmt.Fprintf(&b, "- Project path (`%s`): `%s`\n", adapter, AtlasAgentRuntimePath(adapter, filename))
		}
		fmt.Fprintf(&b, "\n")
	}

	fmt.Fprintf(&b, "## Governance notes\n\n")
	fmt.Fprintf(&b, "- Prefer `atlas-orchestrator` for routing and stop/ask decisions.\n")
	fmt.Fprintf(&b, "- SDD agents frame and execute bounded phases; they do not invent OpenSpec command results.\n")
	fmt.Fprintf(&b, "- Review agents produce evidence only; they never authorize delivery.\n")
	fmt.Fprintf(&b, "- Runtime Repair may restore Atlas-owned agents from Atlas Home or bundled fallback.\n")
	fmt.Fprintf(&b, "- Developer-owned non-Atlas agents under adapter `agents/` directories are left untouched.\n")
	return b.String()
}

// BuildRuntimeManifestDocument builds the runtime manifest for selected adapters.
func BuildRuntimeManifestDocument(projectName string, selected []string) RuntimeManifestDocument {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	adapters := normalizeSelectedAdapters(selected)
	entrypoints := []string{FileAgentsMD}
	for _, adapter := range adapters {
		switch adapter {
		case "cursor":
			entrypoints = append(entrypoints, FileCursorAtlasMDC)
		case "opencode":
			entrypoints = append(entrypoints, FileOpenCodeAtlas)
		}
	}

	agents := make([]RuntimeManifestAgent, 0, len(assets.AtlasAgentFilenames))
	for _, filename := range assets.AtlasAgentFilenames {
		id := strings.TrimSuffix(filename, ".md")
		files := make([]string, 0, len(adapters))
		for _, adapter := range adapters {
			if path := AtlasAgentRuntimePath(adapter, filename); path != "" {
				files = append(files, path)
			}
		}
		agents = append(agents, RuntimeManifestAgent{
			ID:    id,
			Kind:  agentKind(id),
			Files: files,
		})
	}

	return RuntimeManifestDocument{
		SchemaVersion: PersistSchemaVersion,
		GeneratedBy:   "atlas",
		ProjectName:   name,
		Adapters:      adapters,
		Entrypoints:   entrypoints,
		Registry:      FileAgentRegistry,
		SkillsPolicy:  "registry-first",
		Agents:        agents,
	}
}

// RenderRuntimeManifestYAML marshals the runtime manifest.
func RenderRuntimeManifestYAML(projectName string, selected []string) (string, error) {
	doc := BuildRuntimeManifestDocument(projectName, selected)
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// RenderAssetsLockYAML marshals a lock for selected adapters without a Home path.
// Prefer RenderAssetsLockYAMLFor when Atlas Home is known.
func RenderAssetsLockYAML(selected []string) (string, error) {
	return RenderAssetsLockYAMLFor("", ProjectDocument{Adapters: AdaptersPersist{Selected: selected}})
}

// AtlasOwnedAssetPaths returns project-relative Atlas-owned paths for diagnostics.
func AtlasOwnedAssetPaths(selected []string) []string {
	adapters := normalizeSelectedAdapters(selected)
	out := []string{
		FileAgentsMD,
		FileAgentRegistry,
		FileRuntimeManifest,
	}
	for _, adapter := range adapters {
		switch adapter {
		case "cursor":
			out = append(out, FileCursorAtlasMDC)
		case "opencode":
			out = append(out, FileOpenCodeAtlas)
		}
	}
	out = append(out, AtlasAgentRuntimePaths(selected)...)
	return out
}

func agentKind(id string) string {
	switch {
	case id == "atlas-orchestrator":
		return "orchestrator"
	case id == "atlas-worker":
		return "worker"
	case strings.HasPrefix(id, "atlas-sdd-"):
		return "sdd"
	case strings.HasPrefix(id, "atlas-review-"):
		return "review"
	default:
		return "agent"
	}
}
