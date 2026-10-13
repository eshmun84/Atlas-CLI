// Command e2ecodegraph prepares an isolated Atlas-E2E project, builds a
// CodeGraph DB as explicit setup, then verifies read-only composition via the
// real atlas binary.
//
// Required env:
//
//	ATLAS_E2E_ROOT — existing directory that will hold a unique run workspace
//	ATLAS_BIN      — absolute path to atlas binary
//
// Optional:
//
//	ATLAS_E2E_KEEP=1 — retain workspace on failure (always retained on success)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"gopkg.in/yaml.v3"
)

const (
	projectName = "cg-compose-36b6c"
	task        = "explain ProcessPayment callers and dependencies"
	canaryMem   = "CANARY-CG-36B6C-MEMORY payment processing uses ProcessPayment"
)

type matrixRow struct {
	ID     string `json:"id"`
	Result string `json:"result"`
	Detail string `json:"detail,omitempty"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2ecodegraph: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("e2ecodegraph: ok")
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

	if err := os.MkdirAll(filepath.Join(ws.Project, "src"), 0o755); err != nil {
		return err
	}
	if err := writeProject(ws.Project); err != nil {
		return err
	}
	projectID, err := home.ProjectID(ws.Project, projectName)
	if err != nil {
		return err
	}
	if err := writeEvidenceFile(filepath.Join(ws.Evidence, "project-id.txt"), []byte(projectID+"\n")); err != nil {
		return fmt.Errorf("write project-id evidence: %w", err)
	}

	cgDir := codeintel.ProviderStorageDir(ws.AtlasHome, projectID, codeintel.ProviderCodeGraph)
	db := codeintel.GraphDBPath(ws.AtlasHome, projectID, codeintel.ProviderCodeGraph)
	if err := os.MkdirAll(cgDir, 0o700); err != nil {
		return err
	}

	// E2E setup mutation — explicit build, not composition.
	verRun, err := runBounded(context.Background(), 10*time.Second, 4*1024, 4*1024, "codegraph", []string{"--version"}, "", os.Environ())
	if err != nil {
		return fmt.Errorf("codegraph --version: %w", err)
	}
	if err := writeEvidenceFile(filepath.Join(ws.Evidence, "codegraph-version.txt"), verRun.Stdout); err != nil {
		return fmt.Errorf("write codegraph version evidence: %w", err)
	}

	buildRun, err := runBounded(
		context.Background(),
		defaultBuildTimeout,
		maxStdoutCapture,
		maxStderrCapture,
		"codegraph",
		[]string{"build", ws.Project, "--db", db},
		"",
		os.Environ(),
	)
	buildEvidence := append(append([]byte{}, buildRun.Stdout...), buildRun.Stderr...)
	if werr := writeEvidenceFile(filepath.Join(ws.Evidence, "codegraph-build.txt"), buildEvidence); werr != nil {
		return fmt.Errorf("write codegraph build evidence: %w", werr)
	}
	if err != nil {
		return fmt.Errorf("codegraph build: %w", err)
	}
	if _, err := os.Lstat(db); err != nil {
		return fmt.Errorf("graph.db missing after build: %w", err)
	}

	if err := seedMemory(ws.AtlasHome, projectID); err != nil {
		return err
	}

	// Fingerprint live CodeGraph storage BEFORE the first Atlas composition.
	// No warmup query — first compose must leave this tree unchanged.
	beforeFP, err := fingerprintTree(cgDir)
	if err != nil {
		return fmt.Errorf("fingerprint codegraph storage before compose: %w", err)
	}
	if err := writeEvidenceFile(filepath.Join(ws.Evidence, "codegraph-storage-fp-before.txt"), []byte(beforeFP+"\n")); err != nil {
		return fmt.Errorf("write before fingerprint: %w", err)
	}

	raw1, res1, err := runCompose(atlasBin, ws.AtlasHome, ws.Project, task)
	if err != nil {
		return err
	}
	if err := writeEvidenceFile(filepath.Join(ws.Evidence, "compose-1.json"), raw1); err != nil {
		return fmt.Errorf("write compose-1 evidence: %w", err)
	}

	afterFirstFP, err := fingerprintTree(cgDir)
	if err != nil {
		return fmt.Errorf("fingerprint codegraph storage after first compose: %w", err)
	}
	if err := writeEvidenceFile(filepath.Join(ws.Evidence, "codegraph-storage-fp-after-first.txt"), []byte(afterFirstFP+"\n")); err != nil {
		return fmt.Errorf("write after-first fingerprint: %w", err)
	}

	raw2, res2, err := runCompose(atlasBin, ws.AtlasHome, ws.Project, task)
	if err != nil {
		return err
	}
	if err := writeEvidenceFile(filepath.Join(ws.Evidence, "compose-2.json"), raw2); err != nil {
		return fmt.Errorf("write compose-2 evidence: %w", err)
	}

	afterSecondFP, err := fingerprintTree(cgDir)
	if err != nil {
		return fmt.Errorf("fingerprint codegraph storage after second compose: %w", err)
	}
	if err := writeEvidenceFile(filepath.Join(ws.Evidence, "codegraph-storage-fp-after-second.txt"), []byte(afterSecondFP+"\n")); err != nil {
		return fmt.Errorf("write after-second fingerprint: %w", err)
	}

	var rows []matrixRow
	pass := func(id, detail string) { rows = append(rows, matrixRow{ID: id, Result: "PASS", Detail: detail}) }
	fail := func(id, detail string) error {
		rows = append(rows, matrixRow{ID: id, Result: "FAIL", Detail: detail})
		if werr := writeMatrixAndSummary(ws.Evidence, ws.RunDir, rows, false); werr != nil {
			return fmt.Errorf("%s: %s (also failed writing evidence: %v)", id, detail, werr)
		}
		return fmt.Errorf("%s: %s", id, detail)
	}

	if !hasSource(res1, contextcompose.SourceCodeGraph, contextcompose.SourceStatusSupplied) {
		return fail("e2e.codegraph.supplied", fmt.Sprintf("%v", res1.Sources))
	}
	pass("e2e.codegraph.supplied", sourceDetail(res1, contextcompose.SourceCodeGraph))

	if !hasCodeGraphRef(res1, "ProcessPayment", "payments.go") {
		return fail("e2e.codegraph.known_ref", "ProcessPayment / payments.go missing")
	}
	pass("e2e.codegraph.known_ref", "ProcessPayment present")

	if !hasSource(res1, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		return fail("e2e.memory.coexist", fmt.Sprintf("%v", res1.Sources))
	}
	pass("e2e.memory.coexist", sourceDetail(res1, contextcompose.SourceMemory))

	if sourceStatus(res1, contextcompose.SourceContextEconomy) == "" {
		return fail("e2e.ce.status_visible", "context_economy status missing")
	}
	pass("e2e.ce.status_visible", sourceDetail(res1, contextcompose.SourceContextEconomy))

	if absPathLeak(res1) {
		return fail("e2e.no_abs_paths", "absolute path in items")
	}
	pass("e2e.no_abs_paths", "item paths/refs relative")

	if beforeFP != afterFirstFP {
		return fail("e2e.readonly.first_compose", "live CodeGraph storage changed on FIRST compose")
	}
	pass("e2e.readonly.first_compose", "storage fingerprint unchanged after first compose")
	if beforeFP != afterSecondFP {
		return fail("e2e.readonly.second_compose", "live CodeGraph storage changed on second compose")
	}
	pass("e2e.readonly.second_compose", "storage fingerprint unchanged after second compose")

	a, _ := json.Marshal(res1.Items)
	b, _ := json.Marshal(res2.Items)
	if string(a) != string(b) {
		return fail("e2e.determinism", "items diverged")
	}
	pass("e2e.determinism", "identical items across two runs")

	if err := writeMatrixAndSummary(ws.Evidence, ws.RunDir, rows, true); err != nil {
		return err
	}
	failed = false
	fmt.Printf("workspace=%s\n", ws.RunDir)
	return nil
}

func resolveAtlasBin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("ATLAS_BIN is required")
	}
	if !filepath.IsAbs(raw) {
		return "", fmt.Errorf("ATLAS_BIN must be absolute")
	}
	info, err := os.Lstat(raw)
	if err != nil {
		return "", fmt.Errorf("ATLAS_BIN lstat: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("ATLAS_BIN must not be a symbolic link")
	}
	if info.IsDir() {
		return "", fmt.Errorf("ATLAS_BIN is a directory")
	}
	return filepath.Clean(raw), nil
}

func writeProject(root string) error {
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		return err
	}
	cfg := config.DefaultConfig(projectName)
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), data, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module "+projectName+"\n\ngo 1.22\n"), 0o644); err != nil {
		return err
	}
	payments := `package payments

// ProcessPayment is the known E2E symbol for CodeGraph composition.
func ProcessPayment(amount int) int {
	return normalizeAmount(amount)
}

func normalizeAmount(amount int) int {
	if amount < 0 {
		return 0
	}
	return amount
}
`
	mainGo := `package payments

func Entry() {
	_ = ProcessPayment(42)
}
`
	if err := os.WriteFile(filepath.Join(root, "src", "payments.go"), []byte(payments), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "src", "main.go"), []byte(mainGo), 0o644)
}

func seedMemory(atlasHome, projectID string) error {
	store, err := memory.Open(atlasHome, projectID)
	if err != nil {
		return err
	}
	defer store.Close()
	_, err = store.SaveObservation(memory.Observation{
		Type:    memory.TypeFact,
		Title:   "Payment canary",
		Content: canaryMem,
	})
	return err
}

func runCompose(atlasBin, atlasHome, project, task string) ([]byte, composeapp.Result, error) {
	rr, err := runBounded(
		context.Background(),
		defaultCmdTimeout,
		maxStdoutCapture,
		maxStderrCapture,
		atlasBin,
		[]string{"context", "compose", task, "--json"},
		project,
		withEnv(os.Environ(), map[string]string{"ATLAS_HOME": atlasHome}),
	)
	if rr.JSONOK {
		return rr.Stdout, rr.Result, err
	}
	if err == nil {
		err = fmt.Errorf("compose stdout was not valid JSON")
	}
	return rr.Stdout, rr.Result, err
}

func hasSource(res composeapp.Result, source, status string) bool {
	for _, st := range res.Sources {
		if st.Source == source && st.Status == status {
			return true
		}
	}
	return false
}

func sourceStatus(res composeapp.Result, source string) string {
	for _, st := range res.Sources {
		if st.Source == source {
			return st.Status
		}
	}
	return ""
}

func sourceDetail(res composeapp.Result, source string) string {
	for _, st := range res.Sources {
		if st.Source == source {
			return st.Status + ": " + st.Message
		}
	}
	return ""
}

func hasCodeGraphRef(res composeapp.Result, title, pathPart string) bool {
	for _, it := range res.Items {
		if it.Source != contextcompose.SourceCodeGraph {
			continue
		}
		if it.Title == title || strings.Contains(it.Path, pathPart) {
			return true
		}
	}
	return false
}

func absPathLeak(res composeapp.Result) bool {
	for _, it := range res.Items {
		if strings.HasPrefix(it.Path, "/") || strings.HasPrefix(it.Ref, "/") {
			return true
		}
	}
	return false
}

func writeMatrixAndSummary(evidence, ws string, rows []matrixRow, ok bool) error {
	overall := "FAIL"
	if ok {
		overall = "PASS"
	}
	matrix := map[string]any{"overall": overall, "workspace": ws, "rows": rows}
	raw, err := json.MarshalIndent(matrix, "", "  ")
	if err != nil {
		return err
	}
	if err := writeEvidenceFile(filepath.Join(evidence, "matrix.json"), append(raw, '\n')); err != nil {
		return fmt.Errorf("write matrix evidence: %w", err)
	}
	var b strings.Builder
	b.WriteString("# CodeGraph Composition E2E — 36B.6C\n\n")
	b.WriteString("Workspace: `" + ws + "`\n\n")
	b.WriteString("Overall: **" + overall + "**\n\n")
	b.WriteString("| Check | Result | Detail |\n|---|---|---|\n")
	for _, r := range rows {
		b.WriteString("| " + r.ID + " | " + r.Result + " | " + r.Detail + " |\n")
	}
	b.WriteString("\nDistinction:\n")
	b.WriteString("- E2E setup mutation: `codegraph build` only\n")
	b.WriteString("- Atlas composition: first and subsequent `atlas context compose` must leave live CodeGraph storage fingerprint unchanged (query uses an isolated snapshot)\n")
	if err := writeEvidenceFile(filepath.Join(evidence, "SUMMARY.md"), []byte(b.String())); err != nil {
		return fmt.Errorf("write summary evidence: %w", err)
	}
	return nil
}
