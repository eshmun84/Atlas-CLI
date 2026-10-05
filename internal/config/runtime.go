package config

import (
	"fmt"
	"strings"
)

const (
	AgentsManagedBegin = "<!-- ATLAS:MANAGED:BEGIN -->"
	AgentsManagedEnd   = "<!-- ATLAS:MANAGED:END -->"
	AgentsUserBegin    = "<!-- ATLAS:USER:BEGIN -->"
	AgentsUserEnd      = "<!-- ATLAS:USER:END -->"
)

// RuntimeTargets returns allowlisted runtime files for the selected adapters.
// AGENTS.md is always included. Cursor/OpenCode files are conditional.
func RuntimeTargets(doc ProjectDocument) []string {
	targets := []string{FileAgentsMD}
	for _, adapter := range doc.Adapters.Selected {
		switch strings.ToLower(strings.TrimSpace(adapter)) {
		case "cursor":
			targets = append(targets, FileCursorAtlasMDC)
		case "opencode":
			targets = append(targets, FileOpenCodeAtlas)
		}
	}
	return targets
}

// RenderAgentsMD builds AGENTS.md with the compact Atlas gateway blueprint.
// When existing content has a USER section, that body is preserved.
func RenderAgentsMD(projectName string, contextGraphEnabled bool, existing []byte) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	userBody := extractMarkedSection(string(existing), AgentsUserBegin, AgentsUserEnd)
	if strings.TrimSpace(userBody) == "" {
		userBody = "Add project-specific agent instructions here.\nAtlas preserves this section on later updates when possible."
	} else {
		userBody = strings.TrimSpace(userBody)
	}

	graphPref := "enabled"
	if !contextGraphEnabled {
		graphPref = "disabled"
	}

	var managed strings.Builder
	fmt.Fprintf(&managed, "# AGENTS.md\n\n")
	fmt.Fprintf(&managed, "Atlas project gateway for **%s**.\n\n", name)
	fmt.Fprintf(&managed, "This repository is a compact gateway. Atlas Home / the local Atlas framework environment is the canonical source of skills, agents, rules, personas, templates, adapter instructions, and runtime contracts. Do not expect a full skills or agents catalog to be copied into this project.\n\n")
	fmt.Fprintf(&managed, "Read `%s`, `%s`, and selected adapter projections before acting.\n\n", FileAgentsMD, FileConfig)

	fmt.Fprintf(&managed, "## 1. Rules\n\n")
	fmt.Fprintf(&managed, "- Follow Atlas governance and project configuration under `%s`.\n", FileConfig)
	fmt.Fprintf(&managed, "- Prefer Atlas-owned state under `%s/` for Atlas configuration.\n", DirAtlas)
	fmt.Fprintf(&managed, "- Do not store secrets, credentials, or tokens in this file.\n")
	fmt.Fprintf(&managed, "- Do not invent missing Atlas Home assets or project context.\n\n")

	fmt.Fprintf(&managed, "## 2. Professional Identity\n\n")
	fmt.Fprintf(&managed, "- Act as a capable engineering agent operating through Atlas.\n")
	fmt.Fprintf(&managed, "- Stay practical, verification-minded, and aligned with the project's Atlas setup.\n\n")

	fmt.Fprintf(&managed, "## 3. Persona Scope\n\n")
	fmt.Fprintf(&managed, "- Stay within the active Atlas persona or role when Atlas provides one.\n")
	fmt.Fprintf(&managed, "- Do not assume every persona definition is vendored inside this repository.\n\n")

	fmt.Fprintf(&managed, "## 4. Language\n\n")
	fmt.Fprintf(&managed, "- Prefer the project's working language.\n")
	fmt.Fprintf(&managed, "- Keep technical terms clear and unambiguous.\n\n")

	fmt.Fprintf(&managed, "## 5. Tone\n\n")
	fmt.Fprintf(&managed, "- Direct, precise, and compact.\n")
	fmt.Fprintf(&managed, "- Prefer actionable guidance over ceremony.\n\n")

	fmt.Fprintf(&managed, "## 6. Philosophy\n\n")
	fmt.Fprintf(&managed, "- Keep the project surface small: gateway instructions, config/state, and adapter projections.\n")
	fmt.Fprintf(&managed, "- Prefer contextual loading from Atlas Home over copying catalogs into the repo.\n\n")

	fmt.Fprintf(&managed, "## 7. Expertise\n\n")
	fmt.Fprintf(&managed, "- Use Atlas Home expertise sources when available.\n")
	fmt.Fprintf(&managed, "- Do not assume skills, agents, or templates are installed locally in this project.\n\n")

	fmt.Fprintf(&managed, "## 8. Behavior\n\n")
	fmt.Fprintf(&managed, "- Inspect AGENTS.md, `%s`, and adapter files before changing project behavior.\n", FileConfig)
	fmt.Fprintf(&managed, "- Do not download, install, or resolve remote asset catalogs during normal work.\n")
	fmt.Fprintf(&managed, "- Do not materialize CLAUDE.md, GEMINI.md, `.agents/`, or `.claude/` unless a future Atlas slice explicitly allows it.\n\n")

	fmt.Fprintf(&managed, "## 9. Contextual Skill Loading\n\n")
	fmt.Fprintf(&managed, "- Load skills contextually from Atlas Home when Atlas enables them.\n")
	fmt.Fprintf(&managed, "- Do not expect a complete skills catalog inside this repository.\n\n")

	fmt.Fprintf(&managed, "## 10. Agent and Subagent Orchestration\n\n")
	fmt.Fprintf(&managed, "- Prefer Atlas-provided agent and subagent contracts when available.\n")
	fmt.Fprintf(&managed, "- Do not assume this project contains a full local agent catalog.\n\n")

	fmt.Fprintf(&managed, "## Context Graph\n\n")
	fmt.Fprintf(&managed, "- Project preference: Context Graph is **%s** (`context.graph.enabled`).\n", graphPref)
	fmt.Fprintf(&managed, "- Use Context Graph only when Atlas enables or provides it.\n")
	fmt.Fprintf(&managed, "- Do not assume the graph lives inside this project.\n")
	fmt.Fprintf(&managed, "- Do not invent graph context when it is unavailable.")

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n%s\n\n", AgentsManagedBegin, managed.String(), AgentsManagedEnd)
	fmt.Fprintf(&b, "%s\n%s\n%s\n", AgentsUserBegin, userBody, AgentsUserEnd)
	return b.String()
}

