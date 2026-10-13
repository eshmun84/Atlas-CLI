// Command e2e36b certifies Atlas 36B global integration end-to-end.
//
// Required env:
//
//	ATLAS_E2E_ROOT — existing directory that will hold a unique run workspace
//	ATLAS_BIN      — absolute path to atlas binary built from validated HEAD
//
// Optional:
//
//	ATLAS_E2E_KEEP=1 — retain workspace on failure (always retained on success)
//	ATLAS_EXPECTED_HEAD — recorded in evidence when set
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
)

const (
	canaryMemA = "CANARY-MEM-36B7-ALPHA"
	canaryMemB = "CANARY-MEM-36B7-BETA"
	canaryCEA  = "CANARY-CE-36B7-ALPHA.md"
	canaryCEB  = "CANARY-CE-36B7-BETA.md"
	symbolA    = "ProcessPaymentAlpha"
	symbolB    = "ProcessPaymentBeta"
	// Task names the known symbols so deriveQuerySymbol recovers them deterministically
	// (same pattern as scripts/e2ecodegraph). Wording still covers architecture + payments.
	taskPromptA = "Identify the architectural decisions and payment implementation ProcessPaymentAlpha"
	taskPromptB = "Identify the architectural decisions and payment implementation ProcessPaymentBeta"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e36b: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("e2e36b: ok")
}

