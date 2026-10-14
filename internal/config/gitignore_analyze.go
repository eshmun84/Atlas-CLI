package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// GitignoreAnalysisState classifies Atlas local-governance .gitignore health.
type GitignoreAnalysisState string

const (
	GitignoreNotApplicable GitignoreAnalysisState = "not_applicable"
	GitignoreHealthy       GitignoreAnalysisState = "healthy"
	GitignoreMissingFile   GitignoreAnalysisState = "missing_file"
	GitignoreMissingBlock  GitignoreAnalysisState = "missing_block"
	GitignoreStale         GitignoreAnalysisState = "stale"
	GitignoreMultiple      GitignoreAnalysisState = "multiple"
	GitignoreMalformed     GitignoreAnalysisState = "malformed"
)

// GitignoreAnalysis is a read-only classification of .gitignore Atlas blocks.
type GitignoreAnalysis struct {
	Applicable   bool
	State        GitignoreAnalysisState
	BlockCount   int
	Reconcilable bool
	Reason       string
}

// AnalyzeLocalOnlyGitignore inspects .gitignore using the same marker parser as
// Init/Repair reconciliation. Read-only: never writes.
func AnalyzeLocalOnlyGitignore(root string, doc ProjectDocument) (GitignoreAnalysis, error) {
	if !NeedsLocalOnlyGitignore(doc) {
		return GitignoreAnalysis{State: GitignoreNotApplicable}, nil
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return GitignoreAnalysis{}, fmt.Errorf("gitignore: workspace root is required")
	}

	full, err := fsafety.ContainedJoin(root, FileGitignore)
	if err != nil {
		return GitignoreAnalysis{}, err
	}
	existing, readErr := os.ReadFile(full)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return GitignoreAnalysis{
				Applicable:   true,
				State:        GitignoreMissingFile,
				Reconcilable: true,
				Reason:       ".gitignore missing; Atlas local governance block required",
			}, nil
		}
		return GitignoreAnalysis{}, fmt.Errorf("gitignore: read: %w", readErr)
	}

	spans, err := findAtlasGitignoreSpans(string(existing))
	if err != nil {
		return GitignoreAnalysis{
			Applicable:   true,
			State:        GitignoreMalformed,
			Reconcilable: false,
			Reason:       err.Error(),
		}, nil
	}

	out := GitignoreAnalysis{Applicable: true, BlockCount: len(spans)}
	switch {
	case len(spans) == 0:
		out.State = GitignoreMissingBlock
		out.Reconcilable = true
		out.Reason = "no Atlas local governance block"
	case len(spans) > 1:
		out.State = GitignoreMultiple
		out.Reconcilable = true
		out.Reason = fmt.Sprintf("%d Atlas local governance blocks (should consolidate to one)", len(spans))
	default:
		body := string(existing[spans[0].begin:spans[0].end])
		canonical := AtlasLocalGovernanceGitignoreBlock()
		if normalizeGitignoreBlock(body) == normalizeGitignoreBlock(canonical) {
			desired, mergeErr := mergeAtlasGitignoreBlock(string(existing))
			if mergeErr != nil {
				out.State = GitignoreMalformed
				out.Reason = mergeErr.Error()
				return out, nil
			}
			if desired == string(existing) {
				out.State = GitignoreHealthy
				out.Reason = "canonical Atlas local governance block present"
				return out, nil
			}
		}
		out.State = GitignoreStale
		out.Reconcilable = true
		out.Reason = "Atlas local governance block stale or incomplete"
	}
	return out, nil
}

func normalizeGitignoreBlock(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
}
