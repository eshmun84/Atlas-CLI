package sdd

import (
	"fmt"
	"strings"
)

// Service evaluates a project root against one or more Spec Engines.
// Read-only: never mutates the filesystem.
type Service struct {
	engines []Engine
}

// NewService constructs a Service with the given engines.
// Registration order is irrelevant for selection: exactly one present
// engine is required; zero → none; more than one → conflict (fail-closed).
func NewService(engines ...Engine) *Service {
	out := make([]Engine, 0, len(engines))
	for _, e := range engines {
		if e != nil {
			out = append(out, e)
		}
	}
	return &Service{engines: out}
}

type detectedEngine struct {
	eng  Engine
	pres Presence
}

// Evaluate builds a neutral Overview for Status/Doctor.
//
// Selection is fail-closed:
//   - 0 present engines → SDD none
//   - 1 present engine → use it
//   - >1 present engines without explicit selection → EngineConflict;
//     no engine is chosen and no changes are listed from any engine
//
// Detection and listing Issues are always preserved on the Overview, even when
// no engine is selected. Unsafe Detect/List errors are not degraded to
// Incomplete.
func (s *Service) Evaluate(req InspectRequest) Overview {
	req.Root = strings.TrimSpace(req.Root)
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	ov := Overview{}
	if req.Root == "" || s == nil || len(s.engines) == 0 {
		return ov
	}

	var present []detectedEngine
	for _, eng := range s.engines {
		pres, err := eng.Detect(req)
		if err != nil {
			ov.Issues = append(ov.Issues, issueFromEngineError(eng.ID(), "detect", err))
			continue
		}
		for _, msg := range pres.Issues {
			ov.Issues = append(ov.Issues, Issue{
				Kind:    classifyIssueMessage(msg),
				Message: msg,
			})
		}
		if !pres.Present {
			continue
		}
		present = append(present, detectedEngine{eng: eng, pres: pres})
	}

	switch len(present) {
	case 0:
		return ov
	case 1:
		return s.evaluateOne(req, ov, present[0])
	default:
		return conflictOverview(ov, present)
	}
}

func (s *Service) evaluateOne(req InspectRequest, ov Overview, d detectedEngine) Overview {
	label := strings.TrimSpace(d.pres.Label)
	if label == "" {
		label = string(d.eng.ID())
	}
	ov.Presence = Presence{
		Engine:  d.eng.ID(),
		Present: true,
		Label:   label,
	}

	active, archived, issues, listErr := d.eng.ListChanges(req)
	// Preserve adapter issues first — even when ListChanges also returns an error.
	ov.Issues = append(ov.Issues, issues...)
	if listErr != nil {
		ov.Issues = append(ov.Issues, issueFromEngineError(d.eng.ID(), "list", listErr))
		// Fail-closed: do not trust partial change lists on error.
		return ov
	}
	ov.Active = append(ov.Active, active...)
	ov.Archived = append(ov.Archived, archived...)
	return ov
}

func conflictOverview(ov Overview, present []detectedEngine) Overview {
	ov.EngineConflict = true
	ov.Presence = Presence{} // do not select any engine
	ids := make([]string, 0, len(present))
	for _, d := range present {
		id := string(d.eng.ID())
		if id == "" {
			id = strings.TrimSpace(d.pres.Label)
		}
		if id == "" {
			id = "(unnamed)"
		}
		ids = append(ids, id)
	}
	ov.Issues = append(ov.Issues, Issue{
		Kind:    IssueAmbiguous,
		Message: "multiple spec engines present without explicit selection: " + strings.Join(ids, ", "),
	})
	return ov
}

func issueFromEngineError(id EngineID, op string, err error) Issue {
	return Issue{
		Kind:    classifyIssueMessage(err.Error()),
		Message: fmt.Sprintf("engine %s %s: %v", id, op, err),
	}
}

// classifyIssueMessage maps adapter/fsafety error text to an IssueKind.
// Symlink / traversal / escape conditions stay Unsafe (never Incomplete).
func classifyIssueMessage(msg string) IssueKind {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "symlink"),
		strings.Contains(lower, "traversal"),
		strings.Contains(lower, "escapes"):
		return IssueUnsafe
	default:
		return IssueIncomplete
	}
}

// StatusLines returns concise Status-oriented lines for an Overview.
// Examples:
//
//	SDD: none
//	SDD: OpenSpec / Active change: foo
//	SDD: OpenSpec / Active changes: 2
//	SDD: ambiguous
func StatusLines(ov Overview) []string {
	if ov.EngineConflict {
		return []string{"SDD: ambiguous", "Active change: none"}
	}
	if !ov.HasEngine() {
		return []string{"SDD: none"}
	}
	label := ov.Presence.Label
	if label == "" {
		label = string(ov.Presence.Engine)
	}
	lines := []string{"SDD: " + label}
	switch n := len(ov.Active); {
	case n == 0:
		lines = append(lines, "Active change: none")
	case n == 1:
		id := strings.TrimSpace(ov.Active[0].ChangeID)
		if id == "" {
			id = "(unnamed)"
		}
		lines = append(lines, "Active change: "+id)
	default:
		lines = append(lines, fmt.Sprintf("Active changes: %d", n))
	}
	return lines
}