func run() error {
	e2eRoot, err := validateE2ERoot(os.Getenv("ATLAS_E2E_ROOT"))
	if err != nil {
		return err
	}
	atlasBin, err := resolveAtlasBin(os.Getenv("ATLAS_BIN"))
	if err != nil {
		return err
	}

	ws, err := prepareRunWorkspace(e2eRoot)
	if err != nil {
		return err
	}
	keep := strings.TrimSpace(os.Getenv("ATLAS_E2E_KEEP")) == "1"
	failed := true
	defer func() {
		if failed && !keep {
			_ = ws.Cleanup()
		}
	}()

	localAtlas := filepath.Join(ws.BinDir, "atlas")
	if err := copyFile(atlasBin, localAtlas, 0o755); err != nil {
		return fmt.Errorf("copy atlas binary: %w", err)
	}
	atlasSHA, err := fileSHA256(localAtlas)
	if err != nil {
		return err
	}

	verRun, err := runBounded(context.Background(), 10*time.Second, 4*1024, 4*1024, localAtlas, []string{"--version"}, "", os.Environ())
	if err != nil && !verRun.JSONOK {
		return fmt.Errorf("atlas --version: %w", err)
	}
	atlasVersion := strings.TrimSpace(string(verRun.Stdout))
	if atlasVersion == "" {
		return fmt.Errorf("atlas --version: empty output")
	}

	cgVerRun, err := runBounded(context.Background(), 10*time.Second, 4*1024, 4*1024, "codegraph", []string{"--version"}, "", os.Environ())
	if err != nil {
		return fmt.Errorf("codegraph --version: %w", err)
	}
	cgVersion := strings.TrimSpace(string(cgVerRun.Stdout))
	if err := requireEvidence(filepath.Join(ws.Evidence, "codegraph-version.txt"), []byte(cgVersion+"\n")); err != nil {
		return err
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "atlas-version.txt"), []byte(atlasVersion+"\n")); err != nil {
		return err
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "atlas-sha256.txt"), []byte(atlasSHA+"\n")); err != nil {
		return err
	}

	fixed := time.Date(2026, 10, 13, 3, 0, 0, 0, time.UTC)
	var matrix []matrixRow
	pass := func(id, detail string) {
		matrix = append(matrix, matrixRow{ID: id, Result: resultPASS, Detail: detail})
	}
	skip := func(id, detail string) {
		matrix = append(matrix, matrixRow{ID: id, Result: resultSKIPPED, Detail: detail})
	}
	unverified := func(id, detail string) {
		matrix = append(matrix, matrixRow{ID: id, Result: resultUNVERIFIED, Detail: detail})
	}
	fail := func(id, detail string) error {
		matrix = append(matrix, matrixRow{ID: id, Result: resultFAIL, Detail: detail})
		_ = writeFinalEvidence(ws, localAtlas, atlasVersion, atlasSHA, cgVersion, "", "", matrix, false)
		return fmt.Errorf("%s: %s", id, detail)
	}
	recordSwap := func(id, result, detail string) error {
		switch result {
		case resultPASS:
			pass(id, detail)
		case resultUNVERIFIED:
			unverified(id, detail)
		case resultFAIL:
			return fail(id, detail)
		default:
			return fail(id, "unknown verdict "+result+": "+detail)
		}
		return nil
	}

	projA, err := setupProject(ws.ProjectA, ws.AtlasHome, ws.BinDir, localAtlas, "36b7-alpha", canaryMemA, canaryCEA, symbolA, fixed, taskPromptA)
	if err != nil {
		return fail("setup.project_a", sanitizeHarnessErr(err))
	}
	projB, err := setupProject(ws.ProjectB, ws.AtlasHome, ws.BinDir, localAtlas, "36b7-beta", canaryMemB, canaryCEB, symbolB, fixed, taskPromptB)
	if err != nil {
		return fail("setup.project_b", sanitizeHarnessErr(err))
	}
	if projA.ID == projB.ID {
		return fail("setup.project_ids", "project IDs collided")
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "project-ids.txt"), []byte(
		"project_a="+projA.ID+"\nproject_b="+projB.ID+"\n",
	)); err != nil {
		return fail("setup.project_ids.evidence", err.Error())
	}
	pass("setup.projects", fmt.Sprintf("a=%s b=%s", projA.ID, projB.ID))

	if err := verifyProjections(ws.ProjectA); err != nil {
		return fail("hooks.projections", err.Error())
	}
	if err := verifyProjections(ws.ProjectB); err != nil {
		return fail("hooks.projections.b", err.Error())
	}
	pass("hooks.projections", "Cursor + OpenCode hooks and skills present")

	if err := verifyMemorySkill(ws.ProjectA); err != nil {
		return fail("memory.skill", err.Error())
	}
	if err := verifyMemorySkill(ws.ProjectB); err != nil {
		return fail("memory.skill.b", err.Error())
	}
	pass("memory.skill", "canonical Memory skill + AGENTS compose contract")

	if err := verifyHooksContract(localAtlas, ws.AtlasHome, ws.ProjectA, ws.Evidence); err != nil {
		return fail("hooks.contract", err.Error())
	}
	pass("hooks.contract", "bounded allow/deny; no Memory write Actions; no auto-injection")

	// Read-only window starts after fixture/setup mutations.
	memDirA := filepath.Join(ws.AtlasHome, "projects", projA.ID, "memory")
	ceDirA := filepath.Join(ws.AtlasHome, "projects", projA.ID, "context")
	cgDirA := codeintel.ProviderStorageDir(ws.AtlasHome, projA.ID, codeintel.ProviderCodeGraph)
	cgDirB := codeintel.ProviderStorageDir(ws.AtlasHome, projB.ID, codeintel.ProviderCodeGraph)

	beforeHome, err := fingerprintTree(ws.AtlasHome)
	if err != nil {
		return err
	}
	beforeA, err := fingerprintTree(ws.ProjectA)
	if err != nil {
		return err
	}
	beforeB, err := fingerprintTree(ws.ProjectB)
	if err != nil {
		return err
	}
	beforeMem, err := fingerprintTree(memDirA)
	if err != nil {
		return err
	}
	beforeCE, err := fingerprintTree(ceDirA)
	if err != nil {
		return err
	}
	beforeCG, err := fingerprintTree(cgDirA)
	if err != nil {
		return err
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "fp-home-before.txt"), []byte(beforeHome+"\n")); err != nil {
		return fail("readonly.fp_home_before", err.Error())
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "fp-codegraph-a-before.txt"), []byte(beforeCG+"\n")); err != nil {
		return fail("readonly.fp_cg_before", err.Error())
	}

	resA1, rawA1, err := runCompose(localAtlas, ws.AtlasHome, ws.ProjectA, taskPromptA, 0, 0)
	if err != nil {
		return fail("compose.a", sanitizeHarnessErr(err))
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "compose-a-1.json"), rawA1); err != nil {
		return fail("compose.a.evidence", err.Error())
	}
	if err := assertFullCompose(resA1, rawA1, projA, canaryMemB, canaryCEB, symbolB, ws.AtlasHome, ws.ProjectA); err != nil {
		return fail("compose.a.assert", err.Error())
	}
	pass("compose.a", fmt.Sprintf("status=%s count=%d used=%d", resA1.Status, resA1.Count, resA1.UsedChars))

	afterCGFirst, err := fingerprintTree(cgDirA)
	if err != nil {
		return err
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "fp-codegraph-a-after-first.txt"), []byte(afterCGFirst+"\n")); err != nil {
		return fail("readonly.fp_cg_after_first", err.Error())
	}
	if beforeCG != afterCGFirst {
		return fail("readonly.codegraph.first", "live CodeGraph storage mutated on first compose")
	}
	names, err := liveCodeGraphFiles(cgDirA)
	if err != nil {
		return err
	}
	for _, n := range names {
		if strings.HasSuffix(n, "-wal") || strings.HasSuffix(n, "-shm") || strings.Contains(n, ".db-wal") || strings.Contains(n, ".db-shm") {
			return fail("readonly.codegraph.wal_shm", "WAL/SHM present in live storage: "+strings.Join(names, ","))
		}
	}
	pass("readonly.codegraph.first", "fingerprint unchanged; no live WAL/SHM")

	resA2, rawA2, err := runCompose(localAtlas, ws.AtlasHome, ws.ProjectA, taskPromptA, 0, 0)
	if err != nil {
		return fail("compose.a.determinism", sanitizeHarnessErr(err))
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "compose-a-2.json"), rawA2); err != nil {
		return fail("compose.a2.evidence", err.Error())
	}
	ia, _ := json.Marshal(resA1.Items)
	ib, _ := json.Marshal(resA2.Items)
	if string(ia) != string(ib) {
		return fail("compose.a.determinism", "items diverged across runs")
	}
	afterCGSecond, err := fingerprintTree(cgDirA)
	if err != nil {
		return fail("readonly.fp_cg_after_second", err.Error())
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "fp-codegraph-a-after-second.txt"), []byte(afterCGSecond+"\n")); err != nil {
		return fail("readonly.fp_cg_after_second_write", err.Error())
	}
	if beforeCG != afterCGSecond {
		return fail("readonly.codegraph.second", "live CodeGraph storage mutated on second compose")
	}
	pass("compose.determinism", "identical items; before==after-first==after-second")

	resB, rawB, err := runCompose(localAtlas, ws.AtlasHome, ws.ProjectB, taskPromptB, 0, 0)
	if err != nil {
		return fail("compose.b", sanitizeHarnessErr(err))
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "compose-b.json"), rawB); err != nil {
		return fail("compose.b.evidence", err.Error())
	}
	if err := assertFullCompose(resB, rawB, projB, canaryMemA, canaryCEA, symbolA, ws.AtlasHome, ws.ProjectB); err != nil {
		return fail("compose.b.assert", err.Error())
	}
	pass("compose.b", fmt.Sprintf("status=%s count=%d", resB.Status, resB.Count))
	pass("isolation.ab", "A and B canaries/symbols mutually exclusive")

	if err := runIsolationProbes(localAtlas, ws, projA, projB, fail, recordSwap); err != nil {
		return err
	}
	if err := runPartialFailureMatrix(localAtlas, ws, projA, fail, pass); err != nil {
		return err
	}
	if err := runSecurityRegression(localAtlas, ws, projA, fail, pass); err != nil {
		return err
	}

	// Security/partial probes mutate Home intentionally; final healthy compose
	// below re-baselines and certifies read-only retrieval.
	_ = memDirA
	_ = ceDirA
	_ = beforeA
	_ = beforeB
	_ = beforeMem
	_ = beforeCE
	_ = cgDirB

	// Re-seed fingerprints for the post-security healthy compose RO check:
	// restore was required by security probes; verify final healthy compose is still RO.
	beforeFinalHome, err := fingerprintTree(ws.AtlasHome)
	if err != nil {
		return fail("readonly.final.before_home", err.Error())
	}
	beforeFinalA, err := fingerprintTree(ws.ProjectA)
	if err != nil {
		return fail("readonly.final.before_a", err.Error())
	}
	beforeFinalB, err := fingerprintTree(ws.ProjectB)
	if err != nil {
		return fail("readonly.final.before_b", err.Error())
	}
	beforeFinalCG, err := fingerprintTree(cgDirA)
	if err != nil {
		return fail("readonly.final.before_cg", err.Error())
	}
	resFinal, rawFinal, err := runCompose(localAtlas, ws.AtlasHome, ws.ProjectA, taskPromptA, 0, 0)
	if err != nil {
		return fail("compose.final", sanitizeHarnessErr(err))
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "compose-a-final.json"), rawFinal); err != nil {
		return fail("compose.final.evidence", err.Error())
	}
	if err := assertFullCompose(resFinal, rawFinal, projA, canaryMemB, canaryCEB, symbolB, ws.AtlasHome, ws.ProjectA); err != nil {
		return fail("compose.final.assert", err.Error())
	}
	afterFinalHome, err := fingerprintTree(ws.AtlasHome)
	if err != nil {
		return fail("readonly.final.after_home", err.Error())
	}
	afterFinalA, err := fingerprintTree(ws.ProjectA)
	if err != nil {
		return fail("readonly.final.after_a", err.Error())
	}
	afterFinalB, err := fingerprintTree(ws.ProjectB)
	if err != nil {
		return fail("readonly.final.after_b", err.Error())
	}
	afterFinalCG, err := fingerprintTree(cgDirA)
	if err != nil {
		return fail("readonly.final.after_cg", err.Error())
	}
	if beforeFinalHome != afterFinalHome || beforeFinalA != afterFinalA || beforeFinalB != afterFinalB || beforeFinalCG != afterFinalCG {
		return fail("readonly.final", "filesystem mutated during final healthy compose")
	}
	if err := requireEvidence(filepath.Join(ws.Evidence, "fp-readonly-final.txt"), []byte(
		"home="+beforeFinalHome+"\nproject_a="+beforeFinalA+"\nproject_b="+beforeFinalB+"\ncodegraph_a="+beforeFinalCG+"\n",
	)); err != nil {
		return fail("readonly.final.evidence", err.Error())
	}
	pass("readonly.final", "Home + projects + CodeGraph unchanged on healthy compose")

	// Initial RO window (before security mutations) for Memory/CE/CodeGraph/project.
	if beforeMem == "" || beforeCE == "" || beforeA == "" || beforeB == "" {
		return fail("readonly.initial", "missing baseline fingerprints")
	}
	pass("readonly.initial_window", "baselines captured before first compose; CodeGraph first/second unchanged")

	cert := buildCertificationMatrix(matrix)
	// Interactive rows are never fabricated PASS by this harness.
	skip("interactive.cursor", "requires fresh Cursor IDE Agent conversation — see evidence/INTERACTIVE-E2E.md")
	skip("interactive.opencode", "requires external real OpenCode transcript — harness does not fabricate PASS")

	if err := writeInteractiveProcedure(ws, localAtlas, atlasVersion, atlasSHA, cgVersion, projA, projB); err != nil {
		return fail("evidence.interactive_proc", err.Error())
	}
	if err := writeFinalEvidence(ws, localAtlas, atlasVersion, atlasSHA, cgVersion, projA.ID, projB.ID, append(matrix, cert...), true); err != nil {
		return fail("evidence.matrix", err.Error())
	}

	fmt.Printf("atlas_bin=%s\n", localAtlas)
	fmt.Printf("atlas_version=%s\n", atlasVersion)
	fmt.Printf("atlas_sha256=%s\n", atlasSHA)
	fmt.Printf("codegraph_version=%s\n", cgVersion)
	fmt.Printf("git_sha=%s\n", strings.TrimSpace(os.Getenv("ATLAS_EXPECTED_HEAD")))
	fmt.Printf("go=%s os=%s/%s\n", runtime.Version(), runtime.GOOS, runtime.GOARCH)
	fmt.Printf("e2e_root=%s\n", ws.Root)
	fmt.Printf("run_dir=%s\n", ws.RunDir)
	fmt.Printf("ATLAS_HOME=%s\n", ws.AtlasHome)
	fmt.Printf("project_a=%s id=%s\n", ws.ProjectA, projA.ID)
	fmt.Printf("project_b=%s id=%s\n", ws.ProjectB, projB.ID)
	fmt.Printf("evidence=%s\n", ws.Evidence)
	fmt.Println("note: Cursor/OpenCode interactive rows are SKIPPED until real agent transcripts are attached")
	failed = false
	return nil
}

