package capability_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/capability"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

func testCatalog(t *testing.T) []skills.Metadata {
	t.Helper()
	cat, err := skills.BundledCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func baseEvidence(t *testing.T, task string) capability.TaskEvidence {
	t.Helper()
	return capability.TaskEvidence{
		TaskText:    task,
		EnabledPins: skills.DefaultPins(),
		Catalog:     testCatalog(t),
		AdapterIDs:  []string{"cursor"},
	}
}

func hasCap(plan capability.CapabilityPlan, id, minStrength string) bool {
	for _, c := range plan.Capabilities {
		if c.ID != id {
			continue
		}
		if minStrength == "" {
			return true
		}
		if c.Strength == minStrength || (minStrength == capability.StrengthRelevant && c.Strength == capability.StrengthRequired) {
			return true
		}
		return c.Strength == minStrength
	}
	return false
}

func hasSkill(plan capability.CapabilityPlan, id string) bool {
	for _, s := range plan.SelectedSkills {
		if s.ID == id {
			return true
		}
	}
	return false
}

func hasRetrieval(plan capability.CapabilityPlan, source string) bool {
	for _, a := range plan.RetrievalActions {
		if a.Source == source {
			return true
		}
	}
	return false
}

func hasContext(plan capability.CapabilityPlan, id string) bool {
	for _, c := range plan.ContextNeeds {
		if c.ID == id {
			return true
		}
	}
	return false
}

func TestDiscoverPlan_Matrix(t *testing.T) {
	cat := testCatalog(t)
	pins := skills.DefaultPins()

	tests := []struct {
		name  string
		ev    capability.TaskEvidence
		check func(t *testing.T, plan capability.CapabilityPlan)
	}{
		{
			name: "1_documentation_only",
			ev: capability.TaskEvidence{
				TaskText:     "update README title",
				Technologies: []string{"Go", "Go modules"},
				EnabledPins:  pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapDocumentation, capability.StrengthRelevant) {
					t.Fatalf("want documentation: %#v", plan.Capabilities)
				}
				if hasCap(plan, capability.CapSecurityReview, "") {
					t.Fatal("security must not be selected for README")
				}
				if hasRetrieval(plan, capability.RetrievalCodeGraph) {
					t.Fatal("CodeGraph must not run for README")
				}
			},
		},
		{
			name: "2_implementation",
			ev: capability.TaskEvidence{
				TaskText:    "implement user profile endpoint",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapImplementation, "") {
					t.Fatal("want implementation")
				}
				if !hasCap(plan, capability.CapTesting, "") {
					t.Fatal("want testing relevant")
				}
			},
		},
		{
			name: "3_test_debug",
			ev: capability.TaskEvidence{
				TaskText:     "debug failing Go test for race condition",
				Technologies: []string{"Go"},
				EnabledPins:  pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapTesting, "") || !hasCap(plan, capability.CapDebugging, "") {
					t.Fatalf("want testing+debugging: %#v", plan.Capabilities)
				}
			},
		},
		{
			name: "4_security_relevant",
			ev: capability.TaskEvidence{
				TaskText:    "review this authentication change",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapSecurityReview, capability.StrengthRelevant) {
					t.Fatal("want security-review relevant")
				}
				for _, c := range plan.Capabilities {
					if c.ID == capability.CapSecurityReview && c.Strength == capability.StrengthRequired {
						t.Fatal("security must not be required from keywords alone")
					}
				}
			},
		},
		{
			name: "5_architecture_readonly",
			ev: capability.TaskEvidence{
				TaskText:       "explain existing architecture",
				Classification: "direct",
				EnabledPins:    pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapArchitecture, "") {
					t.Fatal("want architecture")
				}
				if hasCap(plan, capability.CapImplementation, "") {
					t.Fatal("implementation must not be required for explain architecture")
				}
				if !hasRetrieval(plan, capability.RetrievalMemory) {
					t.Fatal("architecture should trigger Memory")
				}
			},
		},
		{
			name: "6_continue_memory",
			ev: capability.TaskEvidence{
				TaskText:    "continue previous implementation of auth module",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasRetrieval(plan, capability.RetrievalMemory) {
					t.Fatal("continue should trigger Memory retrieval")
				}
			},
		},
		{
			name: "7_trivial_no_memory",
			ev: capability.TaskEvidence{
				TaskText:    "atlas --version",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if hasRetrieval(plan, capability.RetrievalMemory) {
					t.Fatal("version must not retrieve Memory")
				}
			},
		},
		{
			name: "8_structural_codegraph",
			ev: capability.TaskEvidence{
				TaskText:    "who calls this function and what will this change impact?",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapCodeIntelligence, "") {
					t.Fatal("want code-intelligence")
				}
				if !hasRetrieval(plan, capability.RetrievalCodeGraph) {
					t.Fatal("want CodeGraph retrieval action")
				}
			},
		},
		{
			name: "9_simple_edit_no_codegraph",
			ev: capability.TaskEvidence{
				TaskText:    "change configuration label in settings.yaml",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if hasRetrieval(plan, capability.RetrievalCodeGraph) {
					t.Fatal("simple label edit must not trigger CodeGraph")
				}
			},
		},
		{
			name: "10_unknown_capability_continue",
			ev: capability.TaskEvidence{
				TaskText:    "unrelated chocolate cake recipe",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if plan.Status != capability.StatusSuccess {
					t.Fatalf("status=%s", plan.Status)
				}
				if len(plan.SelectedSkills) != 0 {
					t.Fatalf("expected no specialized skills, got %#v", plan.SelectedSkills)
				}
			},
		},
		{
			name: "11_explicit_required_skill",
			ev: capability.TaskEvidence{
				TaskText:    "use the security-review skill on this diff",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapSecurityReview, capability.StrengthRequired) {
					t.Fatal("explicit use must require security-review")
				}
				if !hasSkill(plan, "security-review") {
					t.Fatal("security-review skill must be selected")
				}
			},
		},
		{
			name: "12_explicit_forbid",
			ev: capability.TaskEvidence{
				TaskText:    "implement feature but do not use codegraph",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if hasRetrieval(plan, capability.RetrievalCodeGraph) {
					t.Fatal("excluded CodeGraph must not appear")
				}
				if hasCap(plan, capability.CapCodeIntelligence, "") {
					t.Fatal("code-intelligence capability must be excluded")
				}
			},
		},
		{
			name: "13_no_memory_exclusion",
			ev: capability.TaskEvidence{
				TaskText:    "continue previous work but don't retrieve Memory",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if hasRetrieval(plan, capability.RetrievalMemory) {
					t.Fatal("Memory retrieval must be excluded")
				}
			},
		},
		{
			name: "14_sdd_propose",
			ev: capability.TaskEvidence{
				TaskText:    "draft change proposal for billing",
				Phase:       "propose",
				Workflow:    "sdd",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapSpecAuthoring, "") || !hasCap(plan, capability.CapResearch, "") {
					t.Fatalf("propose phase: %#v", plan.Capabilities)
				}
				if !hasContext(plan, capability.ContextOpenSpec) {
					t.Fatal("want openspec context need")
				}
			},
		},
		{
			name: "15_sdd_implement",
			ev: capability.TaskEvidence{
				TaskText:     "implement approved change",
				Phase:        "implement",
				Workflow:     "sdd",
				ActiveChange: "add-billing",
				EnabledPins:  pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapImplementation, "") || !hasCap(plan, capability.CapTesting, "") {
					t.Fatalf("implement: %#v", plan.Capabilities)
				}
			},
		},
		{
			name: "16_sdd_verify",
			ev: capability.TaskEvidence{
				TaskText:    "verify the change meets acceptance",
				Phase:       "verify",
				Workflow:    "sdd",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapTesting, "") || !hasCap(plan, capability.CapCodeReview, "") {
					t.Fatalf("verify: %#v", plan.Capabilities)
				}
			},
		},
		{
			name: "17_tech_does_not_dominate",
			ev: capability.TaskEvidence{
				TaskText:     "update Go README",
				Technologies: []string{"Go", "Go modules"},
				EnabledPins:  pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if !hasCap(plan, capability.CapDocumentation, "") {
					t.Fatal("want documentation")
				}
				if hasCap(plan, capability.CapSecurityReview, "") {
					t.Fatal("Go evidence must not force security")
				}
				if hasCap(plan, capability.CapTesting, "") {
					t.Fatal("Go evidence must not force testing for README")
				}
			},
		},
		{
			name: "19_optional_unavailable_continue",
			ev: capability.TaskEvidence{
				TaskText:         "database-migration for invoices",
				ExplicitRequires: []string{},
				EnabledPins:      []skills.Pin{{ID: "documentation", Version: "1.0.0"}},
				Catalog:          cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if plan.Status == capability.StatusBlocked {
					t.Fatal("optional gaps must not block")
				}
			},
		},
		{
			name: "20_required_unavailable_blocked",
			ev: capability.TaskEvidence{
				TaskText:         "use the security-review skill",
				ExplicitRequires: []string{capability.CapSecurityReview},
				EnabledPins:      []skills.Pin{{ID: "documentation", Version: "1.0.0"}},
				Catalog:          cat,
				AdapterIDs:       []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if plan.Status != capability.StatusBlocked {
					t.Fatalf("want blocked, got %s blockers=%v", plan.Status, plan.Blockers)
				}
				if len(plan.Blockers) == 0 {
					t.Fatal("want blockers")
				}
			},
		},
		{
			name: "25_projected_ne_selected",
			ev: capability.TaskEvidence{
				TaskText:    "update README",
				EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
			},
			check: func(t *testing.T, plan capability.CapabilityPlan) {
				if len(plan.ProjectedPins) != len(pins) {
					t.Fatalf("projected pins=%d want %d", len(plan.ProjectedPins), len(pins))
				}
				if len(plan.SelectedSkills) >= len(plan.ProjectedPins) && len(plan.ProjectedPins) > 1 {
					// README should select documentation subset, not all pins.
					if len(plan.SelectedSkills) == len(pins) {
						t.Fatal("selected must not equal all projected pins for docs task")
					}
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan := capability.DiscoverPlan(tc.ev)
			if plan.SchemaVersion != capability.SchemaVersion {
				t.Fatalf("schema=%d", plan.SchemaVersion)
			}
			tc.check(t, plan)
		})
	}
}

