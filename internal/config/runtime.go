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
		switch adapter {
		case "cursor":
			targets = append(targets, FileCursorAtlasMDC)
		case "opencode":
			targets = append(targets, FileOpenCodeAtlas)
		}
	}
	return targets
}

// RenderAgentsMD builds AGENTS.md with the compact Atlas gateway contract.
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
	fmt.Fprintf(&managed, "- Do not invent missing Atlas Home assets or project context.\n")
	fmt.Fprintf(&managed, "- Do not perform Git operations unless a human explicitly requests them.\n")
	fmt.Fprintf(&managed, "- When commit text is requested, use Conventional Commits.\n")
	fmt.Fprintf(&managed, "- Do not add `Co-Authored-By` or any attribution to AI/tools.\n")
	fmt.Fprintf(&managed, "- Tests, review, evidence, or delegation never equal delivery approval.\n")
	fmt.Fprintf(&managed, "- Do not expand scope without a clear proposal and human agreement.\n\n")

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
	fmt.Fprintf(&managed, "- Inspect AGENTS.md, `%s`, and adapter projections before changing project behavior.\n", FileConfig)
	fmt.Fprintf(&managed, "- Do not download, install, generate, or copy skills during normal work.\n")
	fmt.Fprintf(&managed, "- Do not resolve remote asset catalogs during normal work.\n")
	fmt.Fprintf(&managed, "- Do not materialize CLAUDE.md, GEMINI.md, `.agents/`, or `.claude/` unless a future Atlas slice explicitly allows it.\n\n")

	fmt.Fprintf(&managed, "## 9. Contextual Skill Loading\n\n")
	fmt.Fprintf(&managed, "- Load skills only from local paths provided by Atlas.\n")
	fmt.Fprintf(&managed, "- Do not expect a complete skills catalog inside this repository.\n\n")

	fmt.Fprintf(&managed, "## 10. Agent and Subagent Orchestration\n\n")
	fmt.Fprintf(&managed, "- Prefer Atlas-provided agent and subagent contracts when available.\n")
	fmt.Fprintf(&managed, "- Subagents are bounded workers/reviewers; the primary agent keeps responsibility.\n")
	fmt.Fprintf(&managed, "- If safe runtime-native delegation is unavailable, fall back to inline work.\n")
	fmt.Fprintf(&managed, "- Do not assume this project contains a full local agent catalog.\n\n")

	fmt.Fprintf(&managed, "## Context Graph\n\n")
	fmt.Fprintf(&managed, "- Project preference: Context Graph is **%s** (`context.graph.enabled`).\n", graphPref)
	fmt.Fprintf(&managed, "- Context Graph is a preference/context aid only: no graph engine, database, embeddings, index, capsules, or context packs in this project.\n")
	fmt.Fprintf(&managed, "- Use Context Graph only when Atlas enables or provides it.\n")
	fmt.Fprintf(&managed, "- Do not assume the graph lives inside this project.\n")
	fmt.Fprintf(&managed, "- Do not invent graph context when it is unavailable.\n")
	fmt.Fprintf(&managed, "- Do not load the full repository by default.\n")
	fmt.Fprintf(&managed, "- Load raw files only when Atlas context references are insufficient for correctness.")

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n%s\n\n", AgentsManagedBegin, managed.String(), AgentsManagedEnd)
	fmt.Fprintf(&b, "%s\n%s\n%s\n", AgentsUserBegin, userBody, AgentsUserEnd)
	return b.String()
}

// RenderCursorAtlasMDC builds the Cursor adapter projection.
func RenderCursorAtlasMDC(projectName string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "description: Atlas adapter projection for %s\n", name)
	fmt.Fprintf(&b, "alwaysApply: true\n")
	fmt.Fprintf(&b, "---\n\n")
	fmt.Fprintf(&b, "# Atlas adapter projection (Cursor)\n\n")
	fmt.Fprintf(&b, "- Root `%s` is authoritative; do not bypass it.\n", FileAgentsMD)
	fmt.Fprintf(&b, "- This file is an Atlas adapter projection, not a full contract or catalog.\n")
	fmt.Fprintf(&b, "- Do not duplicate the full AGENTS.md contract here.\n")
	fmt.Fprintf(&b, "- Project configuration lives under `%s`.\n", FileConfig)
	return b.String()
}

// RenderOpenCodeAtlas builds the OpenCode adapter projection.
func RenderOpenCodeAtlas(projectName string) string {
	name := strings.TrimSpace(projectName)
	if name == "" {
		name = "this project"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Atlas adapter projection (OpenCode)\n\n")
	fmt.Fprintf(&b, "Project: %s\n\n", name)
	fmt.Fprintf(&b, "- Root `%s` is authoritative; do not bypass it.\n", FileAgentsMD)
	fmt.Fprintf(&b, "- This file (`%s`) is an Atlas adapter projection, not a full contract or catalog.\n", FileOpenCodeAtlas)
	fmt.Fprintf(&b, "- Do not duplicate the full AGENTS.md contract here.\n")
	fmt.Fprintf(&b, "- Project configuration lives under `%s`.\n", FileConfig)
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