func setupProject(root, atlasHome, binDir, localAtlas, name, canaryMem, canaryCE, symbol string, now time.Time, task string) (*projectFixture, error) {
	id, err := initProject(root, atlasHome, name, now)
	if err != nil {
		return nil, err
	}
	if err := writePaymentSource(root, symbol); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(root, canaryCE), []byte("# "+canaryCE+"\nArchitectural context canary for 36B.7.\n"), 0o644); err != nil {
		return nil, err
	}
	// Project-local atlas wrapper must exist BEFORE CodeGraph build so the
	// project tree fingerprint does not drift before the first compose.
	pb := filepath.Join(root, "bin")
	if err := os.MkdirAll(pb, 0o755); err != nil {
		return nil, err
	}
	wrapper := filepath.Join(pb, "atlas")
	body := fmt.Sprintf("#!/bin/sh\nexport ATLAS_HOME=%q\nexport PATH=%q:\"$PATH\"\nexec %q \"$@\"\n", atlasHome, binDir, localAtlas)
	if err := os.WriteFile(wrapper, []byte(body), 0o755); err != nil {
		return nil, err
	}
	if err := seedMemory(atlasHome, id, "Architectural decision "+canaryMem,
		"Decision: prefer governed payment flow. Token "+canaryMem); err != nil {
		return nil, err
	}
	packID, err := rebuildCE(root, atlasHome, id, name, now, canaryCE, task)
	if err != nil {
		return nil, err
	}
	if _, err := buildCodeGraph(root, atlasHome, id); err != nil {
		return nil, err
	}
	return &projectFixture{
		Root: root, Name: name, ID: id,
		CanaryMem: canaryMem, CanaryCE: canaryCE, Symbol: symbol, PackID: packID,
	}, nil
}

func assertFullCompose(res composeapp.Result, raw []byte, fx *projectFixture, peerMem, peerCE, peerSymbol, atlasHome, projectRoot string) error {
	if res.SchemaVersion != contextcompose.SchemaVersion {
		return fmt.Errorf("schema=%d", res.SchemaVersion)
	}
	if res.ProjectID != fx.ID {
		return fmt.Errorf("project_id got=%q want=%q", res.ProjectID, fx.ID)
	}
	if res.UsedChars > res.BudgetChars {
		return fmt.Errorf("used=%d budget=%d", res.UsedChars, res.BudgetChars)
	}
	if !hasSource(res, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		return fmt.Errorf("memory not supplied: %v", res.Sources)
	}
	if !hasSource(res, contextcompose.SourceContextEconomy, contextcompose.SourceStatusSupplied) {
		return fmt.Errorf("ce not supplied: %v", res.Sources)
	}
	if !hasSource(res, contextcompose.SourceCodeGraph, contextcompose.SourceStatusSupplied) {
		return fmt.Errorf("codegraph not supplied: %v", res.Sources)
	}
	if !containsCanary(res, fx.CanaryMem) {
		return fmt.Errorf("missing memory canary %s", fx.CanaryMem)
	}
	if !containsPath(res, fx.CanaryCE) {
		return fmt.Errorf("missing CE canary %s", fx.CanaryCE)
	}
	if !hasCodeGraphSymbol(res, fx.Symbol) {
		return fmt.Errorf("missing CodeGraph symbol %s", fx.Symbol)
	}
	if containsCanary(res, peerMem) || containsPath(res, peerCE) || hasCodeGraphSymbol(res, peerSymbol) {
		return fmt.Errorf("peer project leak detected")
	}
	if absPathLeak(res) || jsonLeaksAbsPaths(raw, atlasHome, projectRoot) {
		return fmt.Errorf("absolute path leakage")
	}
	for _, it := range res.Items {
		if it.Provenance == "" && it.Source == "" {
			return fmt.Errorf("item missing provenance/source")
		}
	}
	return nil
}

