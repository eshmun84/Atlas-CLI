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