func TestDiscoverPlan_DeterministicRepeated(t *testing.T) {
	ev := baseEvidence(t, "implement authentication middleware with tests")
	ev.Technologies = []string{"Go"}
	a := capability.DiscoverPlan(ev)
	b := capability.DiscoverPlan(ev)
	aj, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	bj, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("plans differ\nA=%s\nB=%s", aj, bj)
	}
}

func TestDiscoverPlan_OnlyInspectExcludesImplementation(t *testing.T) {
	ev := baseEvidence(t, "only inspect the auth package, do not implement")
	plan := capability.DiscoverPlan(ev)
	if hasCap(plan, capability.CapImplementation, "") {
		t.Fatalf("implementation excluded: %#v", plan.Capabilities)
	}
}

func TestParseExplicitConstraints(t *testing.T) {
	parsed := capability.ParseExplicitConstraints("use the testing skill and do not retrieve Memory")
	if !contains(parsed.Requires, capability.CapTesting) && !contains(parsed.SkillRequires, "testing") {
		t.Fatalf("requires=%v skills=%v", parsed.Requires, parsed.SkillRequires)
	}
	if !contains(parsed.SkillRequires, "testing") {
		t.Fatalf("skill requires=%v", parsed.SkillRequires)
	}
	if !contains(parsed.Excludes, capability.CapMemoryRetrieval) && !contains(parsed.Excludes, "memory") {
		t.Fatalf("excludes=%v", parsed.Excludes)
	}
}