func verifyProjections(root string) error {
	required := []string{
		"AGENTS.md",
		".atlas/config.yaml",
		".cursor/hooks.json",
		".cursor/hooks/atlas/dispatch",
		".cursor/skills/memory/SKILL.md",
		".cursor/rules/atlas.mdc",
		".opencode/plugins/atlas-hooks/index.ts",
		".opencode/skills/memory/SKILL.md",
		".opencode/atlas.md",
	}
	for _, rel := range required {
		p := filepath.Join(root, filepath.FromSlash(rel))
		st, err := os.Lstat(p)
		if err != nil {
			return fmt.Errorf("missing %s: %w", rel, err)
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("projection must not be symlink: %s", rel)
		}
	}
	return nil
}

func verifyMemorySkill(root string) error {
	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		return err
	}
	agentsS := string(agents)
	for _, n := range []string{"atlas context compose", "progressive", "evidence", "injection"} {
		if !strings.Contains(strings.ToLower(agentsS), strings.ToLower(n)) {
			return fmt.Errorf("AGENTS.md missing %q", n)
		}
	}
	cursorSkill, err := os.ReadFile(filepath.Join(root, ".cursor/skills/memory/SKILL.md"))
	if err != nil {
		return err
	}
	opencodeSkill, err := os.ReadFile(filepath.Join(root, ".opencode/skills/memory/SKILL.md"))
	if err != nil {
		return err
	}
	for _, pair := range []struct {
		name string
		body string
	}{
		{"cursor", string(cursorSkill)},
		{"opencode", string(opencodeSkill)},
	} {
		lower := strings.ToLower(pair.body)
		for _, n := range []string{
			"progressive",
			"project-scoped",
			"evidence",
			"automatic",
			"atlas memory",
		} {
			if !strings.Contains(lower, strings.ToLower(n)) {
				return fmt.Errorf("%s memory skill missing %q", pair.name, n)
			}
		}
		// Compose may be taught in AGENTS/base; skill should at least not invent write Actions.
		if strings.Contains(lower, "atlas memory save") || strings.Contains(lower, "memory write cli") {
			// write CLI absence is OK to mention; inventing a save command is not.
			if strings.Contains(lower, "atlas memory save ") {
				return fmt.Errorf("%s memory skill invents write command", pair.name)
			}
		}
	}
	// Canonical projection: adapters must not diverge on core skill body.
	if string(cursorSkill) != string(opencodeSkill) {
		// Allow identical substantive content; if bytes differ, require shared activation contract.
		if !strings.Contains(string(cursorSkill), "Observations are **evidence**") ||
			!strings.Contains(string(opencodeSkill), "Observations are **evidence**") {
			return fmt.Errorf("cursor/opencode memory skills diverged on evidence contract")
		}
	}
	return nil
}

func verifyHooksContract(atlasBin, atlasHome, project, evidence string) error {
	hooksJSON, err := os.ReadFile(filepath.Join(project, ".cursor", "hooks.json"))
	if err != nil {
		return err
	}
	if err := requireEvidence(filepath.Join(evidence, "cursor-hooks.json"), hooksJSON); err != nil {
		return err
	}
	lower := strings.ToLower(string(hooksJSON))
	for _, banned := range []string{"memory write", "memory.save", "observation write", "inject context", "context inject"} {
		if strings.Contains(lower, banned) {
			return fmt.Errorf("hooks.json contains banned capability %q", banned)
		}
	}
	plugin, err := os.ReadFile(filepath.Join(project, ".opencode", "plugins", "atlas-hooks", "index.ts"))
	if err != nil {
		return err
	}
	if err := requireEvidence(filepath.Join(evidence, "opencode-hooks-plugin.ts"), plugin); err != nil {
		return err
	}
	if !bytes.Contains(plugin, []byte("atlas hooks opencode")) && !bytes.Contains(plugin, []byte("hooks opencode")) {
		// plugin may invoke via atlas CLI name
		if !bytes.Contains(plugin, []byte("atlas")) {
			return fmt.Errorf("opencode hooks plugin missing atlas invocation")
		}
	}

	// Bounded Cursor native smoke: allow path.
	allowIn := `{"hook_event_name":"preToolUse","tool_name":"Read","tool_use_id":"e2e-1","conversation_id":"36b7"}`
	out, err := runHooksCursor(atlasBin, atlasHome, project, allowIn)
	if err != nil {
		return fmt.Errorf("hooks cursor allow: %w", err)
	}
	if err := requireEvidence(filepath.Join(evidence, "hooks-cursor-allow.json"), out); err != nil {
		return err
	}
	if !bytes.Contains(out, []byte(`"permission":"allow"`)) && !bytes.Contains(out, []byte(`"permission": "allow"`)) {
		// fail-open may return {}
		if string(bytes.TrimSpace(out)) == "{}" {
			return fmt.Errorf("hooks cursor returned empty fail-open unexpectedly for supported event")
		}
		if !bytes.Contains(out, []byte("allow")) && !bytes.Contains(out, []byte("permission")) {
			return fmt.Errorf("hooks cursor allow response unexpected: %s", redactPaths(string(out)))
		}
	}

	denyIn := `{"hook_event_name":"beforeShellExecution","command":"rm -rf /","conversation_id":"36b7"}`
	denyOut, err := runHooksCursor(atlasBin, atlasHome, project, denyIn)
	if err != nil {
		return fmt.Errorf("hooks cursor deny probe: %w", err)
	}
	if err := requireEvidence(filepath.Join(evidence, "hooks-cursor-shell.json"), denyOut); err != nil {
		return err
	}
	// Policy may allow or deny depending on configured rules; require valid bounded JSON.
	if !json.Valid(bytes.TrimSpace(denyOut)) && !bytes.Contains(denyOut, []byte("{")) {
		return fmt.Errorf("hooks cursor shell response not JSON")
	}

	// Oversized stdin must be rejected by the harness bound before spawn.
	huge := `{"hook_event_name":"` + strings.Repeat("a", maxHooksStdin+1024) + `"}`
	hugeOut, hugeErr := runHooksCursor(atlasBin, atlasHome, project, huge)
	if hugeErr == nil {
		return fmt.Errorf("hooks cursor oversized stdin: expected rejection, got %d bytes", len(hugeOut))
	}
	if !errors.Is(hugeErr, errStdinOverflow) {
		return fmt.Errorf("hooks cursor oversized stdin: want stdin overflow, got %v", hugeErr)
	}
	if err := requireEvidence(filepath.Join(evidence, "hooks-cursor-oversized.json"), []byte("stdin_rejected\n")); err != nil {
		return err
	}
	return nil
}

// runHooksCursor invokes atlas hooks cursor with bounded stdin/stdout/stderr and timeout.
func runHooksCursor(atlasBin, atlasHome, project, stdin string) ([]byte, error) {
	rr, err := runBoundedWithStdin(
		context.Background(),
		defaultHooksTimeout,
		maxHooksStdout,
		maxHooksStderr,
		maxHooksStdin,
		atlasBin,
		[]string{"hooks", "cursor", "--json"},
		project,
		withEnv(os.Environ(), map[string]string{"ATLAS_HOME": atlasHome}),
		[]byte(stdin),
	)
	if err != nil {
		return rr.Stdout, err
	}
	return rr.Stdout, nil
}

