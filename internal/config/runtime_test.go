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
	wantCount := 1 + 2 + len(config.AtlasAgentRuntimePaths([]string{"cursor", "opencode"}))
	if len(got) != wantCount {
		t.Fatalf("with adapters len=%d want=%d got=%#v", len(got), wantCount, got)
	}

	// Non-canonical casing must not materialize adapter projections.
	doc.Adapters.Selected = []string{"Cursor", "OpenCode"}
	got = config.RuntimeTargets(doc)
	if len(got) != 1 || got[0] != config.FileAgentsMD {
		t.Fatalf("non-canonical adapters must be ignored by targets: %#v", got)
	}
}

func TestRenderAgentsMD_BaseOnly(t *testing.T) {
	t.Parallel()

	got := config.RenderAgentsMD("Demo", true, nil, nil)
	assertContains(t, got,
		"# Atlas Project Runtime Contract",
		config.AgentsBaseBegin,
		config.AgentsBaseEnd,
		config.AgentsUserBegin,
		"Project-specific instructions go here.",
		config.AgentsUserEnd,
		"## 1. Purpose and Authority",
		"## 2. Professional Engineering Behavior",
		"## 3. Scope Control",
		"## 4. Planning and Execution Discipline",
		"## 5. Git and Delivery Authorization",
		"## 6. Remote and External Operations",
		"## 7. Skill and Contract Loading",
		"## 8. Agent and Subagent Orchestration",
		"## 9. Review and Verification",
		"## 10. Context Economy",
		"Demo",
		"Context Graph is **enabled**",
		"Do not add `Co-Authored-By`",
		"Do not invent missing project facts",
		"explicit human request",
	)
	assertNotContains(t, got,
		config.AdapterBlockBegin("cursor"),
		config.AdapterBlockBegin("opencode"),
		"<!-- ATLAS:MANAGED:BEGIN -->",
		"## Cursor Adapter Guidance",
		"## OpenCode Adapter Guidance",
	)
}

func TestRenderAgentsMD_CursorOnly(t *testing.T) {
	t.Parallel()

	got := config.RenderAgentsMD("Demo", true, []string{"cursor"}, nil)
	assertContains(t, got,
		config.AgentsBaseBegin,
		config.AdapterBlockBegin("cursor"),
		config.AdapterBlockEnd("cursor"),
		"## Cursor Adapter Guidance",
		"Cursor must treat root `AGENTS.md` as the project authority",
	)
	assertNotContains(t, got,
		config.AdapterBlockBegin("opencode"),
		"## OpenCode Adapter Guidance",
	)
}

func TestRenderAgentsMD_OpenCodeOnly(t *testing.T) {
	t.Parallel()

	got := config.RenderAgentsMD("Demo", false, []string{"opencode"}, nil)
	assertContains(t, got,
		config.AgentsBaseBegin,
		config.AdapterBlockBegin("opencode"),
		config.AdapterBlockEnd("opencode"),
		"## OpenCode Adapter Guidance",
		"Context Graph is **disabled**",
		"Delegated work produces evidence",
	)
	assertNotContains(t, got,
		config.AdapterBlockBegin("cursor"),
		"## Cursor Adapter Guidance",
	)
}

func TestRenderAgentsMD_CursorAndOpenCode(t *testing.T) {
	t.Parallel()

	got := config.RenderAgentsMD("Demo", true, []string{"opencode", "cursor"}, nil)
	cursorAt := strings.Index(got, config.AdapterBlockBegin("cursor"))
	openAt := strings.Index(got, config.AdapterBlockBegin("opencode"))
	if cursorAt < 0 || openAt < 0 || cursorAt > openAt {
		t.Fatalf("expected cursor block before opencode:\ncursor=%d opencode=%d\n%s", cursorAt, openAt, got)
	}
	assertNotContains(t, got,
		config.AdapterBlockBegin("codex"),
		config.AdapterBlockBegin("claude"),
	)
}

