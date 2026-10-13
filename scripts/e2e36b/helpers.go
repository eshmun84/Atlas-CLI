package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
)

type matrixRow struct {
	ID     string `json:"id"`
	Result string `json:"result"` // PASS | FAIL | SKIPPED
	Detail string `json:"detail,omitempty"`
}

type projectFixture struct {
	Root      string
	Name      string
	ID        string
	CanaryMem string
	CanaryCE  string
	Symbol    string
	PackID    string
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
	return filepath.Clean(raw), nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(dst)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(dst)
		return closeErr
	}
	return nil
}

func initProject(root, atlasHome, name string, now time.Time) (string, error) {
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
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module "+name+"\n\ngo 1.22\n"), 0o644); err != nil {
		return "", err
	}
	return home.ProjectID(root, name)
}

func writePaymentSource(root, symbol string) error {
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		return err
	}
	payments := fmt.Sprintf(`package payments

// %s is the known E2E symbol for 36B.7 integration.
func %s(amount int) int {
	return normalizeAmount(amount)
}

func normalizeAmount(amount int) int {
	if amount < 0 {
		return 0
	}
	return amount
}
`, symbol, symbol)
	mainGo := fmt.Sprintf(`package payments

func Entry() {
	_ = %s(42)
}
`, symbol)
	if err := os.WriteFile(filepath.Join(root, "src", "payments.go"), []byte(payments), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "src", "main.go"), []byte(mainGo), 0o644)
}

func seedMemory(atlasHome, projectID, title, content string) error {
	store, err := memory.Open(atlasHome, projectID)
	if err != nil {
		return err
	}
	defer store.Close()
	_, err = store.SaveObservation(memory.Observation{
		Type:    memory.TypeDecision,
		Title:   title,
		Content: content,
	})
	return err
}

func rebuildCE(root, atlasHome, projectID, name string, now time.Time, ensurePath, task string) (string, error) {
	idx, err := atlascontext.BuildIndex(root, name, now)
	if err != nil {
		return "", err
	}
	idx.ProjectID = projectID
	found := false
	for _, f := range idx.Files {
		if f.Path == ensurePath {
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("index missing %s", ensurePath)
	}
	indexYAML, err := atlascontext.RenderIndexYAML(idx)
	if err != nil {
		return "", err
	}
	capsule := atlascontext.RenderCapsule(idx)
	ensureCanary := func(pack *atlascontext.PackDocument) {
		for _, c := range pack.Candidates {
			if c.Path == ensurePath {
				return
			}
		}
		pack.Candidates = append([]atlascontext.PackCandidate{{
			Path: ensurePath, Kind: "doc", Reason: "36b7 e2e canary",
		}}, pack.Candidates...)
		if len(pack.Candidates) > 24 {
			pack.Candidates = pack.Candidates[:24]
		}
	}
	def := atlascontext.BuildPack(idx, atlascontext.DefaultPackObjective, now)
	ensureCanary(&def)
	taskPack := atlascontext.BuildPack(idx, task, now)
	ensureCanary(&taskPack)
	defYAML, err := atlascontext.RenderPackYAML(def)
	if err != nil {
		return "", err
	}
	taskYAML, err := atlascontext.RenderPackYAML(taskPack)
	if err != nil {
		return "", err
	}
	for _, p := range []struct{ path, data string }{
		{atlascontext.IndexPath(atlasHome, projectID), indexYAML},
		{atlascontext.CapsulePath(atlasHome, projectID), capsule},
		{atlascontext.PackPath(atlasHome, projectID, def.PackID), defYAML},
		{atlascontext.PackPath(atlasHome, projectID, taskPack.PackID), taskYAML},
	} {
		if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(p.path, []byte(p.data), 0o600); err != nil {
			return "", err
		}
	}
	return def.PackID, nil
}

func buildCodeGraph(projectRoot, atlasHome, projectID string) (string, error) {
	cgDir := codeintel.ProviderStorageDir(atlasHome, projectID, codeintel.ProviderCodeGraph)
	db := codeintel.GraphDBPath(atlasHome, projectID, codeintel.ProviderCodeGraph)
	if err := os.MkdirAll(cgDir, 0o700); err != nil {
		return "", err
	}
	buildRun, err := runBounded(
		context.Background(),
		defaultBuildTimeout,
		maxStdoutCapture,
		maxStderrCapture,
		"codegraph",
		[]string{"build", projectRoot, "--db", db},
		"",
		os.Environ(),
	)
	if err != nil {
		return "", fmt.Errorf("codegraph build: %w stdout=%s stderr=%s", err, redactPaths(string(buildRun.Stdout)), redactPaths(string(buildRun.Stderr)))
	}
	if _, err := os.Lstat(db); err != nil {
		return "", fmt.Errorf("graph.db missing after build: %w", err)
	}
	// Atlas-owned metadata so composition freshness is ready (setup mutation only).
	fp, err := codeintel.ComputeSourceFingerprint(projectRoot)
	if err != nil {
		return "", fmt.Errorf("codegraph fingerprint: %w", err)
	}
	meta := codeintel.Metadata{
		SchemaVersion:     codeintel.MetadataSchemaVersion,
		Provider:          string(codeintel.ProviderCodeGraph),
		ProviderVersion:   "3.17.0",
		ProjectID:         projectID,
		GraphDBPath:       db,
		RefreshedAt:       time.Now().UTC().Format(time.RFC3339),
		RefreshMode:       string(codeintel.RefreshModeIncremental),
		SourceFingerprint: fp.Value,
		NodesTotal:        1,
		FilesTotal:        1,
	}
	if err := codeintel.WriteMetadataAtomic(atlasHome, codeintel.MetadataPath(atlasHome, projectID, codeintel.ProviderCodeGraph), meta); err != nil {
		return "", fmt.Errorf("codegraph metadata: %w", err)
	}
	return cgDir, nil
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
		defaultCmdTimeout,
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

func hasCodeGraphSymbol(res composeapp.Result, symbol string) bool {
	for _, it := range res.Items {
		if it.Source != contextcompose.SourceCodeGraph {
			continue
		}
		if it.Title == symbol || strings.Contains(it.Title, symbol) || strings.Contains(it.Excerpt, symbol) || strings.Contains(it.Ref, symbol) {
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

func jsonLeaksAbsPaths(raw []byte, paths ...string) bool {
	s := string(raw)
	for _, p := range paths {
		if p != "" && strings.Contains(s, p) {
			return true
		}
	}
	return false
}

func memoryDBPath(atlasHome, projectID string) string {
	return filepath.Join(atlasHome, "projects", projectID, "memory", memory.FileMemoryDB)
}

func liveCodeGraphFiles(cgDir string) ([]string, error) {
	entries, err := os.ReadDir(cgDir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

func statusIn(got string, want ...string) bool {
	for _, w := range want {
		if got == w {
			return true
		}
	}
	return false
}