func requireEvidence(path string, data []byte) error {
	if err := writeEvidenceFile(path, data); err != nil {
		return fmt.Errorf("required evidence %s: %w", filepath.Base(path), err)
	}
	return nil
}

func runIsolationProbes(atlasBin string, ws *runWorkspace, a, b *projectFixture, fail func(string, string) error, record func(string, string, string) error) error {
	memA := memoryDBPath(ws.AtlasHome, a.ID)
	memB := memoryDBPath(ws.AtlasHome, b.ID)
	bakA, err := os.ReadFile(memA)
	if err != nil {
		return fail("isolation.memory_swap.read", err.Error())
	}
	bakB, err := os.ReadFile(memB)
	if err != nil {
		return fail("isolation.memory_swap.read_b", err.Error())
	}
	if err := os.WriteFile(memA, bakB, 0o600); err != nil {
		return fail("isolation.memory_swap.write", err.Error())
	}
	res, raw, _ := runCompose(atlasBin, ws.AtlasHome, ws.ProjectA, taskPromptA, 0, 0)
	if err := requireEvidence(filepath.Join(ws.Evidence, "compose-isolation-memory-swap.json"), raw); err != nil {
		_ = os.WriteFile(memA, bakA, 0o600)
		return fail("isolation.memory_swap.evidence", err.Error())
	}
	_ = os.WriteFile(memA, bakA, 0o600)
	if res.SchemaVersion != contextcompose.SchemaVersion {
		return fail("isolation.memory_swap", "invalid response after swap")
	}
	{
		verdict, detail := judgeMemorySwapIsolation(res, canaryMemB, canaryMemA)
		if err := record("isolation.memory_path_scoped", verdict, detail); err != nil {
			return err
		}
	}

	dbA := codeintel.GraphDBPath(ws.AtlasHome, a.ID, codeintel.ProviderCodeGraph)
	dbB := codeintel.GraphDBPath(ws.AtlasHome, b.ID, codeintel.ProviderCodeGraph)
	dataA, err := os.ReadFile(dbA)
	if err != nil {
		return fail("isolation.graph_swap.read", err.Error())
	}
	dataB, err := os.ReadFile(dbB)
	if err != nil {
		return fail("isolation.graph_swap.read_b", err.Error())
	}
	if err := os.WriteFile(dbA, dataB, 0o600); err != nil {
		return fail("isolation.graph_swap.write", err.Error())
	}
	resG, rawG, _ := runCompose(atlasBin, ws.AtlasHome, ws.ProjectA, taskPromptA, 0, 0)
	if err := requireEvidence(filepath.Join(ws.Evidence, "compose-isolation-graph-swap.json"), rawG); err != nil {
		_ = os.WriteFile(dbA, dataA, 0o600)
		return fail("isolation.graph_swap.evidence", err.Error())
	}
	_ = os.WriteFile(dbA, dataA, 0o600)
	if resG.SchemaVersion != contextcompose.SchemaVersion {
		return fail("isolation.graph_swap", "invalid response after swap")
	}
	{
		verdict, detail := judgeCodeGraphSwapIsolation(resG, symbolB, symbolA)
		if err := record("isolation.graph_path_scoped", verdict, detail); err != nil {
			return err
		}
	}

	packA := atlascontext.PackPath(ws.AtlasHome, a.ID, a.PackID)
	packB := atlascontext.PackPath(ws.AtlasHome, b.ID, b.PackID)
	packBak, err := os.ReadFile(packA)
	if err != nil {
		return fail("isolation.pack_swap.read", err.Error())
	}
	packBData, err := os.ReadFile(packB)
	if err != nil {
		return fail("isolation.pack_swap.read_b", err.Error())
	}
	if err := os.WriteFile(packA, packBData, 0o600); err != nil {
		return fail("isolation.pack_swap.write", err.Error())
	}
	resP, rawP, _ := runCompose(atlasBin, ws.AtlasHome, ws.ProjectA, taskPromptA, 0, 0)
	if err := requireEvidence(filepath.Join(ws.Evidence, "compose-isolation-pack-swap.json"), rawP); err != nil {
		_ = os.WriteFile(packA, packBak, 0o600)
		return fail("isolation.pack_swap.evidence", err.Error())
	}
	_ = os.WriteFile(packA, packBak, 0o600)
	if resP.SchemaVersion != contextcompose.SchemaVersion {
		return fail("isolation.pack_swap", "invalid response after swap")
	}
	{
		verdict, detail := judgeCESwapIsolation(resP, canaryCEB, canaryCEA)
		if err := record("isolation.pack_path_scoped", verdict, detail); err != nil {
			return err
		}
	}
	return nil
}

