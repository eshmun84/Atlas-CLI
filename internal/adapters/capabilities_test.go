package adapters_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
)

func TestCursorSupportsSkills(t *testing.T) {
	if !adapters.Supports(adapters.Cursor, adapters.ClassSkills) {
		t.Fatal("cursor must support skills")
	}
	caps, ok := adapters.For(adapters.Cursor)
	if !ok || !caps.Skills || caps.SkillsRootRel == "" {
		t.Fatalf("cursor skills caps incomplete: %+v ok=%v", caps, ok)
	}
}

func TestOpenCodeSupportsSkills(t *testing.T) {
	if !adapters.Supports(adapters.OpenCode, adapters.ClassSkills) {
		t.Fatal("opencode must support skills")
	}
	caps, ok := adapters.For(adapters.OpenCode)
	if !ok || !caps.Skills || caps.SkillsRootRel == "" {
		t.Fatalf("opencode skills caps incomplete: %+v ok=%v", caps, ok)
	}
}

func TestCapabilityLookupNoProviderBranching(t *testing.T) {
	// Core-style lookup: iterate known IDs and query by class only.
	var withSkills []adapters.ID
	for _, id := range adapters.KnownIDs() {
		if adapters.Supports(id, adapters.ClassSkills) {
			withSkills = append(withSkills, id)
		}
	}
	if len(withSkills) != 2 {
		t.Fatalf("expected cursor+opencode skills, got %v", withSkills)
	}
	if adapters.Supports(adapters.ID("claude"), adapters.ClassSkills) {
		t.Fatal("unknown adapter must not support skills")
	}
	if adapters.Supports(adapters.Cursor, adapters.ClassHooks) {
		t.Fatal("cursor hooks not implemented in this slice")
	}
	if !adapters.Supports(adapters.Cursor, adapters.ClassAgents) || !adapters.Supports(adapters.Cursor, adapters.ClassMCP) {
		t.Fatal("cursor must advertise agents and mcp")
	}
}
