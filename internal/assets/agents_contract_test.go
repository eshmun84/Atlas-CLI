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
		"## 0. Real OpenSpec provider (required)",
		"## 3. Phases",
		"## 4. Cross-cutting rules",
		"openspec init",
		"governance.openspec_version",
		"opsx-propose",
		"AGENTS.md",
		"Registries/manifest",
		"workflow=sdd",
		"spec_engine=openspec",
		"Fail-closed",
		"BLOCKED",
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
		if !strings.Contains(body, "sdd-openspec.md") {
			t.Fatalf("%s missing SDD/OpenSpec contract reference", name)
		}
	}
}

func TestBaseAgentsTeachThinRouter(t *testing.T) {
	t.Parallel()
	data, err := assets.Content.ReadFile("agents/base.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{
		"## 1. Authority",
		"## 2. Task classification",
		"governed-sdd",
		"interaction-required",
		"## 4. Governance resolution",
		"governance.workflow",
		"governance.spec_engine",
		"## 5. Delivery authority",
		"Cursor Origin",
		"## 6. Signed commits",
		"signing_required",
		"--no-gpg-sign",
		"## 10. Documentation identity boundary",
		"## 11. Memory / Context / Code Intelligence entrypoints",
		"load the **memory** skill",
		"atlas context compose",
		"atlas capability plan",
		"NOT IMPLEMENTED",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("base.md missing %q", want)
		}
	}
	for _, banned := range []string{
		"## 10. Project Memory",
		"Future Memory",
		"inline work by default",
	} {
		if strings.Contains(body, banned) {
			t.Fatalf("base.md must not contain obsolete %q", banned)
		}
	}
}
