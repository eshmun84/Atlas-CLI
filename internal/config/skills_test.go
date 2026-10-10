package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

func TestApplyConfig_ProjectsSkillsForSelectedAdapters(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      "skills-demo",
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: true,
	})
	if !draft.SetValue("adapters.selected", "cursor,opencode") {
		t.Fatal("adapters.selected")
	}
	fixed := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft(), Now: func() time.Time { return fixed }}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{
		".atlas/skill-registry.md",
		".cursor/skills/testing/SKILL.md",
		".opencode/skills/testing/SKILL.md",
		".cursor/skills/code-review/SKILL.md",
		".opencode/skills/documentation/SKILL.md",
	} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	reg, err := os.ReadFile(filepath.Join(root, ".atlas", "skill-registry.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reg), "testing") || strings.Contains(string(reg), "http://") {
		t.Fatalf("unexpected skill registry:\n%s", reg)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	h := config.InspectSkillHealth(root, doc)
	if !h.CatalogReadable || !h.Ready() {
		t.Fatalf("skills health not ready: %+v", h)
	}
	if !adapters.Supports(adapters.Cursor, adapters.ClassSkills) {
		t.Fatal("cursor skills capability required")
	}
}

func TestSkillPins_ExactOnlyInConfig(t *testing.T) {
	doc := config.ProjectDocument{
		Project:    config.ProjectPersist{Name: "x", Mode: "new"},
		Governance: config.GovernancePersist{Workflow: "sdd", SpecEngine: "openspec"},
		Adapters:   config.AdaptersPersist{Selected: []string{"cursor"}},
		SourceControl: config.SourceControlPersist{
			Mode: "none", DefaultRemote: "origin", BranchStrategy: "manual", GovernanceFiles: "local_only",
		},
		Memory: config.MemoryPersist{Strategy: "sqlite_plus_context_capsule"},
		Skills: config.SkillsPersist{Enabled: []config.SkillPinPersist{{ID: "testing", Version: "latest"}}},
	}
	if err := config.ValidateProjectDocument(doc); err == nil {
		t.Fatal("expected latest pin rejected")
	}
}

func TestAgentSkillRefs_InRegistry(t *testing.T) {
	text := config.RenderAgentRegistry("demo", []string{"cursor"}, "")
	if !strings.Contains(text, "Skills:") || !strings.Contains(text, "`testing`") {
		t.Fatalf("agent registry missing skill ids:\n%s", text)
	}
	if strings.Contains(text, ".cursor/skills") {
		t.Fatal("registry must not require provider skill paths")
	}
	_ = skills.DefaultPins()
}
