// Command e2ememory materializes an isolated Atlas project under a validated
// E2E root and verifies Memory Skill projection + read-only CLI context retrieval.
//
// Required env:
//
//	ATLAS_E2E_ROOT  — existing real directory that will hold unique run workspaces
//	ATLAS_BIN       — path to atlas binary (optional; defaults to PATH "atlas")
//
// Safety:
//   - Never deletes pre-existing directories under the E2E root.
//   - Creates a unique atlas-e2e-memory-* workspace per run.
//   - On failure, removes only that uniquely created workspace.
//   - On success, retains the workspace as evidence.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2ememory: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("e2ememory: ok")
}

func run() error {
	root, err := validateE2ERoot(os.Getenv("ATLAS_E2E_ROOT"))
	if err != nil {
		return err
	}
	atlasBin := strings.TrimSpace(os.Getenv("ATLAS_BIN"))
	if atlasBin == "" {
		atlasBin = "atlas"
	}

	ws, err := prepareRunWorkspace(root)
	if err != nil {
		return err
	}
	// Failed initialization cleans only the uniquely created run workspace.
	// Successful runs retain evidence under RunDir.
	// Set ATLAS_E2E_KEEP=1 to retain the workspace even on failure (debug).
	keepEvidence := strings.TrimSpace(os.Getenv("ATLAS_E2E_KEEP")) == "1"
	defer func() {
		if keepEvidence {
			return
		}
		_ = ws.Cleanup()
	}()

	if err := os.Setenv("ATLAS_HOME", ws.AtlasHome); err != nil {
		return err
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      "memory-skill-36b5",
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: true,
	})
	if !draft.SetValue("adapters.selected", "cursor,opencode") {
		return fmt.Errorf("set adapters.selected")
	}
	fixed := time.Date(2026, 10, 11, 23, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  ws.Project,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed },
	}); err != nil {
		return fmt.Errorf("ApplyConfig: %w", err)
	}

	homeSkill := filepath.Join(ws.AtlasHome, "assets", "skills", "memory", "1.0.0", "SKILL.md")
	if _, err := os.Lstat(homeSkill); err != nil {
		return fmt.Errorf("Home memory skill missing: %w", err)
	}
	for _, rel := range []string{
		".cursor/skills/memory/SKILL.md",
		".opencode/skills/memory/SKILL.md",
		".atlas/skill-registry.md",
		".atlas/agent-registry.md",
		"AGENTS.md",
	} {
		if _, err := os.Lstat(filepath.Join(ws.Project, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("missing %s: %w", rel, err)
		}
	}
	reg, err := os.ReadFile(filepath.Join(ws.Project, ".atlas", "skill-registry.md"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(reg), "memory") {
		return fmt.Errorf("skill-registry missing memory")
	}
	agentsReg, err := os.ReadFile(filepath.Join(ws.Project, ".atlas", "agent-registry.md"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(agentsReg), "`memory`") {
		return fmt.Errorf("agent-registry missing memory refs")
	}
	agentsMD, err := os.ReadFile(filepath.Join(ws.Project, "AGENTS.md"))
	if err != nil {
		return err
	}
	if !strings.Contains(string(agentsMD), "## 10. Project Memory") {
		return fmt.Errorf("AGENTS.md missing Project Memory")
	}
	if strings.Contains(string(agentsMD), "Future Memory") {
		return fmt.Errorf("AGENTS.md still says Future Memory")
	}

	projectID, err := home.ProjectID(ws.Project, "memory-skill-36b5")
	if err != nil {
		return err
	}
	store, err := memory.Open(ws.AtlasHome, projectID)
	if err != nil {
		return err
	}
	if _, err := store.SaveObservation(memory.Observation{
		Type:    memory.TypeDecision,
		Title:   "Prefer SQLite Memory Core",
		Content: "Atlas stores project Memory in SQLite under Atlas Home, not in the product repo.",
	}); err != nil {
		_ = store.Close()
		return err
	}
	if err := store.Close(); err != nil {
		return err
	}
	ok, err := memory.Exists(ws.AtlasHome, projectID)
	if err != nil {
		return fmt.Errorf("memory exists check: %w", err)
	}
	if !ok {
		return fmt.Errorf("seeded memory.db missing under ATLAS_HOME=%s project_id=%s", ws.AtlasHome, projectID)
	}
	resolvedHome, err := home.Resolve()
	if err != nil {
		return err
	}
	if filepath.Clean(resolvedHome) != filepath.Clean(ws.AtlasHome) {
		return fmt.Errorf("ATLAS_HOME mismatch: resolve=%s workspace=%s", resolvedHome, ws.AtlasHome)
	}

	cmd := exec.Command(atlasBin, "memory", "context", "SQLite Memory Core storage", "--json")
	cmd.Dir = ws.Project
	cmd.Env = withEnv(os.Environ(), map[string]string{"ATLAS_HOME": ws.AtlasHome})
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("atlas memory context: %v\nATLAS_HOME=%s project=%s\n%s", err, ws.AtlasHome, ws.Project, out)
	}
	var resp memoryretrieve.Response
	if err := json.Unmarshal(out, &resp); err != nil {
		return fmt.Errorf("decode response: %w\n%s", err, out)
	}
	if resp.Status != memoryretrieve.StatusSuccess {
		return fmt.Errorf("status=%q body=%s", resp.Status, out)
	}
	if resp.Count < 1 || len(resp.Items) == 0 {
		return fmt.Errorf("expected context items, got %#v", resp)
	}
	item := resp.Items[0]
	if item.Type != string(memory.TypeDecision) || !strings.Contains(item.Title, "SQLite") {
		return fmt.Errorf("unexpected item: %#v", item)
	}

	fmt.Printf("e2e_root=%s\n", ws.Root)
	fmt.Printf("run_dir=%s\n", ws.RunDir)
	fmt.Printf("project=%s\n", ws.Project)
	fmt.Printf("ATLAS_HOME=%s\n", ws.AtlasHome)
	fmt.Printf("project_id=%s\n", projectID)
	fmt.Printf("home_skill=%s\n", homeSkill)
	fmt.Printf("cursor_skill=%s\n", filepath.Join(ws.Project, ".cursor", "skills", "memory", "SKILL.md"))
	fmt.Printf("opencode_skill=%s\n", filepath.Join(ws.Project, ".opencode", "skills", "memory", "SKILL.md"))
	fmt.Printf("context_status=%s count=%d title=%q type=%q used_chars=%d\n",
		resp.Status, resp.Count, item.Title, item.Type, resp.UsedChars)
	fmt.Println("note: automatic context injection is not claimed")
	fmt.Println("note: run workspace retained as E2E evidence")
	keepEvidence = true
	return nil
}
