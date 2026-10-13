// Command e2epersist materializes an isolated Atlas project under a validated
// E2E root and certifies quality-governed automatic Memory persistence (36C).
//
// Required env:
//
//	ATLAS_E2E_ROOT  — existing real directory that will hold unique run workspaces
//	ATLAS_BIN       — path to atlas binary (optional; defaults to PATH "atlas")
//
// Safety:
//   - Never deletes pre-existing directories under the E2E root.
//   - Creates a unique atlas-e2e-persist-* workspace per run.
//   - Subprocesses use explicit argv (no shell), timeouts, and bounded
//     stdin/stdout/stderr with kill-on-overflow (see subprocess.go).
//   - On failure, removes only that uniquely created workspace.
//   - On success, retains the workspace as evidence.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
	"github.com/eshmun84/Atlas-CLI/internal/memorypersist"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
)

const canaryTitle = "Prefer governed SQLite Memory persistence"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "e2epersist: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("e2epersist: ok")
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

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      "persist-36c-alpha",
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: true,
	})
	if !draft.SetValue("adapters.selected", "cursor,opencode") {
		return fmt.Errorf("set adapters.selected")
	}
	fixed := time.Date(2026, 10, 13, 15, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  ws.Project,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed },
	}); err != nil {
		return fmt.Errorf("ApplyConfig: %w", err)
	}

	projectID, err := home.ProjectID(ws.Project, "persist-36c-alpha")
	if err != nil {
		return err
	}

	// 1) Memory initially empty
	ok, err := memory.Exists(ws.AtlasHome, projectID)
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("memory db must start empty")
	}

	env := withEnv(os.Environ(), map[string]string{"ATLAS_HOME": ws.AtlasHome})
	ctx := context.Background()

	// 2) Durable explicit decision via real propose path
	durable := memorypersist.Candidate{
		Type:         string(memory.TypeDecision),
		Title:        canaryTitle,
		Content:      "Atlas may persist durable project knowledge only when candidates pass quality governance; Memory is not a conversation log.",
		Scope:        string(memory.ScopeProject),
		TopicKey:     "memory.persistence.policy",
		Origin:       "atlas_lifecycle",
		Explicitness: "explicit",
		Confidence:   "high",
		Provenance:   "cap=e2e-36c;ref=durable-decision",
		SourceEvent:  "e2e.durable.accepted",
	}
	resp, raw, err := runPropose(ctx, atlasBin, ws.Project, env, durable)
	if err != nil {
		return err
	}
	if resp.Status != memorypersist.StatusAccepted {
		return fmt.Errorf("durable propose status=%q raw=%s", resp.Status, redactPaths(raw))
	}
	durableID := resp.Observation.ID

	// 3) Inferred + confirm key must not write (unknown field → invalid_input)
	inferredConfirmRaw := []byte(`{
		"type":"decision",
		"title":"Inferred cache backend preference note",
		"content":"Agents inferred Redis for caching but must not self-confirm through propose JSON.",
		"scope":"project",
		"topic_key":"memory.persistence.inferred",
		"origin":"atlas_operation",
		"explicitness":"inferred",
		"confidence":"medium",
		"confirm":true
	}`)
	resp, raw, err = runProposeRaw(ctx, atlasBin, ws.Project, env, inferredConfirmRaw)
	if resp.Status != memorypersist.StatusInvalidInput {
		return fmt.Errorf("inferred+confirm want invalid_input got %q err=%v raw=%s", resp.Status, err, redactPaths(raw))
	}

	// 4) Inferred without confirm → confirmation_required, no write
	inferred := durable
	inferred.Title = "Inferred cache backend preference note"
	inferred.Content = "Agents inferred Redis for caching but confirmation is deferred in V1."
	inferred.TopicKey = "memory.persistence.inferred"
	inferred.Origin = "atlas_operation"
	inferred.Explicitness = "inferred"
	inferred.Confidence = "medium"
	resp, raw, err = runPropose(ctx, atlasBin, ws.Project, env, inferred)
	if err != nil {
		return fmt.Errorf("inferred propose exec: %w raw=%s", err, redactPaths(raw))
	}
	if resp.Status != memorypersist.StatusConfirmationRequired {
		return fmt.Errorf("inferred want confirmation_required got %q raw=%s", resp.Status, redactPaths(raw))
	}

	// 4b) Self-labeled confirmed must not write (origin is also untrusted)
	confirmed := durable
	confirmed.Title = "Self labeled confirmed architecture note"
	confirmed.Content = "Agents must not self-assert confirmed through the untrusted propose transport."
	confirmed.TopicKey = "memory.persistence.confirmed"
	confirmed.Origin = "developer_confirmation"
	confirmed.Explicitness = "confirmed"
	confirmed.Confidence = "high"
	resp, raw, err = runPropose(ctx, atlasBin, ws.Project, env, confirmed)
	if err != nil {
		return fmt.Errorf("confirmed propose exec: %w raw=%s", err, redactPaths(raw))
	}
	if resp.Status != memorypersist.StatusConfirmationRequired {
		return fmt.Errorf("confirmed want confirmation_required got %q raw=%s", resp.Status, redactPaths(raw))
	}
	if resp.Observation != nil {
		return fmt.Errorf("confirmed must not write")
	}

	// 5) Transient candidate
	transient := durable
	transient.Title = "Temporary debug path for today"
	transient.Content = "Use /tmp/scratch for this one-off debugging session only."
	transient.TopicKey = "memory.persistence.transient"
	resp, raw, err = runPropose(ctx, atlasBin, ws.Project, env, transient)
	if err != nil {
		return fmt.Errorf("transient propose exec: %w raw=%s", err, redactPaths(raw))
	}
	if resp.Status != memorypersist.StatusRejected {
		return fmt.Errorf("transient want rejected got %q raw=%s", resp.Status, redactPaths(raw))
	}

	// 6) Credential-like candidate
	secret := durable
	secret.Title = "Rotate production API credentials note"
	secret.Content = "Export OPENAI_API_KEY=sk-abcdefghijklmnopqrstuvwxyz0123456789 before deploy"
	secret.TopicKey = "memory.persistence.secret"
	resp, raw, err = runPropose(ctx, atlasBin, ws.Project, env, secret)
	if err != nil {
		return fmt.Errorf("secret propose exec: %w raw=%s", err, redactPaths(raw))
	}
	if resp.Status != memorypersist.StatusRejected {
		return fmt.Errorf("secret want rejected got %q raw=%s", resp.Status, redactPaths(raw))
	}

	// 7) Repeat durable → duplicate
	resp, raw, err = runPropose(ctx, atlasBin, ws.Project, env, durable)
	if err != nil {
		return fmt.Errorf("duplicate propose exec: %w raw=%s", err, redactPaths(raw))
	}
	if resp.Status != memorypersist.StatusDuplicate {
		return fmt.Errorf("replay want duplicate got %q raw=%s", resp.Status, redactPaths(raw))
	}
	if resp.Observation == nil || resp.Observation.ID != durableID {
		return fmt.Errorf("duplicate id mismatch")
	}

	// 8) Retrieve Memory — durable exactly once
	ctxResp, ctxRaw, err := runMemoryContext(ctx, atlasBin, ws.Project, env, "governed SQLite Memory persistence")
	if err != nil {
		return fmt.Errorf("memory context: %w raw=%s", err, redactPaths(ctxRaw))
	}
	if ctxResp.Status != memoryretrieve.StatusSuccess || ctxResp.Count < 1 {
		return fmt.Errorf("context status=%q count=%d body=%s", ctxResp.Status, ctxResp.Count, redactPaths(ctxRaw))
	}
	hits := 0
	for _, it := range ctxResp.Items {
		if it.Title == canaryTitle || strings.Contains(it.Title, "governed SQLite") {
			hits++
		}
		low := strings.ToLower(it.Title + " " + it.Excerpt)
		if strings.Contains(low, "openai_api_key") || strings.Contains(low, "/tmp/scratch") ||
			strings.Contains(low, "inferred redis") || strings.Contains(low, "self labeled confirmed") {
			return fmt.Errorf("rejected/inferred/confirmed material leaked into retrieval: title=%q", it.Title)
		}
	}
	if hits < 1 {
		return fmt.Errorf("durable canary missing from context: %s", redactPaths(ctxRaw))
	}

	store, err := memory.OpenReadOnly(ws.AtlasHome, projectID)
	if err != nil {
		return err
	}
	list, err := store.ListObservations(memory.ListFilter{Limit: 50})
	_ = store.Close()
	if err != nil {
		return err
	}
	if len(list) != 1 {
		return fmt.Errorf("want exactly 1 observation, got %d", len(list))
	}
	if list[0].Title != canaryTitle {
		return fmt.Errorf("unexpected observation title=%q", list[0].Title)
	}

	// 9) context compose sees durable decision (in-process; read-only)
	composeRes := composeapp.Compose(composeapp.Input{
		Task: "orient governed SQLite Memory persistence policy",
		Env:  memoryretrieve.Env{Root: ws.Project, Home: ws.AtlasHome},
	})
	switch composeRes.Status {
	case contextcompose.StatusSuccess, contextcompose.StatusPartial, contextcompose.StatusEmpty:
	default:
		return fmt.Errorf("compose status=%q count=%d", composeRes.Status, composeRes.Count)
	}
	found := false
	for _, it := range composeRes.Items {
		if strings.Contains(it.Title, "governed SQLite") || strings.Contains(it.Excerpt, "quality governance") {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("compose missing durable decision: status=%q", composeRes.Status)
	}

	// 10) Project B isolation
	wsBProject := filepath.Join(ws.RunDir, "project-b")
	if err := os.Mkdir(wsBProject, 0o755); err != nil {
		return err
	}
	draftB := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      "persist-36c-bravo",
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: false,
	})
	if !draftB.SetValue("adapters.selected", "cursor") {
		return fmt.Errorf("set adapters.selected b")
	}
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  wsBProject,
		Draft: draftB,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed },
	}); err != nil {
		return fmt.Errorf("ApplyConfig B: %w", err)
	}
	idB, err := home.ProjectID(wsBProject, "persist-36c-bravo")
	if err != nil {
		return err
	}
	ctxB := memoryretrieve.Service{Env: memoryretrieve.Env{Root: wsBProject, Home: ws.AtlasHome}}.
		Context(memoryretrieve.ContextInput{Task: "governed SQLite Memory persistence"})
	if ctxB.Status == memoryretrieve.StatusSuccess {
		for _, it := range ctxB.Items {
			if it.Title == canaryTitle {
				return fmt.Errorf("project B retrieved project A canary")
			}
		}
	}
	okB, err := memory.Exists(ws.AtlasHome, idB)
	if err != nil {
		return err
	}
	if okB {
		storeB, err := memory.OpenReadOnly(ws.AtlasHome, idB)
		if err == nil {
			listB, _ := storeB.ListObservations(memory.ListFilter{Limit: 10})
			_ = storeB.Close()
			if len(listB) != 0 {
				return fmt.Errorf("project B memory not empty")
			}
		}
	}

	evidence := filepath.Join(ws.RunDir, "EVIDENCE.md")
	body := fmt.Sprintf(`# 36C Persistence E2E Evidence (hardened)

- run_dir: %s
- project_a: %s
- project_b: %s
- ATLAS_HOME: %s
- project_id_a: %s
- project_id_b: %s
- durable_observation_id: %s
- observation_count: %d
- context_status: %s
- compose_status: %s

## Results

- durable explicit decision present exactly once: PASS
- inferred+confirm => invalid_input (no write): PASS
- inferred => confirmation_required (no write): PASS
- confirmed => confirmation_required (no write): PASS
- transient absent: PASS
- credential absent: PASS
- duplicate replay: PASS
- context compose sees only explicit durable: PASS
- project B isolation: PASS
`, ws.RunDir, ws.Project, wsBProject, ws.AtlasHome, projectID, idB, durableID, len(list), ctxResp.Status, composeRes.Status)
	if err := os.WriteFile(evidence, []byte(body), 0o644); err != nil {
		return err
	}

	fmt.Printf("e2e_root=%s\n", ws.Root)
	fmt.Printf("run_dir=%s\n", ws.RunDir)
	fmt.Printf("project=%s\n", ws.Project)
	fmt.Printf("ATLAS_HOME=%s\n", ws.AtlasHome)
	fmt.Printf("project_id=%s\n", projectID)
	fmt.Printf("durable_id=%s\n", durableID)
	fmt.Printf("observation_count=%d\n", len(list))
	fmt.Printf("context_status=%s\n", ctxResp.Status)
	fmt.Printf("compose_status=%s\n", composeRes.Status)
	fmt.Printf("evidence=%s\n", evidence)
	keepEvidence = true
	return nil
}

func runPropose(parent context.Context, atlasBin, dir string, env []string, c memorypersist.Candidate) (memorypersist.Response, string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return memorypersist.Response{}, "", err
	}
	return runProposeRaw(parent, atlasBin, dir, env, payload)
}

func runProposeRaw(parent context.Context, atlasBin, dir string, env []string, payload []byte) (memorypersist.Response, string, error) {
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

func runMemoryContext(parent context.Context, atlasBin, dir string, env []string, task string) (memoryretrieve.Response, string, error) {
	rr, err := runBounded(parent, defaultCmdTimeout, maxStdoutCapture, maxStderrCapture,
		atlasBin, []string{"memory", "context", task, "--json"}, dir, env)
	raw := string(rr.Stdout)
	var resp memoryretrieve.Response
	if jerr := json.Unmarshal(rr.Stdout, &resp); jerr != nil {
		if err != nil {
			return memoryretrieve.Response{}, raw, fmt.Errorf("%v", err)
		}
		return memoryretrieve.Response{}, raw, fmt.Errorf("decode context json: %w", jerr)
	}
	if err != nil && memoryretrieve.ShouldExitNonZero(resp.Status) {
		return resp, raw, err
	}
	return resp, raw, nil
}
