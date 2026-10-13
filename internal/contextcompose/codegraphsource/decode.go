package codegraphsource

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel/codegraph"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
)

const (
	maxDecodedResults = contextcompose.MaxCodeGraphResults
	maxNameRunes      = 128
	maxKindRunes      = contextcompose.MaxKindRunes
	maxFileRunes      = contextcompose.MaxPathRunes
	maxRoleRunes      = 64
	maxRelated        = 6
)

// queryPayload is the smallest decode surface for `codegraph query --json`.
// Extra fields are ignored. Required shape: object with results array.
type queryPayload struct {
	Name    string          `json:"name"`
	Results []queryHit      `json:"results"`
	Raw     json.RawMessage `json:"-"`
}

type queryHit struct {
	Name    string       `json:"name"`
	Kind    string       `json:"kind"`
	File    string       `json:"file"`
	Line    int          `json:"line"`
	EndLine int          `json:"endLine"`
	Role    string       `json:"role"`
	Callers []relatedHit `json:"callers"`
	Callees []relatedHit `json:"callees"`
}

type relatedHit struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	File string `json:"file"`
	Line int    `json:"line"`
}

func decodeQueryResults(stdout []byte, limit int) ([]queryHit, error) {
	if len(bytesTrimSpace(stdout)) == 0 {
		return nil, fmt.Errorf("%w: empty stdout", codegraph.ErrInvalidJSON)
	}
	if !json.Valid(stdout) {
		return nil, fmt.Errorf("%w: malformed JSON", codegraph.ErrInvalidJSON)
	}

	var probe any
	if err := json.Unmarshal(stdout, &probe); err != nil {
		return nil, fmt.Errorf("%w: %v", codegraph.ErrInvalidJSON, err)
	}
	obj, ok := probe.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: top-level value must be an object", codegraph.ErrInvalidJSON)
	}
	rawResults, hasResults := obj["results"]
	if !hasResults {
		return nil, fmt.Errorf("%w: missing results field", codegraph.ErrInvalidJSON)
	}
	if rawResults == nil {
		return nil, nil
	}
	arr, ok := rawResults.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: results must be an array", codegraph.ErrInvalidJSON)
	}

	var payload queryPayload
	if err := json.Unmarshal(stdout, &payload); err != nil {
		return nil, fmt.Errorf("%w: %v", codegraph.ErrInvalidJSON, err)
	}
	_ = arr

	if limit <= 0 {
		limit = maxDecodedResults
	}
	if limit > maxDecodedResults {
		limit = maxDecodedResults
	}

	out := make([]queryHit, 0, min(len(payload.Results), limit))
	for _, hit := range payload.Results {
		if len(out) >= limit {
			break
		}
		norm, ok := normalizeHit(hit)
		if !ok {
			continue
		}
		out = append(out, norm)
	}
	return out, nil
}

func normalizeHit(hit queryHit) (queryHit, bool) {
	name := boundTrim(hit.Name, maxNameRunes)
	file, fileOK := acceptProjectPath(hit.File)
	if name == "" || !fileOK {
		return queryHit{}, false
	}
	kind := boundTrim(hit.Kind, maxKindRunes)
	role := boundTrim(hit.Role, maxRoleRunes)
	if hit.Line < 0 {
		hit.Line = 0
	}
	if hit.EndLine < 0 {
		hit.EndLine = 0
	}
	return queryHit{
		Name:    name,
		Kind:    kind,
		File:    file,
		Line:    hit.Line,
		EndLine: hit.EndLine,
		Role:    role,
		Callers: normalizeRelated(hit.Callers),
		Callees: normalizeRelated(hit.Callees),
	}, true
}

func normalizeRelated(in []relatedHit) []relatedHit {
	if len(in) == 0 {
		return nil
	}
	out := make([]relatedHit, 0, min(len(in), maxRelated))
	for _, r := range in {
		if len(out) >= maxRelated {
			break
		}
		name := boundTrim(r.Name, maxNameRunes)
		if name == "" {
			continue
		}
		file := ""
		if fp, ok := acceptProjectPath(r.File); ok {
			file = fp
		} else if strings.TrimSpace(r.File) != "" {
			// Related path present but unsafe/oversized: drop the path, keep name.
			file = ""
		}
		line := r.Line
		if line < 0 {
			line = 0
		}
		out = append(out, relatedHit{
			Name: name,
			Kind: boundTrim(r.Kind, maxKindRunes),
			File: file,
			Line: line,
		})
	}
	return out
}

// acceptProjectPath validates a project-relative path without truncating it.
// Oversized, absolute, traversal, or scheme/drive paths are rejected entirely.
func acceptProjectPath(p string) (string, bool) {
	p = strings.TrimSpace(p)
	if p == "" || !utf8.ValidString(p) {
		return "", false
	}
	if utf8.RuneCountInString(p) > maxFileRunes {
		return "", false
	}
	if !safeProjectPath(p) {
		return "", false
	}
	return p, true
}

// safeProjectPath rejects absolute, traversal, scheme, and Windows drive paths.
// Mirrors contextcompose.safeRelativeRef without exporting it.
func safeProjectPath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" || !utf8.ValidString(p) {
		return false
	}
	if strings.Contains(p, "://") {
		return false
	}
	if strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return false
	}
	if len(p) >= 2 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[1] == ':' {
		return false
	}
	normalized := strings.ReplaceAll(p, `\`, "/")
	for _, part := range strings.Split(normalized, "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func boundTrim(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if s == "" || !utf8.ValidString(s) {
		return ""
	}
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes])
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func hitIdentity(h queryHit) string {
	return "codegraph:" + h.Name + ":" + h.File + ":" + strconv.Itoa(h.Line)
}

func hitRef(h queryHit) string {
	if h.Line > 0 {
		return h.File + ":" + strconv.Itoa(h.Line)
	}
	return h.File
}

func hitExcerpt(h queryHit) string {
	parts := make([]string, 0, 4)
	if h.Kind != "" {
		parts = append(parts, h.Kind)
	}
	if h.Role != "" {
		parts = append(parts, "role="+h.Role)
	}
	if names := relatedNames(h.Callers); names != "" {
		parts = append(parts, "callers="+names)
	}
	if names := relatedNames(h.Callees); names != "" {
		parts = append(parts, "callees="+names)
	}
	excerpt := strings.Join(parts, "; ")
	if excerpt == "" {
		excerpt = "CodeGraph symbol " + h.Name
	}
	if utf8.RuneCountInString(excerpt) > contextcompose.MaxExcerptChars {
		runes := []rune(excerpt)
		excerpt = string(runes[:contextcompose.MaxExcerptChars-1]) + "…"
	}
	return excerpt
}

func relatedNames(in []relatedHit) string {
	if len(in) == 0 {
		return ""
	}
	names := make([]string, 0, len(in))
	for _, r := range in {
		if r.Name != "" {
			names = append(names, r.Name)
		}
	}
	return strings.Join(names, ",")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
