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
}

func TestRenderAgentsMD_PreservesUserAndBlueprint(t *testing.T) {
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
		"context.graph.enabled",
		"Do not assume the graph lives inside this project",
		"Do not invent graph context",
		"canonical source of skills",
		"Do not expect a full skills or agents catalog",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
	disabled := config.RenderAgentsMD("Demo", false, nil)
	if !strings.Contains(disabled, "Context Graph is **disabled**") {
		t.Fatalf("disabled graph copy missing:\n%s", disabled)
	}
}

func TestRenderAdapterFiles(t *testing.T) {
	t.Parallel()

	cursor := config.RenderCursorAtlasMDC("Demo")
	if !strings.Contains(cursor, "alwaysApply: true") || !strings.Contains(cursor, "AGENTS.md") {
		t.Fatalf("cursor = %s", cursor)
	}
	opencode := config.RenderOpenCodeAtlas("Demo")
	if !strings.Contains(opencode, ".opencode/atlas.md") || !strings.Contains(opencode, "Demo") {
		t.Fatalf("opencode = %s", opencode)
	}
}
