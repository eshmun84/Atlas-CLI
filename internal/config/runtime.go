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

// Testable seam for bundled agent/projection assets; production uses Home then embed.
var loadAgentsAssetFn = loadAgentsAssetDefault

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
// Missing base or selected adapter assets fail closed (no fabricated fallback).
func RenderAgentsMD(projectName string, contextGraphEnabled bool, selected []string, existing []byte) (string, error) {
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

	baseBody, err := loadAgentsAssetFn("agents/base.md")
	if err != nil {
		return "", fmt.Errorf("render AGENTS.md base contract: %w", err)
	}
	baseBody = applyAgentsPlaceholders(baseBody, name, graphStatus)

	var b strings.Builder
	// Single document H1 outside markers; BASE asset body must not repeat the H1.
	fmt.Fprintf(&b, "# Atlas Project Runtime Contract\n\n")
	fmt.Fprintf(&b, "%s\n%s\n%s\n\n", AgentsBaseBegin, strings.TrimSpace(baseBody), AgentsBaseEnd)

	for _, adapter := range normalizeSelectedAdapters(selected) {
		body, err := loadAgentsAssetFn("agents/adapters/" + adapter + ".md")
		if err != nil {
			return "", fmt.Errorf("render AGENTS.md adapter %s: %w", adapter, err)
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
	return b.String(), nil
}

// RenderCursorAtlasMDC builds the minimal Cursor adapter projection.
func RenderCursorAtlasMDC(projectName string) (string, error) {
	return renderAdapterFile("adapter-files/cursor/atlas.mdc", projectName)
}

// RenderOpenCodeAtlas builds the minimal OpenCode adapter projection.
func RenderOpenCodeAtlas(projectName string) (string, error) {
	return renderAdapterFile("adapter-files/opencode/atlas.md", projectName)
}

func renderAdapterFile(rel, projectName string) (string, error) {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	body, err := loadAgentsAssetFn(rel)
	if err != nil {
		return "", fmt.Errorf("render adapter projection %s: %w", rel, err)
	}
	return applyAgentsPlaceholders(strings.TrimSpace(body), name, "") + "\n", nil
}

func loadAgentsAssetDefault(rel string) (string, error) {
	data, err := home.ReadCanonical(rel)
	if err != nil {
		data, embErr := assets.Content.ReadFile(rel)
		if embErr != nil {
			return "", fmt.Errorf("asset %s: home canonical unavailable (%v); embedded: %w", rel, err, embErr)
		}
		return string(data), nil
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
		return RenderAgentsMD(doc.Project.Name, doc.ContextGraphEnabled(), doc.Adapters.Selected, existing)
	case FileCursorAtlasMDC:
		return RenderCursorAtlasMDC(doc.Project.Name)
	case FileOpenCodeAtlas:
		return RenderOpenCodeAtlas(doc.Project.Name)
	default:
		if IsAtlasAgentRuntimePath(rel) {
			return RenderAtlasAgent(filepath.Base(rel))
		}
		return "", fmt.Errorf("unsupported runtime target %q", rel)
	}
}
