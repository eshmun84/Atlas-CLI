package config_test

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestRuntimeTargets(t *testing.T) {
	t.Parallel()

	doc := config.ProjectDocument{}
	got := config.RuntimeTargets(doc)
	if len(got) != 1 || got[0] != config.FileAgentsMD {
		t.Fatalf("default = %#v", got)
	}

	doc.Adapters.Selected = []string{"cursor", "opencode"}
	got = config.RuntimeTargets(doc)
	if len(got) != 3 {
		t.Fatalf("with adapters = %#v", got)
	}

	// Non-canonical casing must not materialize adapter projections.
	doc.Adapters.Selected = []string{"Cursor", "OpenCode"}
	got = config.RuntimeTargets(doc)
	if len(got) != 1 || got[0] != config.FileAgentsMD {
		t.Fatalf("non-canonical adapters must be ignored by targets: %#v", got)
	}
}

func TestRenderAgentsMD_ContractHardening(t *testing.T) {
	t.Parallel()

	existing := "<!-- ATLAS:USER:BEGIN -->\nCustom note\n<!-- ATLAS:USER:END -->"
	got := config.RenderAgentsMD("Demo", true, []byte(existing))
	for _, want := range []string{
		"<!-- ATLAS:MANAGED:BEGIN -->",
		"<!-- ATLAS:MANAGED:END -->",
		"<!-- ATLAS:USER:BEGIN -->",
		"Custom note",
		"<!-- ATLAS:USER:END -->",
		"Demo",
		"## 1. Rules",
		"## 2. Professional Identity",
		"## 3. Persona Scope",
		"## 4. Language",
		"## 5. Tone",
		"## 6. Philosophy",
		"## 7. Expertise",
		"## 8. Behavior",
		"## 9. Contextual Skill Loading",
		"## 10. Agent and Subagent Orchestration",
		"## Context Graph",
		"canonical source of skills",
		"Do not perform Git operations unless a human explicitly requests them.",
		"Conventional Commits",
		"Co-Authored-By",
		"never equal delivery approval",
		"Do not expand scope without a clear proposal",
		"Do not download, install, generate, or copy skills",
		"Load skills only from local paths provided by Atlas",
		"Subagents are bounded workers/reviewers",
		"fall back to inline work",
		"context.graph.enabled",
		"no graph engine, database, embeddings, index, capsules, or context packs",
		"Do not invent graph context",
		"Do not load the full repository by default",
		"Load raw files only when Atlas context references are insufficient for correctness",
		"Do not expect a full skills or agents catalog",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "source ofskills") {
		t.Fatalf("typo source ofskills must be absent:\n%s", got)
	}
	for _, banned := range []string{
		"skills catalog installed",
		"local skills directory",
		"embeddings index",
		"context pack storage",
	} {
		if strings.Contains(strings.ToLower(got), banned) {
			t.Fatalf("banned catalog/engine language %q present:\n%s", banned, got)
		}
	}

	// USER preservation across regeneration.
	again := config.RenderAgentsMD("Demo", true, []byte(got))
	if !strings.Contains(again, "Custom note") {
		t.Fatalf("USER section lost on regenerate:\n%s", again)
	}
	if !strings.Contains(again, "<!-- ATLAS:MANAGED:BEGIN -->") || !strings.Contains(again, "<!-- ATLAS:USER:BEGIN -->") {
		t.Fatalf("markers missing on regenerate:\n%s", again)
	}

	disabled := config.RenderAgentsMD("Demo", false, nil)
	if !strings.Contains(disabled, "Context Graph is **disabled**") {
		t.Fatalf("disabled graph copy missing:\n%s", disabled)
	}
}

func TestRenderAdapterProjections(t *testing.T) {
	t.Parallel()

	cursor := config.RenderCursorAtlasMDC("Demo")
	for _, want := range []string{
		"alwaysApply: true",
		"Atlas adapter projection (Cursor)",
		"Root `AGENTS.md` is authoritative; do not bypass it.",
		"Do not duplicate the full AGENTS.md contract here.",
	} {
		if !strings.Contains(cursor, want) {
			t.Fatalf("cursor missing %q:\n%s", want, cursor)
		}
	}
	if strings.Contains(cursor, "## 1. Rules") || strings.Contains(cursor, "Conventional Commits") {
		t.Fatalf("cursor projection must not duplicate full AGENTS contract:\n%s", cursor)
	}

	opencode := config.RenderOpenCodeAtlas("Demo")
	for _, want := range []string{
		"Atlas adapter projection (OpenCode)",
		".opencode/atlas.md",
		"Root `AGENTS.md` is authoritative; do not bypass it.",
		"Do not duplicate the full AGENTS.md contract here.",
		"Demo",
	} {
		if !strings.Contains(opencode, want) {
			t.Fatalf("opencode missing %q:\n%s", want, opencode)
		}
	}
	if strings.Contains(opencode, "## 1. Rules") || strings.Contains(opencode, "Conventional Commits") {
		t.Fatalf("opencode projection must not duplicate full AGENTS contract:\n%s", opencode)
	}
}