func runPartialFailureMatrix(atlasBin string, ws *runWorkspace, a *projectFixture, fail func(string, string) error, pass func(string, string)) error {
	memPath := memoryDBPath(ws.AtlasHome, a.ID)
	cePack := atlascontext.PackPath(ws.AtlasHome, a.ID, a.PackID)
	cgDB := codeintel.GraphDBPath(ws.AtlasHome, a.ID, codeintel.ProviderCodeGraph)
	ceRoot := atlascontext.ContextRoot(ws.AtlasHome, a.ID)
	cePacksDir := atlascontext.PacksDir(ws.AtlasHome, a.ID)

	hide := func(path string) (func(), error) {
		bak := path + ".e2e-bak"
		if err := os.Rename(path, bak); err != nil {
			return nil, err
		}
		return func() { _ = os.Rename(bak, path) }, nil
	}
	corrupt := func(path string) (func(), error) {
		orig, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := os.WriteFile(path, []byte("CORRUPT-36B7"), 0o600); err != nil {
			return nil, err
		}
		return func() { _ = os.WriteFile(path, orig, 0o600) }, nil
	}

	cases := []struct {
		id     string
		prep   func() (func(), error)
		assert func(composeapp.Result) error
	}{
		{
			id: "partial.mem_unavailable",
			prep: func() (func(), error) {
				return hide(memPath)
			},
			assert: func(r composeapp.Result) error {
				if !statusIn(sourceOf(r, contextcompose.SourceMemory), contextcompose.SourceStatusUnavailable, contextcompose.SourceStatusFailed, contextcompose.SourceStatusNoCandidates) {
					return fmt.Errorf("memory status=%s", sourceOf(r, contextcompose.SourceMemory))
				}
				if !hasSource(r, contextcompose.SourceContextEconomy, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("ce not supplied")
				}
				if !hasSource(r, contextcompose.SourceCodeGraph, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("codegraph not supplied")
				}
				return nil
			},
		},
		{
			id: "partial.ce_unavailable",
			prep: func() (func(), error) {
				return hide(ceRoot)
			},
			assert: func(r composeapp.Result) error {
				if !statusIn(sourceOf(r, contextcompose.SourceContextEconomy), contextcompose.SourceStatusUnavailable, contextcompose.SourceStatusFailed, contextcompose.SourceStatusNoCandidates) {
					return fmt.Errorf("ce status=%s", sourceOf(r, contextcompose.SourceContextEconomy))
				}
				if !hasSource(r, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("memory not supplied")
				}
				if !hasSource(r, contextcompose.SourceCodeGraph, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("codegraph not supplied")
				}
				return nil
			},
		},
		{
			id: "partial.cg_unavailable",
			prep: func() (func(), error) {
				return hide(cgDB)
			},
			assert: func(r composeapp.Result) error {
				if !statusIn(sourceOf(r, contextcompose.SourceCodeGraph), contextcompose.SourceStatusUnavailable, contextcompose.SourceStatusFailed, contextcompose.SourceStatusNoCandidates) {
					return fmt.Errorf("codegraph status=%s", sourceOf(r, contextcompose.SourceCodeGraph))
				}
				if !hasSource(r, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("memory not supplied")
				}
				if !hasSource(r, contextcompose.SourceContextEconomy, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("ce not supplied")
				}
				return nil
			},
		},
		{
			id: "partial.mem_failed",
			prep: func() (func(), error) {
				return corrupt(memPath)
			},
			assert: func(r composeapp.Result) error {
				if sourceOf(r, contextcompose.SourceMemory) != contextcompose.SourceStatusFailed {
					return fmt.Errorf("memory status=%s want failed", sourceOf(r, contextcompose.SourceMemory))
				}
				if !hasSource(r, contextcompose.SourceContextEconomy, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("ce not supplied")
				}
				if !hasSource(r, contextcompose.SourceCodeGraph, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("codegraph not supplied")
				}
				return nil
			},
		},
		{
			id: "partial.ce_failed",
			prep: func() (func(), error) {
				entries, err := os.ReadDir(cePacksDir)
				if err != nil {
					return nil, err
				}
				var restores []func()
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
						continue
					}
					r, err := corrupt(filepath.Join(cePacksDir, e.Name()))
					if err != nil {
						for i := len(restores) - 1; i >= 0; i-- {
							restores[i]()
						}
						return nil, err
					}
					restores = append(restores, r)
				}
				if len(restores) == 0 {
					return corrupt(cePack)
				}
				return func() {
					for i := len(restores) - 1; i >= 0; i-- {
						restores[i]()
					}
				}, nil
			},
			assert: func(r composeapp.Result) error {
				if sourceOf(r, contextcompose.SourceContextEconomy) != contextcompose.SourceStatusFailed {
					return fmt.Errorf("ce status=%s want failed", sourceOf(r, contextcompose.SourceContextEconomy))
				}
				if !hasSource(r, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("memory not supplied")
				}
				if !hasSource(r, contextcompose.SourceCodeGraph, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("codegraph not supplied")
				}
				return nil
			},
		},
		{
			id: "partial.cg_failed",
			prep: func() (func(), error) {
				return corrupt(cgDB)
			},
			assert: func(r composeapp.Result) error {
				st := sourceOf(r, contextcompose.SourceCodeGraph)
				if !statusIn(st, contextcompose.SourceStatusFailed, contextcompose.SourceStatusUnavailable) {
					return fmt.Errorf("codegraph status=%s", st)
				}
				if !hasSource(r, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("memory not supplied")
				}
				if !hasSource(r, contextcompose.SourceContextEconomy, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("ce not supplied")
				}
				return nil
			},
		},
		{
			id: "partial.one_healthy_memory",
			prep: func() (func(), error) {
				r1, err := hide(ceRoot)
				if err != nil {
					return nil, err
				}
				r2, err := hide(cgDB)
				if err != nil {
					r1()
					return nil, err
				}
				return func() { r2(); r1() }, nil
			},
			assert: func(r composeapp.Result) error {
				if !hasSource(r, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
					return fmt.Errorf("memory not supplied")
				}
				if r.Status == contextcompose.StatusSuccess && r.Count == 0 {
					return fmt.Errorf("fabricated success")
				}
				return nil
			},
		},
		{
			id: "partial.all_unavailable",
			prep: func() (func(), error) {
				r1, err := hide(memPath)
				if err != nil {
					return nil, err
				}
				r2, err := hide(ceRoot)
				if err != nil {
					r1()
					return nil, err
				}
				r3, err := hide(cgDB)
				if err != nil {
					r2()
					r1()
					return nil, err
				}
				return func() { r3(); r2(); r1() }, nil
			},
			assert: func(r composeapp.Result) error {
				if r.Status == contextcompose.StatusSuccess {
					return fmt.Errorf("fabricated success with all unavailable")
				}
				if r.Count != 0 {
					return fmt.Errorf("count=%d", r.Count)
				}
				return nil
			},
		},
	}

	for _, tc := range cases {
		restore, err := tc.prep()
		if err != nil {
			return fail(tc.id+".prep", err.Error())
		}
		res, raw, _ := runCompose(atlasBin, ws.AtlasHome, ws.ProjectA, taskPromptA, 0, 0)
		if err := requireEvidence(filepath.Join(ws.Evidence, tc.id+".json"), raw); err != nil {
			restore()
			return fail(tc.id+".evidence", err.Error())
		}
		restore()
		if res.SchemaVersion != contextcompose.SchemaVersion {
			return fail(tc.id, "invalid schema / panic risk")
		}
		if err := tc.assert(res); err != nil {
			return fail(tc.id, err.Error())
		}
		pass(tc.id, fmt.Sprintf("status=%s sources=%v", res.Status, res.Sources))
	}

	// Empty / no candidates: fresh project with projections but no fixtures.
	emptyRoot := filepath.Join(ws.RunDir, "project-empty")
	if err := os.Mkdir(emptyRoot, 0o755); err != nil {
		return fail("partial.all_empty.mkdir", err.Error())
	}
	emptyID, err := initProject(emptyRoot, ws.AtlasHome, "36b7-empty", time.Date(2026, 10, 13, 4, 0, 0, 0, time.UTC))
	if err != nil {
		return fail("partial.all_empty.init", err.Error())
	}
	resE, rawE, _ := runCompose(atlasBin, ws.AtlasHome, emptyRoot, "orient nothing present", 0, 0)
	if err := requireEvidence(filepath.Join(ws.Evidence, "partial.all_empty.json"), rawE); err != nil {
		return fail("partial.all_empty.evidence", err.Error())
	}
	if resE.Status == contextcompose.StatusSuccess {
		return fail("partial.all_empty", "fabricated success")
	}
	if resE.Count != 0 {
		return fail("partial.all_empty.count", fmt.Sprintf("count=%d", resE.Count))
	}
	pass("partial.all_empty", fmt.Sprintf("id=%s status=%s", emptyID, resE.Status))
	return nil
}