func TestDiscoverPlan_ExplicitSkillSemantics(t *testing.T) {
	cat := testCatalog(t)
	pins := skills.DefaultPins()

	t.Run("use_testing_pinned_success", func(t *testing.T) {
		plan := capability.DiscoverPlan(capability.TaskEvidence{
			TaskText:    "use the testing skill on this change",
			EnabledPins: pins,
			Catalog:     cat,
			AdapterIDs:  []string{"cursor"},
		})
		if plan.Status != capability.StatusSuccess {
			t.Fatalf("status=%s blockers=%v", plan.Status, plan.Blockers)
		}
		if !hasSkill(plan, "testing") {
			t.Fatal("testing skill must be selected")
		}
	})

	t.Run("use_security_not_pinned_blocked", func(t *testing.T) {
		plan := capability.DiscoverPlan(capability.TaskEvidence{
			TaskText:    "use the security-review skill",
			EnabledPins: []skills.Pin{{ID: "documentation", Version: "1.0.0"}},
			Catalog:     cat,
			AdapterIDs:  []string{"cursor"},
		})
		if plan.Status != capability.StatusBlocked {
			t.Fatalf("want blocked, got %s", plan.Status)
		}
		if len(plan.Blockers) == 0 {
			t.Fatal("want blockers")
		}
	})

	t.Run("use_unknown_skill_blocked", func(t *testing.T) {
		plan := capability.DiscoverPlan(capability.TaskEvidence{
			TaskText:    "use database-migration skill",
			EnabledPins: pins,
			Catalog:     cat,
			AdapterIDs:  []string{"cursor"},
		})
		if plan.Status != capability.StatusBlocked {
			t.Fatalf("want blocked for unknown skill, got %s caps=%#v", plan.Status, plan.Capabilities)
		}
		found := false
		for _, b := range plan.Blockers {
			if strings.Contains(b, "database-migration") {
				found = true
			}
		}
		if !found {
			t.Fatalf("blocker must name database-migration: %v", plan.Blockers)
		}
		// Must not expand capability vocabulary with unknown ID.
		if hasCap(plan, "database-migration", "") {
			t.Fatal("unknown skill must not become a capability vocabulary entry")
		}
	})

	t.Run("core_implementation_no_skill_blocker", func(t *testing.T) {
		plan := capability.DiscoverPlan(capability.TaskEvidence{
			TaskText:         "work on feature",
			ExplicitRequires: []string{capability.CapImplementation},
			EnabledPins:      pins,
			Catalog:          cat,
			AdapterIDs:       []string{"cursor"},
		})
		if plan.Status == capability.StatusBlocked {
			t.Fatalf("core capability must not block solely for missing skill: %v", plan.Blockers)
		}
		if !hasCap(plan, capability.CapImplementation, capability.StrengthRequired) {
			t.Fatal("want required implementation")
		}
	})

	t.Run("exclusion_outranks_inference", func(t *testing.T) {
		plan := capability.DiscoverPlan(capability.TaskEvidence{
			TaskText:    "continue previous architecture work but don't retrieve Memory",
			EnabledPins: pins, Catalog: cat, AdapterIDs: []string{"cursor"},
		})
		if hasRetrieval(plan, capability.RetrievalMemory) {
			t.Fatal("exclusion must outrank Memory inference")
		}
	})
}

