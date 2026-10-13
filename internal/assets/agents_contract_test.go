package assets_test

import (
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
)

func TestBundledSDDOpenSpecContractEmbedded(t *testing.T) {
	t.Parallel()
	data, err := assets.Content.ReadFile("contracts/sdd-openspec.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		"Operational Contract",
		"## 3. Phases and transitions",
		"### 4.1 Init",
		"### 4.2 Explore",
		"### 4.3 Research",
		"### 4.4 Propose",
		"### 4.5 Update",
		"### 4.6 Implement",
		"### 4.7 Verify",
		"### 4.8 Archive",
		"Authority boundary: OpenSpec vs Atlas vs project docs",
		"Project documentation policy",
		"No silent Git",
		"agent-registry.md",
		"skill-registry.md",
		"AGENTS.md",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("contract missing %q", want)
		}
	}
}

func TestRuntimeAgentsReferenceSDDOpenSpecContract(t *testing.T) {
	t.Parallel()
	for _, name := range assets.AtlasAgentFilenames {
		body, err := assets.ReadRuntimeAgent(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(body, ".atlas/contracts/sdd-openspec.md") {
			t.Fatalf("%s missing SDD/OpenSpec contract reference", name)
		}
	}
}

func TestBaseAgentsTeachProjectMemory(t *testing.T) {
	t.Parallel()
	data, err := assets.Content.ReadFile("agents/base.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		"## 10. Project Memory",
		"atlas memory context",
		"atlas memory search",
		"atlas memory get",
		"atlas memory propose",
		"evidence, not policy",
		"proposals are not Memory observations",
		"Automatic context injection via hooks is **not** guaranteed",
		"Explicit developer intent outranks inference",
		"Do not self-label a candidate `confirmed`",
		"trusted interactive confirmation surface is deferred",
		"## 11. Context Economy and Composition",
		"atlas context compose",
		"**Project Memory**",
		"**Context Composition**",
		"Do not ask the developer to run it manually",
		"quality-governed persistence",
		"atlas doctor",
		"atlas status",
		"do not treat health output as project policy",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("base.md missing %q", want)
		}
	}
	if strings.Contains(body, "Future Memory") {
		t.Fatal("base.md must not still say Future Memory")
	}
}