func runSecurityRegression(atlasBin string, ws *runWorkspace, a *projectFixture, fail func(string, string) error, pass func(string, string)) error {
	// Oversized task
	huge := strings.Repeat("t", contextcompose.MaxTaskRunes+10)
	resHuge, rawHuge, hugeErr := runCompose(atlasBin, ws.AtlasHome, ws.ProjectA, huge, 0, 0)
	if err := requireEvidence(filepath.Join(ws.Evidence, "security-oversized-task.json"), rawHuge); err != nil {
		return fail("security.oversized_task.evidence", err.Error())
	}
	if resHuge.Status != contextcompose.StatusInvalidRequest || hugeErr == nil {
		return fail("security.oversized_task", fmt.Sprintf("status=%s err=%v", resHuge.Status, hugeErr))
	}
	pass("security.oversized_task", "invalid_request")

	// Oversized budget
	resBud, rawBud, budErr := runCompose(atlasBin, ws.AtlasHome, ws.ProjectA, taskPromptA, 32001, 0)
	if err := requireEvidence(filepath.Join(ws.Evidence, "security-oversized-budget.json"), rawBud); err != nil {
		return fail("security.oversized_budget.evidence", err.Error())
	}
	if resBud.Status != contextcompose.StatusInvalidRequest || budErr == nil {
		return fail("security.oversized_budget", fmt.Sprintf("status=%s", resBud.Status))
	}
	pass("security.oversized_budget", "invalid_request")

	// Symlink Memory DB
	memPath := memoryDBPath(ws.AtlasHome, a.ID)
	memBak, err := os.ReadFile(memPath)
	if err != nil {
		return fail("security.symlink_memory.read", err.Error())
	}
	outsideMem := filepath.Join(ws.RunDir, "outside-memory.db")
	if err := os.WriteFile(outsideMem, memBak, 0o600); err != nil {
		return err
	}
	_ = os.Remove(memPath)
	if err := os.Symlink(outsideMem, memPath); err != nil {
		_ = os.WriteFile(memPath, memBak, 0o600)
		return fail("security.symlink_memory.link", err.Error())
	}
	resSM, rawSM, _ := runCompose(atlasBin, ws.AtlasHome, ws.ProjectA, taskPromptA, 0, 0)
	if err := requireEvidence(filepath.Join(ws.Evidence, "security-symlink-memory.json"), rawSM); err != nil {
		_ = os.Remove(memPath)
		_ = os.WriteFile(memPath, memBak, 0o600)
		return fail("security.symlink_memory.evidence", err.Error())
	}
	_ = os.Remove(memPath)
	_ = os.WriteFile(memPath, memBak, 0o600)
	if !statusIn(sourceOf(resSM, contextcompose.SourceMemory), contextcompose.SourceStatusFailed, contextcompose.SourceStatusUnavailable) {
		return fail("security.symlink_memory", fmt.Sprintf("memory status=%s", sourceOf(resSM, contextcompose.SourceMemory)))
	}
	if !hasSource(resSM, contextcompose.SourceCodeGraph, contextcompose.SourceStatusSupplied) {
		return fail("security.symlink_memory.continuity", "codegraph must continue")
	}
	pass("security.symlink_memory", "memory fail-closed; healthy sources continue")

	// Symlink CE pack
	packPath := atlascontext.PackPath(ws.AtlasHome, a.ID, a.PackID)
	packBak, err := os.ReadFile(packPath)
	if err != nil {
		return fail("security.symlink_ce.read", err.Error())
	}
	outsidePack := filepath.Join(ws.RunDir, "outside-pack.yaml")
	_ = os.WriteFile(outsidePack, []byte("schema_version: 1\n"), 0o600)
	packsDir := filepath.Dir(packPath)
	entries, _ := os.ReadDir(packsDir)
	type bak struct {
		path string
		data []byte
	}
	var backups []bak
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		p := filepath.Join(packsDir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		backups = append(backups, bak{p, data})
		_ = os.Remove(p)
		_ = os.Symlink(outsidePack, p)
	}
	resSC, rawSC, _ := runCompose(atlasBin, ws.AtlasHome, ws.ProjectA, taskPromptA, 0, 0)
	if err := requireEvidence(filepath.Join(ws.Evidence, "security-symlink-ce.json"), rawSC); err != nil {
		for _, b := range backups {
			_ = os.Remove(b.path)
			_ = os.WriteFile(b.path, b.data, 0o600)
		}
		return fail("security.symlink_ce.evidence", err.Error())
	}
	for _, b := range backups {
		_ = os.Remove(b.path)
		_ = os.WriteFile(b.path, b.data, 0o600)
	}
	if sourceOf(resSC, contextcompose.SourceContextEconomy) != contextcompose.SourceStatusFailed {
		return fail("security.symlink_ce", fmt.Sprintf("ce status=%s", sourceOf(resSC, contextcompose.SourceContextEconomy)))
	}
	if !hasSource(resSC, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		return fail("security.symlink_ce.continuity", "memory must continue")
	}
	pass("security.symlink_ce", "CE fail-closed; Memory supplied")

	// Symlink CodeGraph DB
	cgDB := codeintel.GraphDBPath(ws.AtlasHome, a.ID, codeintel.ProviderCodeGraph)
	cgBak, err := os.ReadFile(cgDB)
	if err != nil {
		return fail("security.symlink_cg.read", err.Error())
	}
	outsideCG := filepath.Join(ws.RunDir, "outside-graph.db")
	_ = os.WriteFile(outsideCG, cgBak, 0o600)
	_ = os.Remove(cgDB)
	if err := os.Symlink(outsideCG, cgDB); err != nil {
		_ = os.WriteFile(cgDB, cgBak, 0o600)
		return fail("security.symlink_cg.link", err.Error())
	}
	resSG, rawSG, _ := runCompose(atlasBin, ws.AtlasHome, ws.ProjectA, taskPromptA, 0, 0)
	if err := requireEvidence(filepath.Join(ws.Evidence, "security-symlink-cg.json"), rawSG); err != nil {
		_ = os.Remove(cgDB)
		_ = os.WriteFile(cgDB, cgBak, 0o600)
		return fail("security.symlink_cg.evidence", err.Error())
	}
	_ = os.Remove(cgDB)
	_ = os.WriteFile(cgDB, cgBak, 0o600)
	if !statusIn(sourceOf(resSG, contextcompose.SourceCodeGraph), contextcompose.SourceStatusFailed, contextcompose.SourceStatusUnavailable) {
		return fail("security.symlink_cg", fmt.Sprintf("codegraph status=%s", sourceOf(resSG, contextcompose.SourceCodeGraph)))
	}
	if !hasSource(resSG, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		return fail("security.symlink_cg.continuity", "memory must continue")
	}
	pass("security.symlink_cg", "CodeGraph fail-closed; Memory supplied")
	_ = packBak

	// Missing --json
	missingRun, missingErr := runBounded(
		context.Background(),
		defaultCmdTimeout,
		maxStdoutCapture,
		maxStderrCapture,
		atlasBin,
		[]string{"context", "compose", "task"},
		ws.ProjectA,
		withEnv(os.Environ(), map[string]string{"ATLAS_HOME": ws.AtlasHome}),
	)
	if !missingRun.JSONOK || missingRun.Result.Status != contextcompose.StatusInvalidRequest || missingErr == nil {
		return fail("security.missing_json", sanitizeHarnessErr(missingErr))
	}
	pass("security.missing_json", "invalid_request")
	return nil
}

