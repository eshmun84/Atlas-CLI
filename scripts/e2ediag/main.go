// Command e2ediag certifies Memory diagnostics (Slice 36D) against isolated
// projects under Atlas-E2E.
//
// Required env:
//
//	ATLAS_E2E_ROOT  — existing real directory for unique run workspaces
//	ATLAS_BIN       — path to atlas binary (optional; defaults to PATH "atlas")
//
// Safety: never deletes pre-existing directories; creates atlas-e2e-diag-* only.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memoryhealth"
	"github.com/eshmun84/Atlas-CLI/internal/memorypersist"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2ediag: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("e2ediag: ok")
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

	// CLI routing smoke (doctor/status remain TUI routes).
	if act := cli.Resolve([]string{"status"}); act.Mode != cli.ModeTUI || act.Route != tui.RouteStatus {
		return fmt.Errorf("atlas status route unexpected: %+v", act)
	}
	if act := cli.Resolve([]string{"doctor"}); act.Mode != cli.ModeTUI || act.Route != tui.RouteDoctor {
		return fmt.Errorf("atlas doctor route unexpected: %+v", act)
	}

	fixed := time.Date(2026, 10, 13, 16, 0, 0, 0, time.UTC)
	results := map[string]string{}

	// --- Scenario A: healthy ---
	projectA := filepath.Join(ws.RunDir, "scenario-a")
	if err := os.Mkdir(projectA, 0o755); err != nil {
		return err
	}
	if err := applyInit(projectA, "diag-36d-alpha", fixed); err != nil {
		return fmt.Errorf("scenario A init: %w", err)
	}
	idA, err := home.ProjectID(projectA, "diag-36d-alpha")
	if err != nil {
		return err
	}
	env := withEnv(os.Environ(), map[string]string{"ATLAS_HOME": ws.AtlasHome})
	ctx := context.Background()
	cand := memorypersist.Candidate{
		Type:         string(memory.TypeDecision),
		Title:        "Prefer read-only Memory diagnostics",
		Content:      "Atlas Memory diagnostics must never create, migrate, or repair storage; they only report health.",
		Scope:        string(memory.ScopeProject),
		TopicKey:     "memory.diagnostics.policy",
		Origin:       "atlas_lifecycle",
		Explicitness: "explicit",
		Confidence:   "high",
		Provenance:   "cap=e2e-36d;ref=healthy",
		SourceEvent:  "e2e.diag.healthy",
	}
	resp, raw, err := runPropose(ctx, atlasBin, projectA, env, cand)
	if err != nil {
		return fmt.Errorf("scenario A propose: %w raw=%s", err, redactPaths(raw))
	}
	if resp.Status != memorypersist.StatusAccepted {
		return fmt.Errorf("scenario A propose status=%q raw=%s", resp.Status, redactPaths(raw))
	}

	beforeA, err := fingerprintPair(projectA, ws.AtlasHome)
	if err != nil {
		return err
	}
	diagA, statusA, doctorA, err := runDiagnostics(projectA)
	if err != nil {
		return fmt.Errorf("scenario A diagnostics: %w", err)
	}
	afterA, err := fingerprintPair(projectA, ws.AtlasHome)
	if err != nil {
		return err
	}
	if beforeA != afterA {
		return fmt.Errorf("scenario A diagnostics mutated fingerprints\nbefore=%s\nafter=%s", beforeA, afterA)
	}
	if diagA.Status != memoryhealth.StatusHealthy {
		return fmt.Errorf("scenario A want healthy got %s", diagA.Status)
	}
	if !strings.Contains(statusA, "Memory") || !strings.Contains(statusA, "healthy") {
		return fmt.Errorf("scenario A status missing Memory healthy:\n%s", redactPaths(statusA))
	}
	if !strings.Contains(doctorA, "Memory") || !strings.Contains(doctorA, "memory") {
		return fmt.Errorf("scenario A doctor missing Memory section:\n%s", redactPaths(doctorA))
	}
	// Memory diagnostics messages must not leak absolute Home paths (Home section may).
	for _, c := range diagA.Checks {
		if strings.Contains(c.Message, ws.AtlasHome) || strings.Contains(c.RemediationHint, ws.AtlasHome) {
			return fmt.Errorf("scenario A Memory check leaked ATLAS_HOME: %s", c.ID)
		}
	}
	results["A_healthy"] = "PASS"

	// --- Scenario B: degraded (governed + read-only storage) ---
	projectB := filepath.Join(ws.RunDir, "scenario-b")
	if err := os.Mkdir(projectB, 0o755); err != nil {
		return err
	}
	if err := applyInit(projectB, "diag-36d-bravo", fixed); err != nil {
		return fmt.Errorf("scenario B init: %w", err)
	}
	idB, err := home.ProjectID(projectB, "diag-36d-bravo")
	if err != nil {
		return err
	}
	candB := cand
	candB.Title = "Degraded writability canary"
	candB.TopicKey = "memory.diagnostics.degraded"
	candB.Provenance = "cap=e2e-36d;ref=degraded"
	resp, raw, err = runPropose(ctx, atlasBin, projectB, env, candB)
	if err != nil || resp.Status != memorypersist.StatusAccepted {
		return fmt.Errorf("scenario B propose status=%q err=%v raw=%s", resp.Status, err, redactPaths(raw))
	}
	dbRel, err := memory.DatabaseRel(idB)
	if err != nil {
		return err
	}
	dbAbs := filepath.Join(ws.AtlasHome, filepath.FromSlash(dbRel))
	if err := os.Chmod(dbAbs, 0o400); err != nil {
		return err
	}
	defer func() { _ = os.Chmod(dbAbs, 0o600) }()

	beforeB, err := fingerprintPair(projectB, ws.AtlasHome)
	if err != nil {
		return err
	}
	diagB, statusB, doctorB, err := runDiagnostics(projectB)
	if err != nil {
		return fmt.Errorf("scenario B diagnostics: %w", err)
	}
	afterB, err := fingerprintPair(projectB, ws.AtlasHome)
	if err != nil {
		return err
	}
	if beforeB != afterB {
		return fmt.Errorf("scenario B diagnostics mutated fingerprints")
	}
	if diagB.Status != memoryhealth.StatusDegraded {
		return fmt.Errorf("scenario B want exactly degraded got %s", diagB.Status)
	}
	if diagB.RetrievalLabel != "ready" {
		return fmt.Errorf("scenario B retrieval must remain ready, got %q", diagB.RetrievalLabel)
	}
	if !strings.Contains(statusB, "degraded") {
		return fmt.Errorf("scenario B status missing degraded:\n%s", redactPaths(statusB))
	}
	_ = doctorB
	results["B_degraded"] = "PASS"

	// --- Scenario C: failed (symlink DB) ---
	projectC := filepath.Join(ws.RunDir, "scenario-c")
	if err := os.Mkdir(projectC, 0o755); err != nil {
		return err
	}
	if err := applyInit(projectC, "diag-36d-charlie", fixed); err != nil {
		return fmt.Errorf("scenario C init: %w", err)
	}
	idC, err := home.ProjectID(projectC, "diag-36d-charlie")
	if err != nil {
		return err
	}
	candC := cand
	candC.Title = "Symlink failure canary"
	candC.TopicKey = "memory.diagnostics.symlink"
	candC.Provenance = "cap=e2e-36d;ref=symlink"
	resp, raw, err = runPropose(ctx, atlasBin, projectC, env, candC)
	if err != nil || resp.Status != memorypersist.StatusAccepted {
		return fmt.Errorf("scenario C propose status=%q err=%v raw=%s", resp.Status, err, redactPaths(raw))
	}
	dbRelC, err := memory.DatabaseRel(idC)
	if err != nil {
		return err
	}
	dbAbsC := filepath.Join(ws.AtlasHome, filepath.FromSlash(dbRelC))
	outside := filepath.Join(ws.RunDir, "outside-memory.db")
	if err := os.Rename(dbAbsC, outside); err != nil {
		return err
	}
	if err := os.Symlink(outside, dbAbsC); err != nil {
		return err
	}

	beforeC, err := fingerprintPair(projectC, ws.AtlasHome)
	if err != nil {
		return err
	}
	diagC, statusC, doctorC, err := runDiagnostics(projectC)
	if err != nil {
		return fmt.Errorf("scenario C diagnostics: %w", err)
	}
	afterC, err := fingerprintPair(projectC, ws.AtlasHome)
	if err != nil {
		return err
	}
	if beforeC != afterC {
		return fmt.Errorf("scenario C diagnostics mutated fingerprints")
	}
	if diagC.Status != memoryhealth.StatusFailed {
		return fmt.Errorf("scenario C want failed got %s", diagC.Status)
	}
	if !strings.Contains(statusC, "failed") {
		return fmt.Errorf("scenario C status missing failed:\n%s", redactPaths(statusC))
	}
	if !strings.Contains(doctorC, "symlink") && !strings.Contains(strings.ToLower(doctorC), "symlink") {
		// doctor message should mention symlink via mapped check
		found := false
		for _, c := range diagC.Checks {
			if c.ID == memoryhealth.CheckStorageSymlinkSafe {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("scenario C missing symlink check")
		}
	}
	results["C_failed"] = "PASS"

	// --- Scenario D: unavailable (initialized, no Memory DB yet) ---
	projectD := filepath.Join(ws.RunDir, "scenario-d")
	if err := os.Mkdir(projectD, 0o755); err != nil {
		return err
	}
	if err := applyInit(projectD, "diag-36d-delta", fixed); err != nil {
		return fmt.Errorf("scenario D init: %w", err)
	}
	diagD, statusD, _, err := runDiagnostics(projectD)
	if err != nil {
		return fmt.Errorf("scenario D diagnostics: %w", err)
	}
	if diagD.Status != memoryhealth.StatusUnavailable {
		return fmt.Errorf("scenario D want exactly unavailable got %s", diagD.Status)
	}
	if !strings.Contains(statusD, "unavailable") {
		return fmt.Errorf("scenario D status missing unavailable:\n%s", redactPaths(statusD))
	}
	results["D_unavailable"] = "PASS"

	// Isolation: A diagnostics must not count B/C observations via project id.
	if diagA.ProjectID == idB || diagA.Metrics.ObservationCount != 1 {
		return fmt.Errorf("isolation: A project_id=%s count=%d", diagA.ProjectID, diagA.Metrics.ObservationCount)
	}

	evidence := filepath.Join(ws.RunDir, "EVIDENCE.md")
	var b strings.Builder
	fmt.Fprintf(&b, "# 36D Memory Diagnostics E2E Evidence\n\n")
	fmt.Fprintf(&b, "- run_dir: %s\n", ws.RunDir)
	fmt.Fprintf(&b, "- ATLAS_HOME: %s\n", ws.AtlasHome)
	fmt.Fprintf(&b, "- project_a_id: %s\n", idA)
	fmt.Fprintf(&b, "- project_b_id: %s\n", idB)
	fmt.Fprintf(&b, "- project_c_id: %s\n", idC)
	fmt.Fprintf(&b, "- fingerprint_a_before_after_identical: PASS\n")
	fmt.Fprintf(&b, "- fingerprint_b_before_after_identical: PASS\n")
	fmt.Fprintf(&b, "- fingerprint_c_before_after_identical: PASS\n\n")
	fmt.Fprintf(&b, "## Strict scenario matrix\n\n")
	fmt.Fprintf(&b, "- A_healthy: exactly healthy\n")
	fmt.Fprintf(&b, "- B_degraded: exactly degraded + retrieval ready\n")
	fmt.Fprintf(&b, "- C_failed: exactly failed\n")
	fmt.Fprintf(&b, "- D_unavailable: exactly unavailable\n\n")
	fmt.Fprintf(&b, "## Results\n\n")
	for _, k := range []string{"A_healthy", "B_degraded", "C_failed", "D_unavailable"} {
		fmt.Fprintf(&b, "- %s: %s\n", k, results[k])
	}
	fmt.Fprintf(&b, "\n## Notes\n\n")
	fmt.Fprintf(&b, "- doctor/status are TUI routes; evidence uses the same inspect→doctor.Evaluate→screens render path as the TUI.\n")
	fmt.Fprintf(&b, "- diagnostics remain read-only (no DB create/migrate/repair).\n")
	fmt.Fprintf(&b, "- harness rejects semantic regressions (B≠failed, D≠degraded).\n")
	if err := os.WriteFile(evidence, []byte(b.String()), 0o644); err != nil {
		return err
	}

	fmt.Printf("e2e_root=%s\n", ws.Root)
	fmt.Printf("run_dir=%s\n", ws.RunDir)
	fmt.Printf("ATLAS_HOME=%s\n", ws.AtlasHome)
	fmt.Printf("evidence=%s\n", evidence)
	for _, k := range []string{"A_healthy", "B_degraded", "C_failed", "D_unavailable"} {
		fmt.Printf("%s=%s\n", k, results[k])
	}
	keepEvidence = true
	return nil
}

