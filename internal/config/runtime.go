package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

const (
	AgentsBaseBegin = "<!-- ATLAS:BASE:BEGIN -->"
	AgentsBaseEnd   = "<!-- ATLAS:BASE:END -->"
	AgentsUserBegin = "<!-- ATLAS:USER:BEGIN -->"
	AgentsUserEnd   = "<!-- ATLAS:USER:END -->"

	defaultAgentsUserBody = "Project-specific instructions go here."
)

// Canonical adapter ids that may appear as AGENTS.md adapter blocks today.
var supportedAgentsAdapters = []string{"cursor", "opencode"}

var adapterMarkerRE = regexp.MustCompile(`(?m)<!--\s*ATLAS:ADAPTER:([A-Z0-9_]+):(BEGIN|END)\s*-->`)

// AgentsMarkers reports which AGENTS.md Atlas markers are present.
type AgentsMarkers struct {
	BaseBegin bool
	BaseEnd   bool
	UserBegin bool
	UserEnd   bool

	// AdapterBlocks maps canonical adapter id ("cursor") → both BEGIN/END present.
	AdapterBlocks map[string]bool
	// FoundAdapters lists adapter ids discovered in content (stable order).
	FoundAdapters []string
}

// Complete reports whether BASE and USER markers are present.
func (m AgentsMarkers) Complete() bool {
	return m.BaseBegin && m.BaseEnd && m.UserBegin && m.UserEnd
}

// HasAdapter reports whether both markers for adapter are present.
func (m AgentsMarkers) HasAdapter(adapter string) bool {
	if m.AdapterBlocks == nil {
		return false
	}
	return m.AdapterBlocks[adapter]
}

// ContractSatisfied reports whether BASE/USER and every selected supported
// adapter block are present.
func (m AgentsMarkers) ContractSatisfied(selected []string) bool {
	if !m.Complete() {
		return false
	}
	for _, adapter := range normalizeSelectedAdapters(selected) {
		if !m.HasAdapter(adapter) {
			return false
		}
	}
	return true
}

// UnselectedAdapters returns adapter blocks present in content but not selected.
func (m AgentsMarkers) UnselectedAdapters(selected []string) []string {
	wanted := map[string]bool{}
	for _, adapter := range normalizeSelectedAdapters(selected) {
		wanted[adapter] = true
	}
	var extra []string
	for _, adapter := range m.FoundAdapters {
		if !wanted[adapter] {
			extra = append(extra, adapter)
		}
	}
	return extra
}

// InspectAgentsMarkers scans AGENTS.md content for Atlas markers. Read-only.
func InspectAgentsMarkers(content []byte) AgentsMarkers {
	text := string(content)
	markers := AgentsMarkers{
		BaseBegin:     strings.Contains(text, AgentsBaseBegin),
		BaseEnd:       strings.Contains(text, AgentsBaseEnd),
		UserBegin:     strings.Contains(text, AgentsUserBegin),
		UserEnd:       strings.Contains(text, AgentsUserEnd),
		AdapterBlocks: map[string]bool{},
	}

	seen := map[string]struct {
		begin bool
		end   bool
	}{}
	var order []string
	for _, match := range adapterMarkerRE.FindAllStringSubmatch(text, -1) {
		if len(match) != 3 {
			continue
		}
		id := strings.ToLower(match[1])
		state, ok := seen[id]
		if !ok {
			order = append(order, id)
		}
		switch match[2] {
		case "BEGIN":
			state.begin = true
		case "END":
			state.end = true
		}
		seen[id] = state
	}
	for _, id := range order {
		state := seen[id]
		markers.AdapterBlocks[id] = state.begin && state.end
		markers.FoundAdapters = append(markers.FoundAdapters, id)
	}
	return markers
}

// AdapterBlockBegin returns the BEGIN marker for an adapter id.
func AdapterBlockBegin(adapter string) string {
	return fmt.Sprintf("<!-- ATLAS:ADAPTER:%s:BEGIN -->", strings.ToUpper(strings.TrimSpace(adapter)))
}

// AdapterBlockEnd returns the END marker for an adapter id.
func AdapterBlockEnd(adapter string) string {
	return fmt.Sprintf("<!-- ATLAS:ADAPTER:%s:END -->", strings.ToUpper(strings.TrimSpace(adapter)))
}

// RuntimeTargets returns allowlisted runtime files for the selected adapters.
// AGENTS.md is always included. Cursor/OpenCode projections and Atlas agents
// are conditional on adapter selection.
func RuntimeTargets(doc ProjectDocument) []string {
	targets := []string{FileAgentsMD}
	for _, adapter := range normalizeSelectedAdapters(doc.Adapters.Selected) {
		switch adapter {
		case "cursor":
			targets = append(targets, FileCursorAtlasMDC)
		case "opencode":
			targets = append(targets, FileOpenCodeAtlas)
		}
	}
	targets = append(targets, AtlasAgentRuntimePaths(doc.Adapters.Selected)...)
	return targets
}