func TestRenderAgentsMD_PreservesUserExactly(t *testing.T) {
	t.Parallel()

	userBody := "\n  Keep leading indent\n\n- first item\n- second item\n\nTrailing blank line kept:\n  \n"
	existing := config.AgentsUserBegin + userBody + config.AgentsUserEnd
	got := config.RenderAgentsMD("Demo", true, []string{"cursor"}, []byte(existing))

	begin := strings.Index(got, config.AgentsUserBegin)
	end := strings.Index(got, config.AgentsUserEnd)
	if begin < 0 || end < 0 || end < begin {
		t.Fatalf("USER markers missing:\n%s", got)
	}
	gotBody := got[begin+len(config.AgentsUserBegin) : end]
	if gotBody != userBody {
		t.Fatalf("USER body not preserved exactly\nwant %q\ngot  %q", userBody, gotBody)
	}

	again := config.RenderAgentsMD("Demo", true, []string{"cursor"}, []byte(got))
	begin = strings.Index(again, config.AgentsUserBegin)
	end = strings.Index(again, config.AgentsUserEnd)
	if begin < 0 || end < 0 || end < begin {
		t.Fatalf("USER markers missing on regenerate:\n%s", again)
	}
	againBody := again[begin+len(config.AgentsUserBegin) : end]
	if againBody != userBody {
		t.Fatalf("USER body lost formatting on regenerate\nwant %q\ngot  %q", userBody, againBody)
	}
	markers := config.InspectAgentsMarkers([]byte(again))
	if !markers.ContractSatisfied([]string{"cursor"}) {
		t.Fatalf("markers = %#v", markers)
	}
}

func TestRenderAgentsMD_IgnoresUnselectedAndUnsupported(t *testing.T) {
	t.Parallel()

	got := config.RenderAgentsMD("Demo", true, []string{"cursor", "claude", "Codex", "OpenCode"}, nil)
	assertContains(t, got, config.AdapterBlockBegin("cursor"))
	assertNotContains(t, got,
		config.AdapterBlockBegin("opencode"),
		config.AdapterBlockBegin("claude"),
		config.AdapterBlockBegin("codex"),
	)
}

func TestRenderAdapterProjections(t *testing.T) {
	t.Parallel()

	cursor := config.RenderCursorAtlasMDC("Demo")
	assertContains(t, cursor,
		"alwaysApply: true",
		"Atlas Cursor Entrypoint",
		"Root `AGENTS.md` is the project authority; do not bypass it.",
		".cursor/agents/atlas-orchestrator.md",
		".atlas/agent-registry.md",
		"Cursor-native entrypoint only",
		"Demo",
	)
	assertNotContains(t, cursor,
		"## 1. Purpose and Authority",
		"Conventional Commits",
		"## Cursor Adapter Guidance",
	)

	opencode := config.RenderOpenCodeAtlas("Demo")
	assertContains(t, opencode,
		"Atlas OpenCode Entrypoint",
		".opencode/atlas.md",
		"Root `AGENTS.md` is the project authority; do not bypass it.",
		".opencode/agents/atlas-orchestrator.md",
		".atlas/agent-registry.md",
		"execution surfaces, not independent authorities",
		"Demo",
	)
	assertNotContains(t, opencode,
		"## 1. Purpose and Authority",
		"Conventional Commits",
		"## OpenCode Adapter Guidance",
	)
}

func TestInspectAgentsMarkers_V2(t *testing.T) {
	t.Parallel()

	complete := config.InspectAgentsMarkers([]byte(config.RenderAgentsMD("demo", true, []string{"cursor"}, nil)))
	if !complete.Complete() || !complete.HasAdapter("cursor") || complete.HasAdapter("opencode") {
		t.Fatalf("complete markers = %#v", complete)
	}
	if !complete.ContractSatisfied([]string{"cursor"}) {
		t.Fatal("contract should be satisfied")
	}
	if extras := complete.UnselectedAdapters([]string{"cursor"}); len(extras) != 0 {
		t.Fatalf("extras = %#v", extras)
	}

	partial := config.InspectAgentsMarkers([]byte("<!-- ATLAS:BASE:BEGIN -->\n"))
	if partial.Complete() || !partial.BaseBegin || partial.UserEnd {
		t.Fatalf("partial markers = %#v", partial)
	}

	legacy := config.InspectAgentsMarkers([]byte("<!-- ATLAS:MANAGED:BEGIN -->\n<!-- ATLAS:MANAGED:END -->\n<!-- ATLAS:USER:BEGIN -->\nx\n<!-- ATLAS:USER:END -->\n"))
	if legacy.Complete() || legacy.BaseBegin {
		t.Fatalf("legacy managed markers must not satisfy v2 Complete: %#v", legacy)
	}
}

func assertContains(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
}

func assertNotContains(t *testing.T, got string, banned ...string) {
	t.Helper()
	for _, item := range banned {
		if strings.Contains(got, item) {
			t.Fatalf("unexpected %q:\n%s", item, got)
		}
	}
}
