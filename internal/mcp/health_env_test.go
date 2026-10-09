package mcp_test

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
)

func TestReferencedEnvNames_EnvAndHeaderDeduped(t *testing.T) {
	t.Parallel()
	def := mcp.Definition{
		ID:      "custom",
		EnvRefs: []string{"MY_TOKEN", "OTHER"},
		HeaderRefs: map[string]mcp.HeaderValueRef{
			"Authorization": {Env: "MY_TOKEN", Prefix: "Bearer "},
			"X-Extra":       {Env: "EXTRA_ENV"},
		},
	}
	names := mcp.ReferencedEnvNames(def)
	if len(names) != 3 {
		t.Fatalf("names=%v want 3 unique", names)
	}
	joined := strings.Join(names, ",")
	for _, want := range []string{"MY_TOKEN", "OTHER", "EXTRA_ENV"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %v", want, names)
		}
	}
}

func TestHealth_HeaderEnvUnsetAndSet(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	def := mcp.Definition{
		ID:              "custom-auth",
		DisplayName:     "Custom Auth",
		Source:          mcp.SourceCustom,
		AuthRequirement: mcp.AuthEnvironmentReference,
		HeaderRefs: map[string]mcp.HeaderValueRef{
			"Authorization": {Env: "MY_TOKEN", Prefix: "Bearer "},
		},
		Materializable: true,
		Enabled:        true,
		Transport:      mcp.TransportStreamableHTTP,
		Endpoint:       "https://example.com/mcp",
	}
	t.Setenv("MY_TOKEN", "")
	h := mcp.EvaluateHealth(root, homePath, "p", mcp.DesiredState{Definitions: []mcp.Definition{def}},
		[]mcp.AdapterID{mcp.AdapterCursor},
		map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()})
	if !warningsContain(h, "MY_TOKEN") {
		t.Fatalf("expected MY_TOKEN warning, got %#v", h.Adapters)
	}

	t.Setenv("MY_TOKEN", "secret-value")
	h = mcp.EvaluateHealth(root, homePath, "p", mcp.DesiredState{Definitions: []mcp.Definition{def}},
		[]mcp.AdapterID{mcp.AdapterCursor},
		map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()})
	if warningsContain(h, "MY_TOKEN") {
		t.Fatalf("MY_TOKEN set must not warn: %#v", h.Adapters)
	}
}

func TestContext7_OptionalEnvWarning(t *testing.T) {
	root := t.TempDir()
	homePath := t.TempDir()
	def, ok := mcp.BuiltinByID(mcp.BuiltinContext7)
	if !ok {
		t.Fatal("context7 missing")
	}
	def.Enabled = true
	t.Setenv("CONTEXT7_API_KEY", "")
	h := mcp.EvaluateHealth(root, homePath, "p", mcp.DesiredState{Definitions: []mcp.Definition{def}},
		[]mcp.AdapterID{mcp.AdapterCursor},
		map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()})
	if !warningsContain(h, "CONTEXT7_API_KEY") || !warningsContain(h, "optional") {
		t.Fatalf("context7 optional wording missing: %#v", h.Adapters)
	}
	t.Setenv("CONTEXT7_API_KEY", "k")
	h = mcp.EvaluateHealth(root, homePath, "p", mcp.DesiredState{Definitions: []mcp.Definition{def}},
		[]mcp.AdapterID{mcp.AdapterCursor},
		map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()})
	if warningsContain(h, "CONTEXT7_API_KEY") {
		t.Fatalf("set key must not warn: %#v", h.Adapters)
	}
}

func warningsContain(h mcp.Health, needle string) bool {
	for _, a := range h.Adapters {
		for _, w := range a.Warnings {
			if strings.Contains(w, needle) {
				return true
			}
		}
	}
	for _, w := range h.DefinitionErr {
		if strings.Contains(w, needle) {
			return true
		}
	}
	return false
}