// RenderAgentsMD composes AGENTS.md from the Base Atlas Contract, selected
// adapter blocks, and a preserved ATLAS:USER section when markers are valid.
func RenderAgentsMD(projectName string, contextGraphEnabled bool, selected []string, existing []byte) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	userBody := extractMarkedSection(string(existing), AgentsUserBegin, AgentsUserEnd)
	useDefaultUser := strings.TrimSpace(userBody) == ""
	if useDefaultUser {
		userBody = defaultAgentsUserBody
	}

	graphStatus := "enabled"
	if !contextGraphEnabled {
		graphStatus = "disabled"
	}

	baseBody, err := loadAgentsAsset("agents/base.md")
	if err != nil {
		baseBody = fallbackBaseContract(name, graphStatus)
	}
	baseBody = applyAgentsPlaceholders(baseBody, name, graphStatus)

	var b strings.Builder
	fmt.Fprintf(&b, "# Atlas Project Runtime Contract\n\n")
	fmt.Fprintf(&b, "%s\n%s\n%s\n\n", AgentsBaseBegin, strings.TrimSpace(baseBody), AgentsBaseEnd)

	for _, adapter := range normalizeSelectedAdapters(selected) {
		body, err := loadAgentsAsset("agents/adapters/" + adapter + ".md")
		if err != nil {
			continue
		}
		body = applyAgentsPlaceholders(strings.TrimSpace(body), name, graphStatus)
		fmt.Fprintf(&b, "%s\n%s\n%s\n\n", AdapterBlockBegin(adapter), body, AdapterBlockEnd(adapter))
	}

	// Preserve the exact USER body between markers. Do not inject extra
	// newlines around a preserved body; only format the default body.
	if useDefaultUser {
		fmt.Fprintf(&b, "%s\n%s\n%s\n", AgentsUserBegin, userBody, AgentsUserEnd)
	} else {
		fmt.Fprintf(&b, "%s%s%s\n", AgentsUserBegin, userBody, AgentsUserEnd)
	}
	return b.String()
}

// RenderCursorAtlasMDC builds the minimal Cursor adapter projection.
func RenderCursorAtlasMDC(projectName string) string {
	return renderAdapterFile("adapter-files/cursor/atlas.mdc", projectName)
}

// RenderOpenCodeAtlas builds the minimal OpenCode adapter projection.
func RenderOpenCodeAtlas(projectName string) string {
	return renderAdapterFile("adapter-files/opencode/atlas.md", projectName)
}

func renderAdapterFile(rel, projectName string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	body, err := loadAgentsAsset(rel)
	if err != nil {
		return fallbackAdapterProjection(rel, name)
	}
	return applyAgentsPlaceholders(strings.TrimSpace(body), name, "") + "\n"
}

func loadAgentsAsset(rel string) (string, error) {
	if data, err := home.ReadCanonical(rel); err == nil {
		return string(data), nil
	}
	data, err := assets.Content.ReadFile(rel)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func applyAgentsPlaceholders(body, projectName, graphStatus string) string {
	out := body
	out = strings.ReplaceAll(out, "{{PROJECT_NAME}}", projectName)
	if graphStatus != "" {
		out = strings.ReplaceAll(out, "{{CONTEXT_GRAPH_STATUS}}", graphStatus)
	}
	return out
}

func normalizeSelectedAdapters(selected []string) []string {
	wanted := map[string]bool{}
	for _, adapter := range selected {
		switch adapter {
		case "cursor", "opencode":
			wanted[adapter] = true
		}
	}
	var out []string
	for _, adapter := range supportedAgentsAdapters {
		if wanted[adapter] {
			out = append(out, adapter)
		}
	}
	return out
}

func fallbackBaseContract(projectName, graphStatus string) string {
	return fmt.Sprintf(
		"Atlas project runtime contract for **%s**.\n\nContext Graph preference: **%s**.\nFollow human authority, scope control, and explicit Git authorization.",
		projectName,
		graphStatus,
	)
}

func fallbackAdapterProjection(rel, projectName string) string {
	switch {
	case strings.Contains(rel, "cursor"):
		return fmt.Sprintf("---\ndescription: Atlas Cursor entrypoint for %s\nalwaysApply: true\n---\n\n# Atlas Cursor Entrypoint\n\n- Root `%s` is the project authority; do not bypass it.\n", projectName, FileAgentsMD)
	default:
		return fmt.Sprintf("# Atlas OpenCode Entrypoint\n\nProject: %s\n\n- Root `%s` is the project authority; do not bypass it.\n", projectName, FileAgentsMD)
	}
}

func extractMarkedSection(content, begin, end string) string {
	start := strings.Index(content, begin)
	if start < 0 {
		return ""
	}
	start += len(begin)
	stop := strings.Index(content[start:], end)
	if stop < 0 {
		return ""
	}
	// Return the raw interior between markers; callers decide emptiness.
	return content[start : start+stop]
}

func renderRuntimeFile(rel string, doc ProjectDocument, existing []byte) (string, error) {
	switch rel {
	case FileAgentsMD:
		return RenderAgentsMD(doc.Project.Name, doc.ContextGraphEnabled(), doc.Adapters.Selected, existing), nil
	case FileCursorAtlasMDC:
		return RenderCursorAtlasMDC(doc.Project.Name), nil
	case FileOpenCodeAtlas:
		return RenderOpenCodeAtlas(doc.Project.Name), nil
	default:
		if IsAtlasAgentRuntimePath(rel) {
			return RenderAtlasAgent(filepath.Base(rel))
		}
		return "", fmt.Errorf("unsupported runtime target %q", rel)
	}
}
