// Package policy is the pure V1 quality gate for automatic Memory persistence.
//
// Evaluate is deterministic and has no I/O, Cobra, TUI, or SQLite coupling.
package policy

// Action is the closed V1 policy decision.
type Action string

const (
	ActionAccept              Action = "accept"
	ActionReject              Action = "reject"
	ActionRequireConfirmation Action = "require_confirmation"
)

// Explicitness classifies how strongly the candidate was asserted.
type Explicitness string

const (
	ExplicitnessExplicit Explicitness = "explicit"
	// ExplicitnessConfirmed is reserved for a future trusted confirmation
	// surface. Through the untrusted propose transport it must not accept.
	ExplicitnessConfirmed   Explicitness = "confirmed"
	ExplicitnessInferred    Explicitness = "inferred"
	ExplicitnessSpeculative Explicitness = "speculative"
)

// Origin is the closed V1 set of allowed automatic candidate sources.
type Origin string

const (
	OriginAtlasLifecycle        Origin = "atlas_lifecycle"
	OriginDeveloperConfirmation Origin = "developer_confirmation"
	OriginAtlasOperation        Origin = "atlas_operation"
)

// Confidence is a coarse evidence strength hint (not an LLM score).
type Confidence string

const (
	ConfidenceHigh   Confidence = "high"
	ConfidenceMedium Confidence = "medium"
	ConfidenceLow    Confidence = "low"
)

// Gate identifiers for structured reasons.
const (
	GateDurability  = "durability"
	GateSpecificity = "specificity"
	GateEvidence    = "evidence"
	GateStability   = "stability"
	GateSensitivity = "sensitivity"
	GateDuplication = "duplication"
	GateScope       = "scope"
	GateSize        = "size"
	GateOrigin      = "origin"
	GateType        = "type"
	GateInput       = "input"
)

// Candidate is the minimal bounded input for policy evaluation.
type Candidate struct {
	Type         string
	Title        string
	Content      string
	Scope        string
	TopicKey     string
	Provenance   string // opaque bounded provenance string (not a filesystem path)
	Confidence   Confidence
	Explicitness Explicitness
	Origin       Origin
	SourceEvent  string
	// Confirm is not accepted from untrusted propose payloads in V1.
	// Inferred candidates always require_confirmation; promotion via a
	// caller-supplied boolean is forbidden. A trusted confirmation surface
	// is deferred.
}

// Reason is a structured gate outcome.
type Reason struct {
	Gate    string `json:"gate"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Decision is the pure Evaluate result.
type Decision struct {
	Action  Action   `json:"action"`
	Reasons []Reason `json:"reasons"`
}
