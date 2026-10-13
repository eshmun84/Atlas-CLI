package codegraphsource

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
	"github.com/eshmun84/Atlas-CLI/internal/codeintel/codegraph"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

const (
	defaultQueryTimeout = 5 * time.Second
	defaultMaxOutput    = 256 << 10 // 256 KiB — reject before unbounded decode
)

// Input drives read-only CodeGraph candidate retrieval for composition.
//
// Callers supply a composition-oriented request; this adapter never accepts
// arbitrary database paths. Project identity is resolved only through Env
// (workspace root + Atlas Home). Optional LookPath/Runner hooks support tests.
type Input struct {
	Task     string
	Query    string
	MaxItems int           // 0 → contextcompose.MaxCodeGraphResults
	Timeout  time.Duration // 0 → defaultQueryTimeout
	Env      memoryretrieve.Env

	// Test hooks — nil uses Adapter defaults (exec.LookPath + ExecRunner).
	LookPath codegraph.LookPathFunc
	Runner   codegraph.Runner
}

// Result holds bounded CodeGraph candidates and Composer-compatible status.
type Result struct {
	Candidates   []contextcompose.Candidate
	SourceStatus contextcompose.SourceStatus
	ProjectID    string
	Freshness    string
	QuerySymbol  string
	Message      string
}

