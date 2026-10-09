package config_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/config"
)

var (
	// camelGlue catches mid-sentence glued words like "projectDocs" / "hardenedLater".
	camelGlueRE = regexp.MustCompile(`\s([a-z]{3,}[A-Z][a-z]{2,})\b`)
	// sentenceGlueRE catches "end.Start" without a space (excluding dotted paths).
	sentenceGlueRE = regexp.MustCompile(`[a-zA-Z]\.[A-Z][a-z]`)
	headingGlueRE  = regexp.MustCompile(`[^\n#]#{1,6}[A-Za-z]`)
	bulletGlueRE   = regexp.MustCompile(`[A-Za-z]\n[-*][A-Za-z]`)
)

var bannedTokens = []string{
	"projectdocs",
	"projectocs",
	"hardenedlater",
	"createonce",
	"configyamlonly",
	"notimplemented",
	"contextgraphis",
	"preferenceonly",
	"docs/atlasreadme",
}

func TestRenderAgentsMD_TextQuality(t *testing.T) {
	t.Parallel()

	got := mustRenderAgentsMD(t, "DemoProject", true, []string{"cursor", "opencode"}, nil)
	assertTextQuality(t, "AGENTS.md", got)

	if strings.Count(got, "# Atlas Project Runtime Contract") != 1 {
		t.Fatalf("duplicate or missing H1")
	}
	if strings.Contains(got, "<!-- ATLAS:BASE:BEGIN -->\n# Atlas Project Runtime Contract") {
		t.Fatal("BASE body must not repeat the document H1")
	}
	for _, want := range []string{
		config.AgentsBaseBegin,
		config.AdapterBlockBegin("cursor"),
		config.AdapterBlockBegin("opencode"),
		config.AgentsUserBegin,
		"Context Economy v0",
		"CodeGraph",
		"Atlas Context Graph",
		"NOT IMPLEMENTED",
		"optional externally installed Code Intelligence provider",
		".atlas/contracts/sdd-openspec.md",
		"Do not invent CodeGraph results",
		"Do not invent graph context",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q", want)
		}
	}
	// Unselected adapters stay out.
	assertNotContains(t, got,
		config.AdapterBlockBegin("claude"),
		config.AdapterBlockBegin("codex"),
	)
}

func TestRenderAgentsMD_PreservesUserAndAdapterSelection(t *testing.T) {
	t.Parallel()

	user := "\nKeep my notes.\n"
	existing := config.AgentsUserBegin + user + config.AgentsUserEnd
	got := mustRenderAgentsMD(t, "Demo", true, []string{"cursor"}, []byte(existing))
	begin := strings.Index(got, config.AgentsUserBegin)
	end := strings.Index(got, config.AgentsUserEnd)
	if begin < 0 || end < 0 {
		t.Fatal("missing USER markers")
	}
	body := got[begin+len(config.AgentsUserBegin) : end]
	if body != user {
		t.Fatalf("USER body mutated:\n%q\nwant:\n%q", body, user)
	}
	assertContains(t, got, config.AdapterBlockBegin("cursor"), "## Cursor Adapter Guidance")
	assertNotContains(t, got, config.AdapterBlockBegin("opencode"))
	assertTextQuality(t, "AGENTS.md+user", got)
}

func TestAdapterProjections_TextQuality(t *testing.T) {
	t.Parallel()

	cursor := mustRenderCursorAtlasMDC(t, "Demo")
	assertTextQuality(t, "cursor atlas.mdc", cursor)
	assertContains(t, cursor,
		"AGENTS.md",
		".atlas/contracts/sdd-openspec.md",
		"Context Economy",
		".atlas/config.yaml",
	)

	opencode := mustRenderOpenCodeAtlas(t, "Demo")
	assertTextQuality(t, "opencode atlas.md", opencode)
	assertContains(t, opencode,
		"AGENTS.md",
		".atlas/contracts/sdd-openspec.md",
		"Context Economy",
	)
}

func TestSDDOpenSpecContract_TextQuality(t *testing.T) {
	t.Parallel()

	got, err := config.RenderSDDOpenSpecContract()
	if err != nil {
		t.Fatal(err)
	}
	assertTextQuality(t, "sdd-openspec", got)
	assertContains(t, got,
		"does **not** execute OpenSpec CLI commands",
		"AGENTS.md",
		"No silent Git",
	)
}

func TestBundledAgentAssets_TextQuality(t *testing.T) {
	t.Parallel()

	entries, err := assets.Content.ReadDir("agents/runtime")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		raw, err := assets.Content.ReadFile("agents/runtime/" + entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		assertTextQuality(t, entry.Name(), string(raw))
		text := string(raw)
		if !strings.Contains(text, "AGENTS.md") {
			t.Fatalf("%s missing AGENTS.md authority reference", entry.Name())
		}
		if strings.Contains(strings.ToLower(text), "execute openspec") &&
			!strings.Contains(text, "Do not execute real OpenSpec") &&
			!strings.Contains(text, "do not execute real OpenSpec") {
			// Allow explicit prohibition wording only.
			if !strings.Contains(text, "Do not execute") && !strings.Contains(text, "does not execute") {
				t.Fatalf("%s may imply OpenSpec execution", entry.Name())
			}
		}
	}
}

func TestAgentRegistry_TextQuality(t *testing.T) {
	t.Parallel()

	got := config.RenderAgentRegistry("Demo", []string{"cursor"}, "")
	assertTextQuality(t, "agent-registry", got)
	assertContains(t, got,
		"# Atlas Agent Registry",
		config.FileSDDOpenSpecContract,
		"Do not execute real OpenSpec CLI commands",
		"AGENTS.md",
	)
}

func TestProjectDocsScaffold_TextQuality(t *testing.T) {
	t.Parallel()

	assertTextQuality(t, "docs scaffold", config.ProjectDocsScaffoldContent)
	assertContains(t, config.ProjectDocsScaffoldContent,
		"project-owned",
		"Runtime Repair",
		"docs/atlas/README.md",
	)
	assertNotContains(t, config.ProjectDocsScaffoldContent,
		"framework documentation into this project",
	)
}

func assertTextQuality(t *testing.T, label, text string) {
	t.Helper()
	lower := strings.ToLower(text)
	for _, tok := range bannedTokens {
		if strings.Contains(lower, tok) {
			t.Fatalf("%s contains banned token %q", label, tok)
		}
	}
	if m := camelGlueRE.FindStringSubmatch(text); len(m) > 1 {
		// Allow YAML/front-matter keys like alwaysApply in cursor projection.
		if m[1] != "alwaysApply" {
			t.Fatalf("%s camel-glue defect %q", label, m[1])
		}
	}
	for _, m := range sentenceGlueRE.FindAllStringIndex(text, -1) {
		ctx := text[max(0, m[0]-24):min(len(text), m[1]+24)]
		if strings.Contains(ctx, ".atlas") || strings.Contains(ctx, ".cursor") ||
			strings.Contains(ctx, ".opencode") || strings.Contains(ctx, ".md") ||
			strings.Contains(ctx, ".yaml") || strings.Contains(ctx, "http") ||
			strings.Contains(ctx, "v0.") {
			continue
		}
		t.Fatalf("%s sentence-glue defect near %q", label, strings.ReplaceAll(ctx, "\n", "|"))
	}
	if headingGlueRE.FindString(text) != "" {
		t.Fatalf("%s heading glued to previous text", label)
	}
	if bulletGlueRE.FindString(text) != "" {
		t.Fatalf("%s bullet glued to previous text", label)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