func TestComposeSourcesFromPlan(t *testing.T) {
	plan := capability.CapabilityPlan{
		RetrievalActions: []capability.RetrievalAction{
			{Source: capability.RetrievalMemory},
			{Source: capability.RetrievalCodeGraph},
		},
	}
	sources := capability.ComposeSourcesFromPlan(plan)
	if len(sources) != 2 {
		t.Fatalf("sources=%v", sources)
	}
}

func TestDiscoverPlan_ProjectIsolationFields(t *testing.T) {
	// Plans must not embed foreign project absolute Memory paths.
	ev := baseEvidence(t, "continue prior decision about auth")
	plan := capability.DiscoverPlan(ev)
	raw, _ := json.Marshal(plan)
	if strings.Contains(string(raw), "/projects/") && strings.Contains(string(raw), "memory.db") {
		t.Fatalf("plan must not leak Memory DB paths: %s", raw)
	}
	if hasRetrieval(plan, capability.RetrievalMemory) {
		for _, a := range plan.RetrievalActions {
			if a.Source == capability.RetrievalMemory && strings.Contains(a.Query, string(filepath.Separator)+"projects"+string(filepath.Separator)) {
				t.Fatal("retrieval query must stay task-scoped, not path-scoped to another project")
			}
		}
	}
}

func TestGatherEvidence_ReadOnlyCheap(t *testing.T) {
	root := t.TempDir()
	// Marker file for Go evidence without full inspect.
	if err := writeFile(t, filepath.Join(root, "go.mod"), "module example\n"); err != nil {
		t.Fatal(err)
	}
	res, err := capability.GatherEvidence(capability.GatherInput{
		Root: root,
		Home: t.TempDir(),
		Task: "update README",
	})
	if err != nil {
		t.Fatal(err)
	}
	foundGo := false
	for _, tech := range res.Evidence.Technologies {
		if strings.EqualFold(tech, "Go") || strings.Contains(strings.ToLower(tech), "go") {
			foundGo = true
		}
	}
	if !foundGo {
		t.Fatalf("want Go technology evidence, got %v", res.Evidence.Technologies)
	}
	plan := capability.DiscoverPlan(res.Evidence)
	if hasCap(plan, capability.CapSecurityReview, "") {
		t.Fatal("tech must not dominate")
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, path, body string) error {
	t.Helper()
	return os.WriteFile(path, []byte(body), 0o644)
}