// Retrieve probes CodeGraph availability, queries the Atlas-owned graph.db
// when usable, and maps hits to Candidates. Never builds or mutates the graph.
func Retrieve(in Input) Result {
	ref, err := resolveProject(in.Env)
	if err != nil {
		return mapResolveError(err)
	}

	task := strings.TrimSpace(in.Task)
	if task == "" {
		task = strings.TrimSpace(in.Query)
	}
	if task == "" {
		return failed(ref.ProjectID, "", "task or query is required")
	}
	if !utf8.ValidString(task) {
		return failed(ref.ProjectID, "", "task/query must be valid UTF-8")
	}
	if utf8.RuneCountInString(task) > contextcompose.MaxTaskRunes {
		return failed(ref.ProjectID, "", "task exceeds maximum length")
	}

	limit := in.MaxItems
	switch {
	case limit < 0:
		return failed(ref.ProjectID, "", "max_items must be non-negative")
	case limit == 0:
		limit = contextcompose.MaxCodeGraphResults
	case limit > contextcompose.MaxCodeGraphResults:
		limit = contextcompose.MaxCodeGraphResults
	}

	adapter := &codegraph.Adapter{
		LookPath: in.LookPath,
		Runner:   in.Runner,
	}
	if adapter.LookPath == nil || adapter.Runner == nil {
		def := codegraph.New()
		if adapter.LookPath == nil {
			adapter.LookPath = def.LookPath
		}
		if adapter.Runner == nil {
			adapter.Runner = codegraph.ExecRunner{
				Timeout:   timeoutOrDefault(in.Timeout),
				MaxOutput: defaultMaxOutput,
			}
		}
	}

	timeout := timeoutOrDefault(in.Timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	project := codeintel.Project{
		Root:     ref.Root,
		ID:       ref.ProjectID,
		HomePath: ref.Home,
	}
	st, statusErr := adapter.Status(ctx, project)
	if statusErr != nil {
		return failed(ref.ProjectID, "", "codegraph status failed")
	}

	snap := codeintel.EnrichSnapshot(project, st)
	freshness := string(snap.Freshness)
	switch snap.State {
	case codeintel.StateUnavailable, codeintel.StateMissing:
		return unavailable(ref.ProjectID, freshness, messageOr(snap.Message, "codegraph executable not available"))
	case codeintel.StateIncompatible:
		return failed(ref.ProjectID, freshness, messageOr(snap.Message, "codegraph version incompatible"))
	case codeintel.StateError:
		return failed(ref.ProjectID, freshness, messageOr(snap.Message, "codegraph provider error"))
	}

	if !snap.GraphPresent {
		return unavailable(ref.ProjectID, freshness, "codegraph graph.db not present")
	}

	dbPath := strings.TrimSpace(snap.GraphDBPath)
	if dbPath == "" {
		return unavailable(ref.ProjectID, freshness, "codegraph graph.db path unresolved")
	}
	if _, leafErr := codeintel.InspectStorageLeaf(ref.Home, dbPath); leafErr != nil {
		return failed(ref.ProjectID, freshness, "codegraph graph.db unreadable")
	}

	symbol, ok := deriveQuerySymbol(task)
	if !ok {
		return noCandidates(ref.ProjectID, freshness, "", "no symbol query derived from task")
	}

	execPath := strings.TrimSpace(snap.Executable)
	if execPath == "" {
		// Status already probed; re-resolve for the query invocation.
		look := adapter.LookPath
		path, lookErr := look(codegraph.ExecutableName)
		if lookErr != nil || strings.TrimSpace(path) == "" {
			return unavailable(ref.ProjectID, freshness, "codegraph executable not available")
		}
		execPath = path
	}

	runner := adapter.Runner
	if runner == nil {
		runner = codegraph.ExecRunner{Timeout: timeout, MaxOutput: defaultMaxOutput}
	}

	// Query an isolated snapshot so CodeGraph/SQLite never creates WAL/SHM
	// beside the Atlas-managed live graph.db.
	qsnap, snapCleanup, snapErr := prepareQuerySnapshot(dbPath)
	if snapErr != nil {
		return failed(ref.ProjectID, freshness, "codegraph query snapshot failed").withSymbol(symbol)
	}
	defer snapCleanup()

	args := codegraph.QueryLimitArgs(qsnap.DB, symbol, limit)
	runResult, runErr := runner.Run(ctx, execPath, args)
	if runErr != nil {
		return mapRunError(ref.ProjectID, freshness, symbol, runErr)
	}
	if runResult.Truncated {
		return failed(ref.ProjectID, freshness, "codegraph query output exceeded bound").withSymbol(symbol)
	}

	hits, decodeErr := decodeQueryResults(runResult.Stdout, limit)
	if decodeErr != nil {
		return failed(ref.ProjectID, freshness, "codegraph query returned malformed JSON").withSymbol(symbol)
	}
	if len(hits) == 0 {
		msg := "codegraph query returned no results"
		if snap.Freshness == codeintel.FreshnessStale {
			msg = msg + "; graph is stale"
		}
		return noCandidates(ref.ProjectID, freshness, symbol, msg)
	}

	cands := toCandidates(hits)
	msg := "codegraph query returned symbol results"
	if snap.Freshness == codeintel.FreshnessStale {
		msg = msg + "; graph is stale"
	}
	return Result{
		Candidates:  cands,
		ProjectID:   ref.ProjectID,
		Freshness:   freshness,
		QuerySymbol: symbol,
		Message:     msg,
		SourceStatus: contextcompose.SourceStatus{
			Source:  contextcompose.SourceCodeGraph,
			Status:  contextcompose.SourceStatusSupplied,
			Message: boundStatusMessage(msg),
		},
	}
}

func toCandidates(hits []queryHit) []contextcompose.Candidate {
	out := make([]contextcompose.Candidate, 0, len(hits))
	for i, h := range hits {
		score := (len(hits) - i) * 10
		out = append(out, contextcompose.Candidate{
			Source:     contextcompose.SourceCodeGraph,
			Kind:       kindOr(h.Kind, "symbol"),
			ID:         hitIdentity(h),
			Title:      h.Name,
			Excerpt:    hitExcerpt(h),
			Path:       h.File,
			Ref:        hitRef(h),
			Score:      score,
			Provenance: "codegraph",
		})
	}
	return out
}

func mapRunError(projectID, freshness, symbol string, err error) Result {
	switch {
	case errors.Is(err, codegraph.ErrTimeout), errors.Is(err, context.DeadlineExceeded):
		return failed(projectID, freshness, "codegraph query timed out").withSymbol(symbol)
	case errors.Is(err, codegraph.ErrOutputExceeded):
		return failed(projectID, freshness, "codegraph query output exceeded bound").withSymbol(symbol)
	case errors.Is(err, codegraph.ErrProcessFailed):
		return failed(projectID, freshness, "codegraph query process failed").withSymbol(symbol)
	default:
		return failed(projectID, freshness, "codegraph query failed").withSymbol(symbol)
	}
}

type projectRef struct {
	Root      string
	Home      string
	ProjectID string
}

func resolveProject(env memoryretrieve.Env) (projectRef, error) {
	root := strings.TrimSpace(env.Root)
	if root == "" {
		getwd := env.Getwd
		if getwd == nil {
			getwd = os.Getwd
		}
		wd, err := getwd()
		if err != nil {
			return projectRef{}, fmt.Errorf("codegraphsource: working directory: %w", err)
		}
		root = wd
	}

	cfg, err := config.LoadAt(root)
	if err != nil {
		return projectRef{}, mapConfigLoadError(err)
	}
	name := strings.TrimSpace(cfg.Project.Name)
	if name == "" {
		return projectRef{}, memoryretrieve.ErrNotInitialized
	}
	projectID, err := home.ProjectID(root, name)
	if err != nil {
		return projectRef{}, err
	}

	homePath := strings.TrimSpace(env.Home)
	if homePath == "" {
		homePath, err = home.Resolve()
		if err != nil {
			return projectRef{}, err
		}
	}
	return projectRef{Root: root, Home: homePath, ProjectID: projectID}, nil
}

func mapConfigLoadError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("%w: %w", memoryretrieve.ErrNotInitialized, err)
	case errors.Is(err, fsafety.ErrSymlink),
		errors.Is(err, fsafety.ErrPathTraversal),
		errors.Is(err, fsafety.ErrPathEscape),
		errors.Is(err, fsafety.ErrAbsolutePath),
		errors.Is(err, fsafety.ErrNotRegularFile):
		return fmt.Errorf("%w: %w", memoryretrieve.ErrUnsafeFilesystem, err)
	default:
		return err
	}
}