// RenderCursorAtlasMDC builds the Cursor adapter rule file.
func RenderCursorAtlasMDC(projectName string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "description: Atlas-managed project rules for %s\n", name)
	fmt.Fprintf(&b, "alwaysApply: true\n")
	fmt.Fprintf(&b, "---\n\n")
	fmt.Fprintf(&b, "# Atlas\n\n")
	fmt.Fprintf(&b, "This project is an Atlas gateway for **%s**.\n\n", name)
	fmt.Fprintf(&b, "- Follow root `%s` (managed + user sections).\n", FileAgentsMD)
	fmt.Fprintf(&b, "- Project configuration lives under `%s`.\n", FileConfig)
	fmt.Fprintf(&b, "- Atlas Home remains the canonical source of skills, agents, rules, and contracts.\n")
	fmt.Fprintf(&b, "- Do not store secrets in adapter files.\n")
	fmt.Fprintf(&b, "- Do not expect a full skills/agents catalog inside this repository.\n")
	return b.String()
}

// RenderOpenCodeAtlas builds the project-local OpenCode adapter file.
func RenderOpenCodeAtlas(projectName string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Atlas OpenCode adapter\n\n")
	fmt.Fprintf(&b, "Project: %s\n\n", name)
	fmt.Fprintf(&b, "This file is the Atlas project-local OpenCode runtime adapter (`%s`).\n\n", FileOpenCodeAtlas)
	fmt.Fprintf(&b, "- Follow root %s for gateway instructions.\n", FileAgentsMD)
	fmt.Fprintf(&b, "- Read Atlas configuration from `%s`.\n", FileConfig)
	fmt.Fprintf(&b, "- Atlas Home remains the canonical source of skills, agents, rules, and contracts.\n")
	fmt.Fprintf(&b, "- Do not store secrets, credentials, or tokens here.\n")
	fmt.Fprintf(&b, "- Do not expect a full skills/agents catalog inside this repository.\n")
	return b.String()
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
	return strings.TrimSpace(content[start : start+stop])
}

func renderRuntimeFile(rel string, doc ProjectDocument, existing []byte) (string, error) {
	switch rel {
	case FileAgentsMD:
		return RenderAgentsMD(doc.Project.Name, doc.ContextGraphEnabled(), existing), nil
	case FileCursorAtlasMDC:
		return RenderCursorAtlasMDC(doc.Project.Name), nil
	case FileOpenCodeAtlas:
		return RenderOpenCodeAtlas(doc.Project.Name), nil
	default:
		return "", fmt.Errorf("unsupported runtime target %q", rel)
	}
}
