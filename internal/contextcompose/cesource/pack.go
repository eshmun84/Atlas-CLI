package cesource

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
	"gopkg.in/yaml.v3"
)

// Pack schema version matches context.packSchemaVersion (unexported, currently 1).
const packSchemaVersion = 1

// MaxStoredPackCandidates is the hard bound for an on-disk pack candidate list.
// Matches context.BuildPack / contextcompose.MaxCEPaths.
const MaxStoredPackCandidates = contextcompose.MaxCEPaths

type packOutcome int

const (
	packMissing packOutcome = iota
	packOK
	packFailed
)

type packLoadResult struct {
	Outcome    packOutcome
	Candidates []context.PackCandidate
	Message    string
	Provenance string // context_economy:pack when from disk
}

// resolvePackCandidates loads a stored pack for the task objective, then the
// default objective. Only a genuine miss on both may fall back to BuildPack.
// Unsafe/corrupt/incompatible packs fail closed — never silent recovery.
func resolvePackCandidates(homePath, projectID, task string, idx context.IndexDocument) (cands []context.PackCandidate, provenance string, failMsg string, ok bool) {
	objectives := []string{task}
	if task != context.DefaultPackObjective {
		objectives = append(objectives, context.DefaultPackObjective)
	}

	for _, objective := range objectives {
		res := loadStoredPack(homePath, projectID, objective, idx)
		switch res.Outcome {
		case packOK:
			return res.Candidates, res.Provenance, "", true
		case packFailed:
			return nil, "", res.Message, false
		case packMissing:
			// try next objective / BuildPack fallback
		}
	}

	// Genuine miss — derive from the already-loaded index (no ApplyUpdate).
	pack := context.BuildPack(idx, task, time.Unix(0, 0).UTC())
	return pack.Candidates, "context_economy:index", "", true
}

func loadStoredPack(homePath, projectID, objective string, idx context.IndexDocument) packLoadResult {
	objective = strings.TrimSpace(objective)
	if objective == "" {
		objective = context.DefaultPackObjective
	}
	expectedID := packIDFor(objective)
	rel := context.ContextPackRel(projectID, expectedID)

	present, err := context.InspectContextLeaf(homePath, rel)
	if err != nil {
		return packLoadResult{
			Outcome: packFailed,
			Message: "context economy pack filesystem unsafe",
		}
	}
	if !present {
		return packLoadResult{Outcome: packMissing}
	}

	data, err := readPackBounded(homePath, rel)
	if err != nil {
		return packLoadResult{
			Outcome: packFailed,
			Message: classifyPackReadError(err),
		}
	}

	var pack context.PackDocument
	if err := yaml.Unmarshal(data, &pack); err != nil {
		return packLoadResult{
			Outcome: packFailed,
			Message: "context economy pack yaml invalid",
		}
	}

	if msg := validateStoredPack(pack, projectID, expectedID, objective, idx); msg != "" {
		return packLoadResult{
			Outcome: packFailed,
			Message: msg,
		}
	}

	return packLoadResult{
		Outcome:    packOK,
		Candidates: pack.Candidates,
		Provenance: "context_economy:pack",
	}
}

func validateStoredPack(pack context.PackDocument, projectID, expectedPackID, expectedObjective string, idx context.IndexDocument) string {
	switch {
	case pack.SchemaVersion == 0:
		return "context economy pack schema missing"
	case pack.SchemaVersion != packSchemaVersion:
		return "context economy pack schema incompatible"
	}

	gotProject := strings.TrimSpace(pack.ProjectID)
	if gotProject == "" {
		return "context economy pack project identity missing"
	}
	if gotProject != projectID {
		return "context economy pack project identity mismatch"
	}
	// Cross-project: pack claims this project but index belongs elsewhere.
	if idxProject := strings.TrimSpace(idx.ProjectID); idxProject != "" && idxProject != projectID {
		return "context economy pack project identity mismatch"
	}

	gotPackID := strings.TrimSpace(pack.PackID)
	if gotPackID == "" {
		return "context economy pack identity missing"
	}
	if gotPackID != expectedPackID {
		return "context economy pack identity mismatch"
	}

	gotObjective := strings.TrimSpace(pack.Objective)
	if gotObjective == "" {
		return "context economy pack objective missing"
	}
	if gotObjective != strings.TrimSpace(expectedObjective) {
		return "context economy pack objective mismatch"
	}

	if len(pack.Candidates) > MaxStoredPackCandidates {
		return "context economy pack candidate list exceeds maximum"
	}

	indexed := indexedPathSet(idx)
	for i, c := range pack.Candidates {
		path := strings.TrimSpace(c.Path)
		if path == "" {
			return fmt.Sprintf("context economy pack candidate %d missing path", i)
		}
		if unsafePackPath(path) {
			return "context economy pack candidate path unsafe"
		}
		if len([]rune(path)) > contextcompose.MaxPathRunes {
			return "context economy pack candidate path exceeds maximum"
		}
		if len([]rune(strings.TrimSpace(c.Reason))) > contextcompose.MaxExcerptChars {
			return "context economy pack candidate reason exceeds maximum"
		}
		if len([]rune(strings.TrimSpace(c.Kind))) > contextcompose.MaxKindRunes {
			return "context economy pack candidate kind exceeds maximum"
		}
		if !indexed[path] {
			return "context economy pack candidate path not in index"
		}
	}
	return ""
}

// indexedPathSet returns project-relative paths present on the loaded index.
// Mirrors BuildPack membership: Files plus governance/runtime surfaces
// (RuntimeAtlas, Contracts) and other indexed category lists. No FS walk.
func indexedPathSet(idx context.IndexDocument) map[string]bool {
	out := map[string]bool{}
	add := func(paths ...string) {
		for _, p := range paths {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			out[p] = true
		}
	}
	for _, f := range idx.Files {
		add(f.Path)
	}
	add(idx.RuntimeAtlas...)
	add(idx.Contracts...)
	add(idx.ConfigFiles...)
	add(idx.TestFiles...)
	add(idx.Docs...)
	add(idx.Manifests...)
	add(idx.Entrypoints...)
	return out
}

func unsafePackPath(path string) bool {
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\`) || strings.Contains(path, "://") {
		return true
	}
	if len(path) >= 2 && ((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) && path[1] == ':' {
		return true
	}
	normalized := strings.ReplaceAll(path, `\`, "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func classifyPackReadError(err error) string {
	switch {
	case err == nil:
		return "context economy pack read failed"
	case errors.Is(err, errPackOversized):
		return "context economy pack exceeds maximum size"
	case errors.Is(err, os.ErrNotExist):
		// Inspect said present; disappearance mid-read is still a failure, not a miss.
		return "context economy pack read failed"
	case errors.Is(err, fsafety.ErrSymlink),
		errors.Is(err, fsafety.ErrPathTraversal),
		errors.Is(err, fsafety.ErrPathEscape),
		errors.Is(err, fsafety.ErrAbsolutePath),
		errors.Is(err, fsafety.ErrNotRegularFile):
		return "context economy pack filesystem unsafe"
	default:
		return "context economy pack read failed"
	}
}

// packIDFor mirrors context.packIDFor (unexported) for locating stored packs.
func packIDFor(objective string) string {
	cleaned := strings.ToLower(strings.TrimSpace(objective))
	var b strings.Builder
	lastDash := false
	for _, r := range cleaned {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = "pack"
	}
	if len(id) > 48 {
		id = id[:48]
		id = strings.Trim(id, "-")
	}
	return id
}
