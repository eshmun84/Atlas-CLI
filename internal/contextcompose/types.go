package contextcompose

// Schema and status vocabulary for the composition response envelope.
const (
	SchemaVersion = 1

	StatusSuccess        = "success"
	StatusEmpty          = "empty"
	StatusPartial        = "partial"
	StatusInvalidRequest = "invalid_request"

	// Well-known source identifiers. Unknown sources are allowed but sort after these.
	SourceMemory         = "memory"
	SourceContextEconomy = "context_economy"
	SourceCodeGraph      = "codegraph"

	// SourceStatus values the composer may emit. Callers may supply richer
	// statuses; the composer never upgrades them to imply retrieval.
	SourceStatusSupplied     = "supplied"      // candidates were present in the request
	SourceStatusNoCandidates = "no_candidates" // requested but no candidates
	SourceStatusUnavailable  = "unavailable"   // caller-reported
	SourceStatusFailed       = "failed"        // caller-reported
	SourceStatusOmitted      = "omitted"       // all candidates dropped (budget/unsafe)

	DefaultBudgetChars = 8000
	MaxBudgetChars     = 32000
	// MemoryBudgetPercent is the default memory source share of the global budget.
	MemoryBudgetPercent = 50
	// CodeGraphBudgetPercent is the default CodeGraph source share of the global budget.
	// Kept below Memory so CodeGraph cannot starve Memory or Context Economy.
	CodeGraphBudgetPercent = 25
	MaxCandidates          = 32
	MaxCEPaths             = 24
	// MaxCodeGraphResults is the default retrieve-time cap for CodeGraph candidates.
	MaxCodeGraphResults = 8
	MaxExcerptChars     = 280
	DefaultMaxItems     = 32

	// Collection bounds reuse MaxCandidates so request-controlled slices cannot
	// inflate processing cost or response size without limit.
	MaxSources         = MaxCandidates                  // Request.Sources
	MaxSourceStatuses  = MaxCandidates                  // Request.SourceStatuses
	MaxSourceCaps      = MaxCandidates                  // Request.SourceCaps entries
	MaxOmissions       = MaxCandidates                  // Response.Omissions
	MaxResponseSources = MaxSources + MaxSourceStatuses // inferred Sources∪Statuses
	MaxLimitsEntries   = 8                              // package-owned Limits notes

	MaxTaskRunes          = 2048
	MaxQueryRunes         = 2048
	MaxIdentityRunes      = 256
	MaxKindRunes          = 64
	MaxSourceRunes        = 64
	MaxTitleRunes         = 512
	MaxPathRunes          = 512
	MaxRefRunes           = 512
	MaxProvenanceRunes    = 256
	MaxSourceMessageRunes = 256
)

// Request is a bounded, in-memory composition request.
// Callers supply already-retrieved candidates; Compose does not fetch them.
type Request struct {
	Task           string
	Query          string
	BudgetChars    int // 0 → DefaultBudgetChars; > MaxBudgetChars → invalid
	Sources        []string
	SourceCaps     map[string]int // optional per-source rune caps (must be >= 0)
	MaxItems       int            // 0 → DefaultMaxItems; capped at MaxCandidates
	Candidates     []Candidate
	SourceStatuses []SourceStatus // optional caller-reported availability
}

// Candidate is one untrusted composition input.
type Candidate struct {
	Source     string
	Kind       string
	ID         string // stable identity within Source (required)
	Title      string
	Excerpt    string
	Path       string // optional project-relative path
	Ref        string // optional opaque reference string
	Score      int    // preserved from the source; not re-scored
	Provenance string
}

// Item is one budgeted entry in the composed pack.
type Item struct {
	Source     string `json:"source"`
	Kind       string `json:"kind,omitempty"`
	ID         string `json:"id"`
	Title      string `json:"title,omitempty"`
	Excerpt    string `json:"excerpt,omitempty"`
	Path       string `json:"path,omitempty"`
	Ref        string `json:"ref,omitempty"`
	Score      int    `json:"score"`
	Provenance string `json:"provenance,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

// SourceStatus reports per-source outcome without claiming retrieval unless the caller did.
type SourceStatus struct {
	Source  string `json:"source"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Omission records a dropped candidate or aggregate drop reason.
type Omission struct {
	Source   string `json:"source,omitempty"`
	Identity string `json:"identity,omitempty"`
	Reason   string `json:"reason"`
	Count    int    `json:"count,omitempty"`
}

// ErrorBody is a non-sensitive structured error.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Response is the versioned composition envelope.
type Response struct {
	SchemaVersion int            `json:"schema_version"`
	Status        string         `json:"status"`
	Task          string         `json:"task,omitempty"`
	Query         string         `json:"query,omitempty"`
	BudgetChars   int            `json:"budget_chars"`
	UsedChars     int            `json:"used_chars"`
	Count         int            `json:"count"`
	Items         []Item         `json:"items,omitempty"`
	Sources       []SourceStatus `json:"sources,omitempty"`
	Omissions     []Omission     `json:"omissions,omitempty"`
	Limits        []string       `json:"limits,omitempty"`
	Error         *ErrorBody     `json:"error,omitempty"`
}