func mapResolveError(err error) Result {
	if err == nil {
		return failed("", "", "codegraph request failed")
	}
	switch {
	case errors.Is(err, memoryretrieve.ErrNotInitialized),
		errors.Is(err, os.ErrNotExist):
		return unavailable("", "", "atlas project not initialized")
	case errors.Is(err, memoryretrieve.ErrUnsafeFilesystem),
		errors.Is(err, fsafety.ErrSymlink),
		errors.Is(err, fsafety.ErrPathTraversal),
		errors.Is(err, fsafety.ErrPathEscape),
		errors.Is(err, fsafety.ErrAbsolutePath),
		errors.Is(err, fsafety.ErrNotRegularFile):
		return failed("", "", "filesystem path unsafe")
	default:
		return failed("", "", "codegraph request failed")
	}
}

func unavailable(projectID, freshness, message string) Result {
	return Result{
		ProjectID: projectID,
		Freshness: freshness,
		Message:   message,
		SourceStatus: contextcompose.SourceStatus{
			Source:  contextcompose.SourceCodeGraph,
			Status:  contextcompose.SourceStatusUnavailable,
			Message: boundStatusMessage(message),
		},
	}
}

func noCandidates(projectID, freshness, symbol, message string) Result {
	return Result{
		ProjectID:   projectID,
		Freshness:   freshness,
		QuerySymbol: symbol,
		Message:     message,
		SourceStatus: contextcompose.SourceStatus{
			Source:  contextcompose.SourceCodeGraph,
			Status:  contextcompose.SourceStatusNoCandidates,
			Message: boundStatusMessage(message),
		},
	}
}

func failed(projectID, freshness, message string) Result {
	return Result{
		ProjectID: projectID,
		Freshness: freshness,
		Message:   message,
		SourceStatus: contextcompose.SourceStatus{
			Source:  contextcompose.SourceCodeGraph,
			Status:  contextcompose.SourceStatusFailed,
			Message: boundStatusMessage(message),
		},
	}
}

func (r Result) withSymbol(symbol string) Result {
	r.QuerySymbol = symbol
	return r
}

func timeoutOrDefault(d time.Duration) time.Duration {
	if d <= 0 {
		return defaultQueryTimeout
	}
	return d
}

func kindOr(kind, fallback string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return fallback
	}
	return kind
}

func messageOr(msg, fallback string) string {
	if strings.TrimSpace(msg) != "" {
		return msg
	}
	return fallback
}

func boundStatusMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	if utf8.RuneCountInString(msg) <= contextcompose.MaxSourceMessageRunes {
		return msg
	}
	runes := []rune(msg)
	return string(runes[:contextcompose.MaxSourceMessageRunes])
}