func applyInit(root, name string, fixed time.Time) error {
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      name,
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: true,
	})
	if !draft.SetValue("adapters.selected", "cursor,opencode") {
		return fmt.Errorf("set adapters.selected")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed },
	})
	return err
}

func runDiagnostics(projectRoot string) (memoryhealth.Diagnostics, string, string, error) {
	result, err := inspect.Inspect(projectRoot)
	if err != nil {
		return memoryhealth.Diagnostics{}, "", "", err
	}
	report := doctor.Evaluate(result)
	statusView := screens.StatusWithReport(result, report)
	doctorView := screens.Doctor(report, result)
	return result.Runtime.Memory, statusView, doctorView, nil
}

func fingerprintPair(projectRoot, atlasHome string) (string, error) {
	p, err := fingerprintTree(projectRoot)
	if err != nil {
		return "", err
	}
	h, err := fingerprintTree(atlasHome)
	if err != nil {
		return "", err
	}
	return p + "|" + h, nil
}

func runPropose(parent context.Context, atlasBin, dir string, env []string, c memorypersist.Candidate) (memorypersist.Response, string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return memorypersist.Response{}, "", err
	}
	rr, err := runBoundedWithStdin(parent, defaultCmdTimeout, maxStdoutCapture, maxStderrCapture, maxProposeStdin,
		atlasBin, []string{"memory", "propose", "--json"}, dir, env, payload)
	raw := string(rr.Stdout)
	var resp memorypersist.Response
	if jerr := json.Unmarshal(rr.Stdout, &resp); jerr != nil {
		if err != nil {
			return memorypersist.Response{}, raw, fmt.Errorf("%v", err)
		}
		return memorypersist.Response{}, raw, fmt.Errorf("decode propose json: %w", jerr)
	}
	if err != nil && memorypersist.ShouldExitNonZero(resp.Status) {
		return resp, raw, err
	}
	return resp, raw, nil
}
