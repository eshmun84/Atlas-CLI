// Package sdd defines the neutral Spec-Driven Development domain.
//
// Atlas governs and observes SDD changes without owning or duplicating the
// authoritative change artifacts. Provider-specific paths, filenames, and
// commands belong exclusively in a Spec Engine adapter.
package sdd

import "time"

// EngineID is an opaque Spec Engine identifier supplied by an adapter.
// Core treats it as a label/token only — never as a filesystem path.
type EngineID string

// LifecycleState is the neutral change lifecycle Atlas can distinguish.
//
// Conceptual progression:
//
//	active → ready → verified → archived
//
// Fail closed: when state cannot be determined confidently, use Unknown
// or Incomplete — never invent success.
//
// LifecycleReady means required artifacts exist and required tasks appear
// complete, so the change looks ready for verification. It does NOT mean
// verification passed, correctness proven, or acceptance criteria verified.
// VerificationState remains independent (typically Unknown) until
// authoritative verification evidence exists.
type LifecycleState string

const (
	LifecycleUnknown    LifecycleState = "unknown"
	LifecycleIncomplete LifecycleState = "incomplete"
	LifecycleActive     LifecycleState = "active"
	LifecycleReady      LifecycleState = "ready"
	LifecycleVerified   LifecycleState = "verified"
	LifecycleArchived   LifecycleState = "archived"
)

// VerificationState records available verification metadata without copying
// verification artifacts into an Atlas evidence store.
// Independent of LifecycleState: LifecycleReady must not imply Pass.
type VerificationState string

const (
	VerificationUnknown    VerificationState = "unknown"
	VerificationNone       VerificationState = "none"
	VerificationIncomplete VerificationState = "incomplete"
	VerificationPass       VerificationState = "pass"
	VerificationFail       VerificationState = "fail"
)

// SpecRef is a normalized pointer to a spec surface owned by the Spec Engine.
// Ref is opaque adapter-supplied data (not an Atlas filesystem path contract).
type SpecRef struct {
	Name string
	Ref  string
}

// ChangeRef is the neutral representation of one Spec Engine change.
// It holds references and enough state for governance/status — never a copy
// of proposal/spec/design/tasks/verify/archive contents.
type ChangeRef struct {
	ProjectID    string
	Engine       EngineID
	ChangeID     string
	Lifecycle    LifecycleState
	Location     string // opaque adapter reference (not Atlas-owned storage)
	Archived     bool
	Verification VerificationState
	UpdatedAt    *time.Time
	ArchivedAt   *time.Time
	SpecRefs     []SpecRef
	Ambiguous    bool
	Issues       []string // short fail-closed notes (malformed, unsafe, incomplete)
}

// Presence describes whether a Spec Engine is detectable at a project root.
type Presence struct {
	Engine  EngineID
	Present bool
	Label   string // human-facing short label (adapter-supplied)
	Issues  []string
}

// Issue classifies a read-only SDD observation for Doctor.
type IssueKind string

const (
	IssueNone       IssueKind = ""
	IssueMalformed  IssueKind = "malformed"
	IssueUnsafe     IssueKind = "unsafe"
	IssueAmbiguous  IssueKind = "ambiguous"
	IssueIncomplete IssueKind = "incomplete"
	IssueInfo       IssueKind = "info"
)

// Issue is a single Doctor-oriented finding from SDD inspection.
type Issue struct {
	Kind    IssueKind
	Message string
	Change  string // optional ChangeID
}

// Overview is the read-only SDD snapshot for Status/Doctor consumption.
type Overview struct {
	Presence Presence
	Active   []ChangeRef
	Archived []ChangeRef
	Issues   []Issue

	// EngineConflict is true when more than one Spec Engine is present and
	// there is no explicit selection. Atlas must not pick a winner by
	// registration order, first match, or filesystem order.
	EngineConflict bool
}

// HasEngine reports whether exactly one Spec Engine is selected for use.
// False when none are present or when EngineConflict is set.
func (o Overview) HasEngine() bool {
	if o.EngineConflict {
		return false
	}
	return o.Presence.Present && o.Presence.Engine != ""
}
