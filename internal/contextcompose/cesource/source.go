package cesource

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// Input drives read-only Context Economy candidate retrieval for composition.
type Input struct {
	Task     string
	Query    string
	MaxItems int // 0 → contextcompose.MaxCEPaths
	Env      memoryretrieve.Env
}

// Result holds CE path/capsule candidates and Composer-compatible status.
type Result struct {
	Candidates   []contextcompose.Candidate
	SourceStatus contextcompose.SourceStatus
	ProjectID    string
	CEState      string // context.Status* vocabulary when known
	Message      string
}

// Retrieve loads existing CE artifacts and maps them to Candidates.
func Retrieve(in Input) Result {
	ref, err := resolveProject(in.Env)
	if err != nil {
		return mapResolveError(err)
	}

	indexRel, capsuleRel, resolveErr := resolveRels(ref.Home, ref.ProjectID)
	if resolveErr != nil {
		return failed(ref.ProjectID, context.StatusUnreadable, "context economy artifacts unreadable")
	}

	indexPresent, indexErr := context.InspectContextLeaf(ref.Home, indexRel)
	if indexErr != nil {
		return failed(ref.ProjectID, context.StatusUnreadable, "context economy index unreadable")
	}
	if !indexPresent {
		return unavailable(ref.ProjectID, context.StatusMissing, "context economy index missing")
	}

	idx, present, loadErr := context.LoadIndexAt(ref.Home, ref.ProjectID)
	if loadErr != nil {
		return failed(ref.ProjectID, context.StatusUnreadable, "context economy index unreadable")
	}
	if !present {
		return unavailable(ref.ProjectID, context.StatusMissing, "context economy index missing")
	}

	capsulePresent, capsuleErr := context.InspectContextLeaf(ref.Home, capsuleRel)
	if capsuleErr != nil {
		return failed(ref.ProjectID, context.StatusUnreadable, "context economy capsule unreadable")
	}

	ceState := context.StatusPresent
	msg := "context economy artifacts present"
	if !capsulePresent {
		ceState = context.StatusStale
		msg = "context economy index present but capsule missing"
	} else {
		stale, staleErr := context.IsStale(ref.Root, idx)
		if staleErr != nil {
			return failed(ref.ProjectID, context.StatusUnreadable, "unable to evaluate context economy freshness")
		}
		if stale {
			ceState = context.StatusStale
			msg = context.DescribeStale(true)
		}
	}

	task := strings.TrimSpace(in.Task)
	if task == "" {
		task = strings.TrimSpace(in.Query)
	}
	if task == "" {
		task = context.DefaultPackObjective
	}

	cands, buildMsg, buildOK := buildCandidates(ref.Home, ref.ProjectID, idx, task, in.MaxItems, capsulePresent)
	if !buildOK {
		return failed(ref.ProjectID, context.StatusUnreadable, buildMsg)
	}
	if len(cands) == 0 {
		return Result{
			ProjectID: ref.ProjectID,
			CEState:   ceState,
			Message:   messageOr(msg, "context economy returned no candidates"),
			SourceStatus: contextcompose.SourceStatus{
				Source:  contextcompose.SourceContextEconomy,
				Status:  contextcompose.SourceStatusNoCandidates,
				Message: messageOr(msg, "context economy returned no candidates"),
			},
		}
	}

	statusMsg := msg
	if ceState == context.StatusStale {
		statusMsg = msg
	}
	return Result{
		Candidates: cands,
		ProjectID:  ref.ProjectID,
		CEState:    ceState,
		Message:    statusMsg,
		SourceStatus: contextcompose.SourceStatus{
			Source:  contextcompose.SourceContextEconomy,
			Status:  contextcompose.SourceStatusSupplied,
			Message: statusMsg,
		},
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
			return projectRef{}, fmt.Errorf("cesource: working directory: %w", err)
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

func resolveRels(homePath, projectID string) (indexRel, capsuleRel string, err error) {
	// Prefer canonical when present or unsafe; fall back to legacy only when
	// canonical is cleanly absent — mirrors context.resolveContextRels.
	canonIdx := context.ContextIndexRel(projectID)
	present, err := context.InspectContextLeaf(homePath, canonIdx)
	if err != nil {
		return canonIdx, context.ContextCapsuleRel(projectID), err
	}
	if present {
		return canonIdx, context.ContextCapsuleRel(projectID), nil
	}
	legacyIdx := context.LegacyContextIndexRel(projectID)
	legacyPresent, legacyErr := context.InspectContextLeaf(homePath, legacyIdx)
	if legacyErr != nil {
		return legacyIdx, context.LegacyContextCapsuleRel(projectID), legacyErr
	}
	if legacyPresent {
		return legacyIdx, context.LegacyContextCapsuleRel(projectID), nil
	}
	return canonIdx, context.ContextCapsuleRel(projectID), nil
}

func buildCandidates(homePath, projectID string, idx context.IndexDocument, task string, maxItems int, capsulePresent bool) (cands []contextcompose.Candidate, failMsg string, ok bool) {
	limit := maxItems
	if limit <= 0 {
		limit = contextcompose.MaxCEPaths
	}
	if limit > contextcompose.MaxCEPaths {
		limit = contextcompose.MaxCEPaths
	}

	packCands, provenance, failMsg, ok := resolvePackCandidates(homePath, projectID, task, idx)
	if !ok {
		return nil, failMsg, false
	}

	out := make([]contextcompose.Candidate, 0, limit+1)
	if capsulePresent {
		out = append(out, capsuleCandidate(idx))
	}

	seen := map[string]bool{}
	for i, pc := range packCands {
		path := strings.TrimSpace(pc.Path)
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		score := (len(packCands) - i) * 10
		out = append(out, contextcompose.Candidate{
			Source:     contextcompose.SourceContextEconomy,
			Kind:       kindOr(pc.Kind, "path"),
			ID:         "path:" + path,
			Title:      path,
			Excerpt:    strings.TrimSpace(pc.Reason),
			Path:       path,
			Score:      score,
			Provenance: provenance,
		})
		if countPaths(out) >= limit {
			break
		}
	}
	return out, "", true
}

func countPaths(cands []contextcompose.Candidate) int {
	n := 0
	for _, c := range cands {
		if c.Kind != "capsule" && c.Path != "" {
			n++
		}
	}
	return n
}

func capsuleCandidate(idx context.IndexDocument) contextcompose.Candidate {
	var parts []string
	if name := strings.TrimSpace(idx.ProjectName); name != "" {
		parts = append(parts, "project "+name)
	}
	if len(idx.Languages) > 0 {
		parts = append(parts, "languages: "+strings.Join(boundStrings(idx.Languages, 6), ", "))
	}
	if len(idx.Frameworks) > 0 {
		parts = append(parts, "frameworks: "+strings.Join(boundStrings(idx.Frameworks, 4), ", "))
	}
	if len(idx.Entrypoints) > 0 {
		parts = append(parts, "entrypoints: "+strings.Join(boundStrings(idx.Entrypoints, 4), ", "))
	}
	excerpt := strings.Join(parts, "; ")
	if utf8.RuneCountInString(excerpt) > contextcompose.MaxExcerptChars {
		runes := []rune(excerpt)
		excerpt = string(runes[:contextcompose.MaxExcerptChars-1]) + "…"
	}
	prov := "capsule"
	if fp := strings.TrimSpace(idx.Fingerprint); fp != "" {
		if len(fp) > 16 {
			fp = fp[:16]
		}
		prov = "capsule:" + fp
	}
	return contextcompose.Candidate{
		Source:     contextcompose.SourceContextEconomy,
		Kind:       "capsule",
		ID:         "capsule",
		Title:      "Context Economy capsule",
		Excerpt:    excerpt,
		Score:      1200,
		Provenance: prov,
	}
}

func boundStrings(in []string, max int) []string {
	if len(in) <= max {
		return in
	}
	return in[:max]
}

func kindOr(kind, fallback string) string {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return fallback
	}
	return kind
}

func mapResolveError(err error) Result {
	if err == nil {
		return failed("", "", "context economy request failed")
	}
	switch {
	case errors.Is(err, memoryretrieve.ErrNotInitialized),
		errors.Is(err, os.ErrNotExist):
		return unavailable("", context.StatusNA, "atlas project not initialized")
	case errors.Is(err, memoryretrieve.ErrUnsafeFilesystem),
		errors.Is(err, fsafety.ErrSymlink),
		errors.Is(err, fsafety.ErrPathTraversal),
		errors.Is(err, fsafety.ErrPathEscape),
		errors.Is(err, fsafety.ErrAbsolutePath),
		errors.Is(err, fsafety.ErrNotRegularFile):
		return failed("", context.StatusUnreadable, "filesystem path unsafe")
	default:
		return failed("", context.StatusUnreadable, "context economy request failed")
	}
}

func unavailable(projectID, ceState, message string) Result {
	return Result{
		ProjectID: projectID,
		CEState:   ceState,
		Message:   message,
		SourceStatus: contextcompose.SourceStatus{
			Source:  contextcompose.SourceContextEconomy,
			Status:  contextcompose.SourceStatusUnavailable,
			Message: message,
		},
	}
}

func failed(projectID, ceState, message string) Result {
	return Result{
		ProjectID: projectID,
		CEState:   ceState,
		Message:   message,
		SourceStatus: contextcompose.SourceStatus{
			Source:  contextcompose.SourceContextEconomy,
			Status:  contextcompose.SourceStatusFailed,
			Message: message,
		},
	}
}

func messageOr(msg, fallback string) string {
	if strings.TrimSpace(msg) != "" {
		return msg
	}
	return fallback
}
