// Command e2ecompose materializes isolated Atlas projects under a validated
// E2E root and verifies Context Composition through the real atlas binary.
//
// Required env:
//
//	ATLAS_E2E_ROOT  — existing real directory that will hold unique run workspaces
//	ATLAS_BIN       — absolute path to atlas binary built from the validated HEAD
//
// Safety:
//   - Never deletes pre-existing directories under the E2E root.
//   - Creates a unique atlas-e2e-compose-* workspace per run.
//   - On failure, removes only that uniquely created workspace (unless ATLAS_E2E_KEEP=1).
//   - On success, retains the workspace as evidence.
//   - No network dependency; no Git mutations; no production Home/Memory touch.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
)

const (
	canaryMemA = "CANARY-MEM-36B6E-ALPHA-SQLite-under-Home"
	canaryMemB = "CANARY-MEM-36B6E-BRAVO-isolation-peer"
	canaryCEA  = "CANARY-CE-36B6E-ALPHA.md"
	canaryCEB  = "CANARY-CE-36B6E-BRAVO.md"
	taskOrient = "orient atlas architecture CANARY-MEM-36B6E-ALPHA"
)

type matrixRow struct {
	ID     string `json:"id"`
	Result string `json:"result"` // PASS | FAIL | SKIPPED
	Detail string `json:"detail,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2ecompose: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("e2ecompose: ok")
}

func run() error {
	root, err := validateE2ERoot(os.Getenv("ATLAS_E2E_ROOT"))
	if err != nil {
		return err
	}
	atlasBin, err := resolveAtlasBin(os.Getenv("ATLAS_BIN"))
	if err != nil {
		return err
	}

	ws, err := prepareRunWorkspace(root)
	if err != nil {
		return err
	}
	keepEvidence := strings.TrimSpace(os.Getenv("ATLAS_E2E_KEEP")) == "1"
	defer func() {
		if keepEvidence {
			return
		}
		_ = ws.Cleanup()
	}()

	evidenceDir := filepath.Join(ws.RunDir, "evidence")
	if err := os.Mkdir(evidenceDir, 0o755); err != nil {
		return err
	}

	verRun, err := runBounded(context.Background(), 10*time.Second, 4*1024, 4*1024, atlasBin, []string{"--version"}, "", os.Environ())
	if err != nil && !verRun.JSONOK {
		return fmt.Errorf("atlas --version: %w", err)
	}
	version := strings.TrimSpace(string(verRun.Stdout))
	if version == "" {
		return fmt.Errorf("atlas --version: empty output")
	}

	projA := filepath.Join(ws.RunDir, "project-a")
	projB := filepath.Join(ws.RunDir, "project-b")
	projMem := filepath.Join(ws.RunDir, "project-mem-only")
	projCE := filepath.Join(ws.RunDir, "project-ce-only")
	projEmpty := filepath.Join(ws.RunDir, "project-empty")
	for _, p := range []string{projA, projB, projMem, projCE, projEmpty} {
		if err := os.Mkdir(p, 0o755); err != nil {
			return err
		}
	}
	// Default prepareRunWorkspace also created ws.Project; leave it unused as a sentinel.
	_ = ws.Project

	fixed := time.Date(2026, 10, 12, 18, 0, 0, 0, time.UTC)
	idA, err := initProject(projA, ws.AtlasHome, "compose-a-36b6e", true, true, fixed)
	if err != nil {
		return fmt.Errorf("init project-a: %w", err)
	}
	idB, err := initProject(projB, ws.AtlasHome, "compose-b-36b6e", true, true, fixed)
	if err != nil {
		return fmt.Errorf("init project-b: %w", err)
	}
	idMem, err := initProject(projMem, ws.AtlasHome, "compose-mem-36b6e", true, false, fixed)
	if err != nil {
		return fmt.Errorf("init project-mem: %w", err)
	}
	idCE, err := initProject(projCE, ws.AtlasHome, "compose-ce-36b6e", false, true, fixed)
	if err != nil {
		return fmt.Errorf("init project-ce: %w", err)
	}
	idEmpty, err := initProject(projEmpty, ws.AtlasHome, "compose-empty-36b6e", false, false, fixed)
	if err != nil {
		return fmt.Errorf("init project-empty: %w", err)
	}

	if idA == idB || idA == idMem || idA == idCE {
		return fmt.Errorf("project id collision: a=%s b=%s mem=%s ce=%s", idA, idB, idMem, idCE)
	}

	// Distinct canaries.
	if err := seedMemory(ws.AtlasHome, idA, "Alpha canary", canaryMemA); err != nil {
		return err
	}
	if err := seedMemory(ws.AtlasHome, idB, "Bravo canary", canaryMemB); err != nil {
		return err
	}
	if err := seedMemory(ws.AtlasHome, idMem, "Mem-only canary", canaryMemA+"-MEMONLY"); err != nil {
		return err
	}
	if err := writeCanaryFile(projA, canaryCEA, "# alpha CE canary\n"); err != nil {
		return err
	}
	if err := writeCanaryFile(projB, canaryCEB, "# bravo CE canary\n"); err != nil {
		return err
	}
	if err := writeCanaryFile(projCE, canaryCEA, "# ce-only canary\n"); err != nil {
		return err
	}
	packIDA, err := rebuildCE(projA, ws.AtlasHome, idA, "compose-a-36b6e", fixed, canaryCEA)
	if err != nil {
		return err
	}
	if _, err := rebuildCE(projB, ws.AtlasHome, idB, "compose-b-36b6e", fixed, canaryCEB); err != nil {
		return err
	}
	if _, err := rebuildCE(projCE, ws.AtlasHome, idCE, "compose-ce-36b6e", fixed, canaryCEA); err != nil {
		return err
	}

	beforeFP, err := fingerprintTree(ws.AtlasHome)
	if err != nil {
		return err
	}
	beforeProjA, err := fingerprintTree(projA)
	if err != nil {
		return err
	}

	var matrix []matrixRow
	pass := func(id, detail string) { matrix = append(matrix, matrixRow{ID: id, Result: "PASS", Detail: detail}) }
	fail := func(id, detail string) error {
		matrix = append(matrix, matrixRow{ID: id, Result: "FAIL", Detail: detail})
		_ = writeMatrix(evidenceDir, matrix, atlasBin, version, ws)
		return fmt.Errorf("%s: %s", id, detail)
	}

	// --- A: both sources ---
	resBoth, rawBoth, err := runCompose(atlasBin, ws.AtlasHome, projA, taskOrient, 0, 0)
	if err != nil {
		return fail("e2e.compose.both", sanitizeHarnessErr(err))
	}
	if err := writeEvidence(evidenceDir, "compose-both.json", rawBoth); err != nil {
		return err
	}
	if resBoth.SchemaVersion != contextcompose.SchemaVersion {
		return fail("e2e.compose.both.schema", fmt.Sprintf("schema=%d", resBoth.SchemaVersion))
	}
	if resBoth.ProjectID != idA {
		return fail("e2e.compose.both.project_id", fmt.Sprintf("got=%q want=%q", resBoth.ProjectID, idA))
	}
	if resBoth.UsedChars > resBoth.BudgetChars {
		return fail("e2e.compose.both.budget", fmt.Sprintf("used=%d budget=%d", resBoth.UsedChars, resBoth.BudgetChars))
	}
	if resBoth.BudgetChars != contextcompose.DefaultBudgetChars {
		return fail("e2e.compose.both.default_budget", fmt.Sprintf("budget=%d", resBoth.BudgetChars))
	}
	if !hasSource(resBoth, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		return fail("e2e.compose.both.memory_status", fmt.Sprintf("%v", resBoth.Sources))
	}
	if !hasSource(resBoth, contextcompose.SourceContextEconomy, contextcompose.SourceStatusSupplied) {
		return fail("e2e.compose.both.ce_status", fmt.Sprintf("%v", resBoth.Sources))
	}
	if !containsCanary(resBoth, canaryMemA) {
		return fail("e2e.compose.both.mem_canary", "missing memory canary")
	}
	if !containsPath(resBoth, canaryCEA) {
		return fail("e2e.compose.both.ce_canary", "missing CE canary path")
	}
	if containsCanary(resBoth, canaryMemB) || containsPath(resBoth, canaryCEB) {
		return fail("e2e.compose.both.isolation_leak", "project-b artifacts present in project-a")
	}
	if jsonLeaksAbsPaths(rawBoth, ws.AtlasHome, projA) {
		return fail("e2e.compose.both.path_leak", "absolute Home/project paths in JSON")
	}
	pass("e2e.compose.both", fmt.Sprintf("status=%s count=%d used=%d", resBoth.Status, resBoth.Count, resBoth.UsedChars))

	// Determinism
	resBoth2, _, err := runCompose(atlasBin, ws.AtlasHome, projA, taskOrient, 0, 0)
	if err != nil {
		return fail("e2e.compose.determinism", sanitizeHarnessErr(err))
	}
	a, _ := json.Marshal(resBoth.Items)
	b, _ := json.Marshal(resBoth2.Items)
	if string(a) != string(b) {
		return fail("e2e.compose.determinism", "items diverged across runs")
	}
	pass("e2e.compose.determinism", "identical items across two runs")

	// Isolation: compose in B must not see A
	resB, rawB, err := runCompose(atlasBin, ws.AtlasHome, projB, "isolation peer CANARY-MEM-36B6E-BRAVO", 0, 0)
	if err != nil {
		return fail("e2e.isolation.project_b", sanitizeHarnessErr(err))
	}
	_ = writeEvidence(evidenceDir, "compose-project-b.json", rawB)
	if resB.ProjectID != idB {
		return fail("e2e.isolation.project_b.id", resB.ProjectID)
	}
	if containsCanary(resB, canaryMemA) || containsPath(resB, canaryCEA) {
		return fail("e2e.isolation.cross_project", "project-a leaked into project-b")
	}
	if !containsCanary(resB, canaryMemB) {
		return fail("e2e.isolation.project_b.canary", "missing bravo memory canary")
	}
	pass("e2e.isolation", fmt.Sprintf("a=%s b=%s", idA, idB))

	// Memory only / CE only / empty
	resMem, _, err := runCompose(atlasBin, ws.AtlasHome, projMem, taskOrient+"-MEMONLY", 0, 0)
	if err != nil {
		return fail("e2e.compose.mem_only", sanitizeHarnessErr(err))
	}
	if !hasSource(resMem, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		return fail("e2e.compose.mem_only.memory", "memory must be supplied")
	}
	ceMemOnly := sourceOf(resMem, contextcompose.SourceContextEconomy)
	if ceMemOnly != contextcompose.SourceStatusUnavailable && ceMemOnly != contextcompose.SourceStatusNoCandidates {
		return fail("e2e.compose.mem_only.ce", fmt.Sprintf("want unavailable|no_candidates got %q", ceMemOnly))
	}
	pass("e2e.compose.mem_only", resMem.Status)

	resCEOnly, _, err := runCompose(atlasBin, ws.AtlasHome, projCE, "orient architecture", 0, 0)
	if err != nil {
		return fail("e2e.compose.ce_only", sanitizeHarnessErr(err))
	}
	if !hasSource(resCEOnly, contextcompose.SourceContextEconomy, contextcompose.SourceStatusSupplied) {
		return fail("e2e.compose.ce_only.ce", "context_economy must be supplied")
	}
	memSt := sourceOf(resCEOnly, contextcompose.SourceMemory)
	if memSt != contextcompose.SourceStatusUnavailable && memSt != contextcompose.SourceStatusNoCandidates {
		return fail("e2e.compose.ce_only.memory", fmt.Sprintf("want unavailable|no_candidates got %q", memSt))
	}
	pass("e2e.compose.ce_only", resCEOnly.Status)

	resEmpty, _, err := runCompose(atlasBin, ws.AtlasHome, projEmpty, "orient nothing", 0, 0)
	if resEmpty.SchemaVersion != contextcompose.SchemaVersion {
		return fail("e2e.compose.empty", sanitizeHarnessErr(err))
	}
	if resEmpty.Count != 0 {
		return fail("e2e.compose.empty.count", fmt.Sprintf("count=%d", resEmpty.Count))
	}
	if resEmpty.Status == contextcompose.StatusSuccess {
		return fail("e2e.compose.empty.fabricated", "success with no sources is not allowed")
	}
	pass("e2e.compose.empty", fmt.Sprintf("status=%s count=%d", resEmpty.Status, resEmpty.Count))
	_ = idEmpty

	// Budgets
	for _, tc := range []struct {
		id, detail string
		budget     int
		wantStatus string
		check      func(composeapp.Result) error
	}{
		{"e2e.budget.default", "8000", 0, "", func(r composeapp.Result) error {
			if r.BudgetChars != 8000 {
				return fmt.Errorf("budget=%d", r.BudgetChars)
			}
			if r.UsedChars > r.BudgetChars {
				return fmt.Errorf("used>budget")
			}
			return nil
		}},
		{"e2e.budget.max", "32000", 32000, "", func(r composeapp.Result) error {
			if r.BudgetChars != 32000 {
				return fmt.Errorf("budget=%d", r.BudgetChars)
			}
			return nil
		}},
		{"e2e.budget.min", "1", 1, "", func(r composeapp.Result) error {
			if r.BudgetChars != 1 {
				return fmt.Errorf("budget=%d", r.BudgetChars)
			}
			if r.UsedChars > 1 {
				return fmt.Errorf("used=%d", r.UsedChars)
			}
			return nil
		}},
		{"e2e.budget.oversized", "32001", 32001, contextcompose.StatusInvalidRequest, func(r composeapp.Result) error {
			if r.Status != contextcompose.StatusInvalidRequest {
				return fmt.Errorf("status=%s", r.Status)
			}
			return nil
		}},
		{"e2e.budget.unicode", "unicode", 4000, "", func(r composeapp.Result) error {
			// used_chars is runes — verify field present and within budget
			if r.UsedChars > r.BudgetChars {
				return fmt.Errorf("used=%d budget=%d", r.UsedChars, r.BudgetChars)
			}
			return nil
		}},
	} {
		task := taskOrient
		if tc.id == "e2e.budget.unicode" {
			task = "orient " + strings.Repeat("界", 20) + " " + canaryMemA
		}
		r, raw, err := runCompose(atlasBin, ws.AtlasHome, projA, task, tc.budget, 0)
		if tc.wantStatus == contextcompose.StatusInvalidRequest {
			if r.SchemaVersion != contextcompose.SchemaVersion {
				return fail(tc.id, sanitizeHarnessErr(err))
			}
			if r.Status != contextcompose.StatusInvalidRequest {
				return fail(tc.id, fmt.Sprintf("status=%s", r.Status))
			}
			if err == nil {
				return fail(tc.id, "expected non-zero exit for invalid budget")
			}
		} else if err != nil {
			return fail(tc.id, sanitizeHarnessErr(err))
		}
		if tc.check != nil {
			if err := tc.check(r); err != nil {
				_ = writeEvidence(evidenceDir, tc.id+".json", raw)
				return fail(tc.id, err.Error())
			}
		}
		pass(tc.id, tc.detail)
	}

	// Skill / agent projection (from ApplyConfig) — read-only checks first.
	for _, rel := range []string{
		"AGENTS.md",
		".cursor/skills/memory/SKILL.md",
		".opencode/skills/memory/SKILL.md",
	} {
		body, err := os.ReadFile(filepath.Join(projA, filepath.FromSlash(rel)))
		if err != nil {
			return fail("e2e.projection."+filepath.Base(rel), err.Error())
		}
		if !strings.Contains(string(body), "atlas context compose") {
			return fail("e2e.projection."+filepath.Base(rel), "missing compose instruction")
		}
	}
	pass("e2e.projection.skills", "AGENTS.md + cursor/opencode memory skill teach compose")

	afterFP, err := fingerprintTree(ws.AtlasHome)
	if err != nil {
		return err
	}
	afterProjA, err := fingerprintTree(projA)
	if err != nil {
		return err
	}
	if beforeFP != afterFP {
		return fail("e2e.readonly.home", "ATLAS_HOME mutated during composition")
	}
	if beforeProjA != afterProjA {
		return fail("e2e.readonly.project", "project-a mutated during composition")
	}
	pass("e2e.readonly", "Home + project-a fingerprints unchanged across compose calls")

	// Security mutations (restored afterward; not part of read-only fingerprint window).
	// Use taskOrient so Memory canary remains retrievable while CE pack is attacked.
	packRel := atlascontext.ContextPackRel(idA, packIDA)
	packAbs := filepath.Join(ws.AtlasHome, filepath.FromSlash(packRel))
	outside := filepath.Join(ws.RunDir, "outside-pack.yaml")
	if err := os.WriteFile(outside, []byte("schema_version: 1\n"), 0o600); err != nil {
		return err
	}
	bak, err := os.ReadFile(packAbs)
	if err != nil {
		return fmt.Errorf("read pack for security tests: %w", err)
	}
	// Symlink every pack under the project context packs dir.
	packsDir := filepath.Dir(packAbs)
	entries, err := os.ReadDir(packsDir)
	if err != nil {
		return err
	}
	type packBak struct {
		path string
		data []byte
	}
	var backups []packBak
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		p := filepath.Join(packsDir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		backups = append(backups, packBak{path: p, data: data})
		_ = os.Remove(p)
		if err := os.Symlink(outside, p); err != nil {
			return err
		}
	}
	resSym, rawSym, symErr := runCompose(atlasBin, ws.AtlasHome, projA, taskOrient, 0, 0)
	_ = writeEvidence(evidenceDir, "compose-symlink-pack.json", rawSym)
	for _, b := range backups {
		_ = os.Remove(b.path)
		_ = os.WriteFile(b.path, b.data, 0o600)
	}
	if resSym.SchemaVersion != contextcompose.SchemaVersion {
		return fail("e2e.security.symlink_pack.decode", sanitizeHarnessErr(symErr))
	}
	ceSym := sourceOf(resSym, contextcompose.SourceContextEconomy)
	if ceSym != contextcompose.SourceStatusFailed {
		return fail("e2e.security.symlink_pack.ce", fmt.Sprintf("want failed got %q", ceSym))
	}
	if !hasSource(resSym, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		return fail("e2e.security.symlink_pack.memory", "memory must remain supplied after CE symlink rejection")
	}
	if !containsCanary(resSym, canaryMemA) {
		return fail("e2e.security.symlink_pack.canary", "memory canary missing after CE failure")
	}
	pass("e2e.security.symlink_pack", "CE failed closed; Memory supplied with canary")

	if err := os.WriteFile(packAbs, []byte(strings.Repeat("#", 256*1024+64)), 0o600); err != nil {
		return err
	}
	// Oversized default pack; also pad task packs for fail-closed coverage.
	for _, b := range backups {
		if b.path == packAbs {
			continue
		}
		_ = os.WriteFile(b.path, []byte(strings.Repeat("#", 256*1024+64)), 0o600)
	}
	resOver, _, overErr := runCompose(atlasBin, ws.AtlasHome, projA, taskOrient, 0, 0)
	for _, b := range backups {
		_ = os.WriteFile(b.path, b.data, 0o600)
	}
	_ = os.WriteFile(packAbs, bak, 0o600)
	if resOver.SchemaVersion != contextcompose.SchemaVersion {
		return fail("e2e.security.oversized_pack.decode", sanitizeHarnessErr(overErr))
	}
	if sourceOf(resOver, contextcompose.SourceContextEconomy) != contextcompose.SourceStatusFailed {
		return fail("e2e.security.oversized_pack", fmt.Sprintf("want failed got %q", sourceOf(resOver, contextcompose.SourceContextEconomy)))
	}
	if !hasSource(resOver, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		return fail("e2e.security.oversized_pack.memory", "memory must remain supplied")
	}
	pass("e2e.security.oversized_pack", "CE failed on >256KiB; Memory supplied")

	huge := strings.Repeat("t", contextcompose.MaxTaskRunes+10)
	resHuge, _, hugeErr := runCompose(atlasBin, ws.AtlasHome, projA, huge, 0, 0)
	if resHuge.SchemaVersion != contextcompose.SchemaVersion {
		return fail("e2e.security.oversized_task.decode", sanitizeHarnessErr(hugeErr))
	}
	if resHuge.Status != contextcompose.StatusInvalidRequest {
		return fail("e2e.security.oversized_task", resHuge.Status)
	}
	if hugeErr == nil {
		return fail("e2e.security.oversized_task.exit", "expected non-zero exit")
	}
	pass("e2e.security.oversized_task", "invalid_request")

	missingRun, missingErr := runBounded(
		context.Background(),
		defaultComposeTimeout,
		maxStdoutCapture,
		maxStderrCapture,
		atlasBin,
		[]string{"context", "compose", "task"},
		projA,
		withEnv(os.Environ(), map[string]string{"ATLAS_HOME": ws.AtlasHome}),
	)
	if !missingRun.JSONOK || missingRun.Result.Status != contextcompose.StatusInvalidRequest {
		return fail("e2e.security.missing_json", sanitizeHarnessErr(missingErr))
	}
	if missingErr == nil {
		return fail("e2e.security.missing_json.exit", "expected non-zero exit")
	}
	pass("e2e.security.missing_json", "invalid_request")

	matrix = append(matrix, matrixRow{
		ID:     "e2e.agent.cursor.consumption",
		Result: "SKIPPED",
		Detail: "manual procedure in evidence/AGENT_CONSUMPTION.md — CLI JSON alone is not agent consumption",
	})
	matrix = append(matrix, matrixRow{
		ID:     "e2e.agent.opencode.consumption",
		Result: "SKIPPED",
		Detail: "manual procedure in evidence/AGENT_CONSUMPTION.md — CLI JSON alone is not agent consumption",
	})

	if err := writeAgentProcedure(evidenceDir, atlasBin, version, projA, ws.AtlasHome, idA); err != nil {
		return err
	}
	if err := writeMatrix(evidenceDir, matrix, atlasBin, version, ws); err != nil {
		return err
	}

	fmt.Printf("atlas_bin=%s\n", atlasBin)
	fmt.Printf("atlas_version=%s\n", version)
	fmt.Printf("e2e_root=%s\n", ws.Root)
	fmt.Printf("run_dir=%s\n", ws.RunDir)
	fmt.Printf("ATLAS_HOME=%s\n", ws.AtlasHome)
	fmt.Printf("project_a=%s id=%s\n", projA, idA)
	fmt.Printf("project_b=%s id=%s\n", projB, idB)
	fmt.Printf("evidence=%s\n", evidenceDir)
	fmt.Println("note: used_chars/budget_chars are UTF-8 runes, not tokenizer tokens")
	fmt.Println("note: automatic context injection is not claimed")
	fmt.Println("note: Cursor/OpenCode agent-consumption requires manual procedure (SKIPPED)")
	fmt.Println("note: run workspace retained as E2E evidence")
	keepEvidence = true
	return nil
}

func resolveAtlasBin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("ATLAS_BIN is required (absolute path to binary built from validated HEAD)")
	}
	if !filepath.IsAbs(raw) {
		abs, err := filepath.Abs(raw)
		if err != nil {
			return "", err
		}
		raw = abs
	}
	info, err := os.Lstat(raw)
	if err != nil {
		return "", fmt.Errorf("ATLAS_BIN: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// Allow resolving a symlink leaf to a real binary, but record resolved path.
		resolved, err := filepath.EvalSymlinks(raw)
		if err != nil {
			return "", err
		}
		raw = resolved
		info, err = os.Lstat(raw)
		if err != nil {
			return "", err
		}
	}
	if info.IsDir() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("ATLAS_BIN is not an executable file: %s", raw)
	}
	return raw, nil
}

func initProject(root, atlasHome, name string, withMemoryPlaceholder, withCEPlaceholder bool, now time.Time) (string, error) {
	_ = withMemoryPlaceholder
	_ = withCEPlaceholder
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      name,
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: true,
	})
	if !draft.SetValue("adapters.selected", "cursor,opencode") {
		return "", fmt.Errorf("set adapters.selected")
	}
	// Isolate ApplyConfig Home via env for this process (caller sets ATLAS_HOME too).
	if err := os.Setenv("ATLAS_HOME", atlasHome); err != nil {
		return "", err
	}
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return now },
	}); err != nil {
		return "", fmt.Errorf("ApplyConfig: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module "+name+"\n"), 0o644); err != nil {
		return "", err
	}
	id, err := home.ProjectID(root, name)
	if err != nil {
		return "", err
	}
	return id, nil
}

func seedMemory(atlasHome, projectID, title, content string) error {
	store, err := memory.Open(atlasHome, projectID)
	if err != nil {
		return err
	}
	defer store.Close()
	_, err = store.SaveObservation(memory.Observation{
		Type:    memory.TypeFact,
		Title:   title,
		Content: content,
	})
	return err
}

func writeCanaryFile(root, name, body string) error {
	return os.WriteFile(filepath.Join(root, name), []byte(body), 0o644)
}

func rebuildCE(root, atlasHome, projectID, name string, now time.Time, ensurePath string) (string, error) {
	idx, err := atlascontext.BuildIndex(root, name, now)
	if err != nil {
		return "", err
	}
	idx.ProjectID = projectID
	if ensurePath != "" {
		found := false
		for _, e := range idx.Files {
			if e.Path == ensurePath {
				found = true
				break
			}
		}
		if !found {
			return "", fmt.Errorf("CE index missing canary path %q", ensurePath)
		}
	}
	indexYAML, err := atlascontext.RenderIndexYAML(idx)
	if err != nil {
		return "", err
	}
	capsule := atlascontext.RenderCapsule(idx)
	pack := atlascontext.BuildPack(idx, atlascontext.DefaultPackObjective, now)
	// Guarantee canary path membership for E2E assertions.
	if ensurePath != "" {
		has := false
		for _, c := range pack.Candidates {
			if c.Path == ensurePath {
				has = true
				break
			}
		}
		if !has {
			pack.Candidates = append([]atlascontext.PackCandidate{{
				Path: ensurePath, Kind: "doc", Reason: "e2e canary",
			}}, pack.Candidates...)
			if len(pack.Candidates) > 24 {
				pack.Candidates = pack.Candidates[:24]
			}
		}
	}
	packYAML, err := atlascontext.RenderPackYAML(pack)
	if err != nil {
		return "", err
	}
	// Also store a pack keyed to the orientation task so task-specific lookup hits.
	taskPack := atlascontext.BuildPack(idx, taskOrient, now)
	if ensurePath != "" {
		has := false
		for _, c := range taskPack.Candidates {
			if c.Path == ensurePath {
				has = true
				break
			}
		}
		if !has {
			taskPack.Candidates = append([]atlascontext.PackCandidate{{
				Path: ensurePath, Kind: "doc", Reason: "e2e canary",
			}}, taskPack.Candidates...)
			if len(taskPack.Candidates) > 24 {
				taskPack.Candidates = taskPack.Candidates[:24]
			}
		}
	}
	taskPackYAML, err := atlascontext.RenderPackYAML(taskPack)
	if err != nil {
		return "", err
	}
	for _, p := range []struct {
		path string
		data string
	}{
		{atlascontext.IndexPath(atlasHome, projectID), indexYAML},
		{atlascontext.CapsulePath(atlasHome, projectID), capsule},
		{atlascontext.PackPath(atlasHome, projectID, pack.PackID), packYAML},
		{atlascontext.PackPath(atlasHome, projectID, taskPack.PackID), taskPackYAML},
	} {
		if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(p.path, []byte(p.data), 0o600); err != nil {
			return "", err
		}
	}
	return pack.PackID, nil
}

func runCompose(atlasBin, atlasHome, project, task string, budget, limit int) (composeapp.Result, []byte, error) {
	args := []string{"context", "compose", task, "--json"}
	if budget > 0 {
		args = append(args, "--budget", fmt.Sprintf("%d", budget))
	}
	if limit > 0 {
		args = append(args, "--limit", fmt.Sprintf("%d", limit))
	}
	rr, err := runBounded(
		context.Background(),
		defaultComposeTimeout,
		maxStdoutCapture,
		maxStderrCapture,
		atlasBin,
		args,
		project,
		withEnv(os.Environ(), map[string]string{"ATLAS_HOME": atlasHome}),
	)
	if rr.JSONOK {
		return rr.Result, rr.Stdout, err
	}
	if err == nil {
		err = errors.New("compose stdout was not valid JSON")
	}
	return rr.Result, rr.Stdout, err
}

func sanitizeHarnessErr(err error) string {
	if err == nil {
		return "unknown error"
	}
	msg := err.Error()
	switch {
	case errors.Is(err, errProcessTimeout):
		return "subprocess timed out"
	case errors.Is(err, errOutputOverflow):
		return "subprocess output exceeded capture bound"
	default:
		return redactPaths(msg)
	}
}

func hasSource(res composeapp.Result, source, status string) bool {
	for _, s := range res.Sources {
		if s.Source == source && s.Status == status {
			return true
		}
	}
	return false
}

func sourceOf(res composeapp.Result, source string) string {
	for _, s := range res.Sources {
		if s.Source == source {
			return s.Status
		}
	}
	return ""
}

func containsCanary(res composeapp.Result, canary string) bool {
	for _, it := range res.Items {
		if strings.Contains(it.Title, canary) || strings.Contains(it.Excerpt, canary) || strings.Contains(it.Ref, canary) || strings.Contains(it.ID, canary) {
			return true
		}
	}
	return false
}

func containsPath(res composeapp.Result, path string) bool {
	for _, it := range res.Items {
		if it.Path == path || strings.Contains(it.Path, path) {
			return true
		}
	}
	return false
}

func jsonLeaksAbsPaths(raw []byte, paths ...string) bool {
	s := string(raw)
	for _, p := range paths {
		if p != "" && strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func writeEvidence(dir, name string, raw []byte) error {
	// Sanitize: truncate oversized evidence.
	if len(raw) > 512*1024 {
		raw = raw[:512*1024]
	}
	return os.WriteFile(filepath.Join(dir, name), raw, 0o600)
}

func writeMatrix(dir string, rows []matrixRow, atlasBin, version string, ws *runWorkspace) error {
	doc := map[string]any{
		"slice":          "36B.6E",
		"atlas_bin":      atlasBin,
		"atlas_version":  version,
		"e2e_root":       ws.Root,
		"run_dir":        ws.RunDir,
		"ATLAS_HOME":     ws.AtlasHome,
		"budget_unit":    "utf8_runes_not_tokens",
		"auto_injection": false,
		"matrix":         rows,
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "MATRIX.json"), b, 0o600)
}

func writeAgentProcedure(dir, atlasBin, version, project, atlasHome, projectID string) error {
	body := fmt.Sprintf(`# Agent consumption procedure (36B.6E)

CLI JSON execution alone does **not** prove agent consumption.
Interactive verification is required for Cursor and OpenCode.

## Environment

- atlas_bin: %s
- atlas_version: %s
- project: %s
- ATLAS_HOME: %s
- project_id: %s
- canary memory: %s
- canary CE path: %s

## Preconditions

1. Open the project directory in Cursor (and separately in OpenCode).
2. Confirm projected files exist:
   - AGENTS.md contains "atlas context compose"
   - .cursor/skills/memory/SKILL.md (Cursor) or .opencode/skills/memory/SKILL.md (OpenCode)
3. Do not modify global agent settings.
4. Do not claim native automatic injection.

## Procedure (each agent)

1. Start a new agent chat in the project root.
2. Ask a substantive orientation task that requires prior context, e.g.:
   "Orient me on this Atlas project's architecture. Use Atlas composed context and cite any canary facts you retrieve."
3. Observe whether the agent invokes:
   `+"`"+`atlas context compose "<task>" --json`+"`"+`
   (or the Memory skill path first, then compose for broader orientation).
4. Confirm the agent answer references the canary Memory content and/or CE path.
5. Confirm the agent does not claim automatic hook injection of context.

## Recording

Save a short transcript excerpt (command invocation + canary citation) under this evidence directory as:
- cursor-consumption.txt
- opencode-consumption.txt

Mark matrix rows PASS only when those files exist and show canary use.
`, atlasBin, version, project, atlasHome, projectID, canaryMemA, canaryCEA)
	return os.WriteFile(filepath.Join(dir, "AGENT_CONSUMPTION.md"), []byte(body), 0o600)
}