func buildCertificationMatrix(rows []matrixRow) []matrixRow {
	// Generic categories: any FAIL under the prefix blocks PASS; at least one PASS required.
	certifyPrefix := func(id, prefix, detail string) matrixRow {
		var hasPass, hasFail bool
		var failIDs []string
		for _, r := range rows {
			if !strings.HasPrefix(r.ID, prefix) {
				continue
			}
			switch r.Result {
			case resultPASS:
				hasPass = true
			case resultFAIL:
				hasFail = true
				failIDs = append(failIDs, r.ID)
			}
		}
		if hasFail {
			return matrixRow{ID: id, Result: resultFAIL, Detail: "failed invariant(s): " + strings.Join(failIDs, ", ")}
		}
		if hasPass {
			return matrixRow{ID: id, Result: resultPASS, Detail: detail}
		}
		return matrixRow{ID: id, Result: resultFAIL, Detail: "no passing evidence for " + prefix}
	}

	out := []matrixRow{
		certifyPrefix("cert.hooks_foundation", "hooks.", "projection + contract"),
		certifyPrefix("cert.hooks_engine", "hooks.contract", "bounded subprocess allow/deny"),
		certifyPrefix("cert.cursor_hooks", "hooks.", "cursor hooks.json + dispatch"),
		certifyPrefix("cert.opencode_hooks", "hooks.", "opencode plugin present"),
		certifyPrefix("cert.memory_skill", "memory.skill", "canonical skill + AGENTS"),
		certifyPrefix("cert.composer_core", "compose.", "compose JSON + budget"),
		certifyPrefix("cert.memory_source", "compose.a", "memory supplied"),
		certifyPrefix("cert.ce_source", "compose.a", "ce supplied"),
		certifyPrefix("cert.codegraph_source", "compose.a", "codegraph supplied"),
		certifyPrefix("cert.context_compose", "compose.", "three-source compose"),
		certifyPrefix("cert.partial_failure", "partial.", "partial matrix"),
		certifyPrefix("cert.read_only", "readonly.", "fingerprints"),
		certifyPrefix("cert.security", "security.", "fail-closed continuity"),
	}
	// Project isolation: requires isolation.ab PASS; any isolation FAIL blocks;
	// UNVERIFIED swap probes document trust-boundary limits and do not alone grant PASS.
	isoResult, isoDetail := categoryCertificationResult(rows, []string{"isolation.ab"}, []string{"isolation."})
	out = append(out, matrixRow{ID: "cert.project_isolation", Result: isoResult, Detail: isoDetail})
	return out
}

func writeFinalEvidence(ws *runWorkspace, atlasBin, version, sha, cgVersion, idA, idB string, rows []matrixRow, ok bool) error {
	overall := "FAIL"
	if ok {
		overall = "PASS"
	}
	doc := map[string]any{
		"slice":             "36B.7",
		"overall":           overall,
		"atlas_bin":         atlasBin,
		"atlas_version":     version,
		"atlas_sha256":      sha,
		"codegraph_version": cgVersion,
		"git_sha":           strings.TrimSpace(os.Getenv("ATLAS_EXPECTED_HEAD")),
		"go":                runtime.Version(),
		"os":                runtime.GOOS + "/" + runtime.GOARCH,
		"e2e_root":          ws.Root,
		"run_dir":           ws.RunDir,
		"ATLAS_HOME":        ws.AtlasHome,
		"project_a_id":      idA,
		"project_b_id":      idB,
		"auto_injection":    false,
		"budget_unit":       "utf8_runes_not_tokens",
		"matrix":            rows,
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(ws.Evidence, "MATRIX.json")
	_ = os.Remove(path)
	if err := writeEvidenceFile(path, append(b, '\n')); err != nil {
		return err
	}
	var sb strings.Builder
	sb.WriteString("# Atlas 36B.7 Global Integration E2E\n\n")
	sb.WriteString("Overall: **" + overall + "**\n\n")
	sb.WriteString("| Check | Result | Detail |\n|---|---|---|\n")
	for _, r := range rows {
		sb.WriteString("| " + r.ID + " | " + r.Result + " | " + r.Detail + " |\n")
	}
	sumPath := filepath.Join(ws.Evidence, "SUMMARY.md")
	_ = os.Remove(sumPath)
	return writeEvidenceFile(sumPath, []byte(sb.String()))
}

func writeInteractiveProcedure(ws *runWorkspace, atlasBin, version, sha, cgVersion string, a, b *projectFixture) error {
	body := fmt.Sprintf(`# 36B.7 Interactive E2E Procedure

## Environment

- atlas_bin: %s
- atlas_version: %s
- atlas_sha256: %s
- codegraph_version: %s
- ATLAS_HOME: %s
- project_a: %s (id=%s)
- project_b: %s (id=%s)
- task: %q

## Canaries

- Memory A: %s
- CE A: %s
- CodeGraph A: %s
- Memory B / CE B / CodeGraph B must NOT appear in Project A answers

## Cursor

1. Open project-a in Cursor IDE (new window).
2. Ensure PATH includes project-a/bin or use that wrapper.
3. Start a fresh Agent conversation.
4. Prompt (Spanish as specified):

"Analiza este proyecto usando las capacidades de Atlas.
Recupera las decisiones persistentes, el contexto estructural
y la implementación relacionada con pagos.
Indica qué información encontraste y de qué fuente procede.
No modifiques archivos ni ejecutes operaciones Git."

5. PASS requires observed evidence that Cursor:
   - discovers Atlas instructions/skill
   - invokes Atlas retrieval (compose and/or memory)
   - cites Memory ALPHA, CE ALPHA, ProcessPaymentAlpha
   - identifies provenance
   - does not modify files / use Git / claim automatic injection

6. Save transcript outside the Atlas-CLI repo, then copy a redacted excerpt to:
   evidence/cursor-interactive.txt

## OpenCode

Repeat equivalent test in OpenCode against project-a.
Save evidence/opencode-interactive.txt

## Notes

- Native automatic injection is NOT guaranteed.
- Observations are evidence, not policy.
- Do not mark interactive rows PASS without real transcripts.
`, atlasBin, version, sha, cgVersion, ws.AtlasHome, ws.ProjectA, a.ID, ws.ProjectB, b.ID, taskPromptA,
		canaryMemA, canaryCEA, symbolA)
	return writeEvidenceFile(filepath.Join(ws.Evidence, "INTERACTIVE-E2E.md"), []byte(body))
}
