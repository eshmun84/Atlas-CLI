package context

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

// PackDocument is a simple task-oriented context pack.
type PackDocument struct {
	SchemaVersion int             `yaml:"schema_version"`
	PackID        string          `yaml:"pack_id"`
	ProjectID     string          `yaml:"project_id"`
	Objective     string          `yaml:"objective"`
	CreatedAt     string          `yaml:"created_at"`
	Candidates    []PackCandidate `yaml:"candidates"`
	Limits        []string        `yaml:"limits"`
}

// PackCandidate is one suggested path with a brief reason.
type PackCandidate struct {
	Path   string `yaml:"path"`
	Reason string `yaml:"reason"`
	Kind   string `yaml:"kind,omitempty"`
}

const packSchemaVersion = 1

// DefaultPackObjective is used when the TUI updates context without a custom task.
const DefaultPackObjective = "Orient to Atlas-governed project work with minimal context"

// BuildPack creates a deterministic rule-based pack from an index and objective.
func BuildPack(idx IndexDocument, objective string, now time.Time) PackDocument {
	obj := strings.TrimSpace(objective)
	if obj == "" {
		obj = DefaultPackObjective
	}
	tokens := tokenize(obj)
	candidates := rankCandidates(idx, tokens)
	limits := []string{
		"Pack is rule-based; not semantic search.",
		"Paths listed were present in the index at pack creation.",
		"Do not invent file contents; open listed paths only as needed.",
		"Ignored/noise paths were not indexed (node_modules, .git, backups, large binaries, etc.).",
	}
	if idx.Truncated {
		limits = append(limits, "Index was truncated; some repo paths are not indexed.")
	}
	if len(candidates) == 0 {
		limits = append(limits, "No strong candidates matched the objective; start from capsule and AGENTS.md.")
	}
	id := packIDFor(obj)
	return PackDocument{
		SchemaVersion: packSchemaVersion,
		PackID:        id,
		ProjectID:     idx.ProjectID,
		Objective:     obj,
		CreatedAt:     now.UTC().Format(time.RFC3339),
		Candidates:    candidates,
		Limits:        limits,
	}
}

// RenderPackYAML marshals a pack document.
func RenderPackYAML(pack PackDocument) (string, error) {
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&pack); err != nil {
		_ = enc.Close()
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func packIDFor(objective string) string {
	cleaned := strings.ToLower(objective)
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

func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	stop := map[string]bool{
		"a": true, "an": true, "the": true, "to": true, "for": true, "and": true,
		"or": true, "of": true, "in": true, "on": true, "with": true, "this": true,
		"that": true, "into": true, "from": true,
	}
	var out []string
	seen := map[string]bool{}
	for _, f := range fields {
		if len(f) < 2 || stop[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

type scored struct {
	c     PackCandidate
	score int
}

func rankCandidates(idx IndexDocument, tokens []string) []PackCandidate {
	var scoredList []scored
	add := func(path, kind, reason string, score int) {
		if path == "" {
			return
		}
		scoredList = append(scoredList, scored{
			c:     PackCandidate{Path: path, Kind: kind, Reason: reason},
			score: score,
		})
	}

	// Always prefer governance surfaces.
	for _, p := range unique(append(append([]string{}, idx.RuntimeAtlas...), idx.Contracts...)) {
		kind := "runtime_atlas"
		reason := "Atlas runtime/governance surface"
		if strings.Contains(p, "contracts/") {
			kind = "contract"
			reason = "operational contract"
		}
		score := 100
		if p == "AGENTS.md" {
			score = 120
			reason = "project authority contract"
		}
		add(p, kind, reason, score)
	}

	for _, f := range idx.Files {
		score := kindBaseScore(f.Kind)
		reason := "indexed " + f.Kind
		pathLower := strings.ToLower(f.Path)
		base := strings.ToLower(filepath.Base(f.Path))
		for _, tok := range tokens {
			if strings.Contains(pathLower, tok) || strings.Contains(base, tok) {
				score += 25
				reason = fmt.Sprintf("matches intent token %q", tok)
			}
		}
		switch {
		case containsAny(tokens, "test", "verify", "check") && f.Kind == "test":
			score += 40
			reason = "test surface for verification intent"
		case containsAny(tokens, "doc", "docs", "readme") && f.Kind == "doc":
			score += 35
			reason = "documentation for docs intent"
		case containsAny(tokens, "init", "setup", "config") && (f.Kind == "config" || f.Kind == "runtime_atlas"):
			score += 30
			reason = "config/runtime for setup intent"
		case containsAny(tokens, "sdd", "openspec", "spec", "propose", "implement") && f.Kind == "contract":
			score += 45
			reason = "SDD/OpenSpec contract for phase work"
		case containsAny(tokens, "agent", "orchestrat") && strings.Contains(pathLower, "agents/"):
			score += 35
			reason = "agent pack for orchestration intent"
		}
		if score >= 40 || f.Kind == "entrypoint" || f.Kind == "manifest" {
			add(f.Path, f.Kind, reason, score)
		}
	}

	sort.SliceStable(scoredList, func(i, j int) bool {
		if scoredList[i].score != scoredList[j].score {
			return scoredList[i].score > scoredList[j].score
		}
		return scoredList[i].c.Path < scoredList[j].c.Path
	})

	seen := map[string]bool{}
	var out []PackCandidate
	for _, s := range scoredList {
		if seen[s.c.Path] {
			continue
		}
		seen[s.c.Path] = true
		out = append(out, s.c)
		if len(out) >= 24 {
			break
		}
	}
	return out
}

func kindBaseScore(kind string) int {
	switch kind {
	case "runtime_atlas":
		return 90
	case "contract":
		return 85
	case "entrypoint":
		return 55
	case "manifest":
		return 50
	case "doc":
		return 35
	case "test":
		return 30
	case "config":
		return 40
	default:
		return 10
	}
}

func containsAny(tokens []string, words ...string) bool {
	set := map[string]bool{}
	for _, t := range tokens {
		set[t] = true
	}
	for _, w := range words {
		if set[w] {
			return true
		}
	}
	return false
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
