package memoryhealth

// Status is the overall Memory health summary.
type Status string

const (
	StatusHealthy     Status = "healthy"
	StatusDegraded    Status = "degraded"
	StatusUnavailable Status = "unavailable"
	StatusFailed      Status = "failed"
)

// CheckSeverity is the outcome of one Memory diagnostic check.
type CheckSeverity string

const (
	SeverityPass CheckSeverity = "pass"
	SeverityInfo CheckSeverity = "info"
	SeverityWarn CheckSeverity = "warn"
	SeverityFail CheckSeverity = "fail"
)

// Check is one structured Memory diagnostic finding.
type Check struct {
	ID              string            `json:"id"`
	Severity        CheckSeverity     `json:"severity"`
	Status          string            `json:"status"`
	Message         string            `json:"message"`
	RemediationHint string            `json:"remediation_hint,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// Metrics are bounded aggregate counters safe for Status/Doctor display.
type Metrics struct {
	ObservationCount      int `json:"observation_count"`
	ProjectScopeCount     int `json:"project_scope_count"`
	DecisionCount         int `json:"decision_count"`
	ConstraintCount       int `json:"constraint_count"`
	PatternCount          int `json:"pattern_count"`
	DuplicateExactCount   int `json:"duplicate_exact_count"`
	DuplicateAnomalyCount int `json:"duplicate_anomaly_count"`
	MalformedCount        int `json:"malformed_count"`
	OversizedCount        int `json:"oversized_count"`
	InvalidScopeCount     int `json:"invalid_scope_count"`
	InvalidTypeCount      int `json:"invalid_type_count"`
	AnomalyCount          int `json:"anomaly_count"`
}

// Diagnostics is the structured Memory diagnostics result.
type Diagnostics struct {
	Status  Status   `json:"status"`
	Checks  []Check  `json:"checks"`
	Metrics Metrics  `json:"metrics"`
	Limits  []string `json:"limits,omitempty"`

	// Compact executive fields for Status (no absolute Home paths).
	StorageLabel     string `json:"storage_label"`
	SchemaLabel      string `json:"schema_label"`
	RetrievalLabel   string `json:"retrieval_label"`
	PersistenceLabel string `json:"persistence_label"`
	QualityLabel     string `json:"quality_label"`
	Enabled          bool   `json:"enabled"`
	PersistenceMode  string `json:"persistence_mode"`
	ProjectID        string `json:"project_id,omitempty"`
	StorageRel       string `json:"storage_rel,omitempty"` // Home-relative only
}

// Input carries read-only facts for Memory inspection.
type Input struct {
	Root            string
	HomePath        string
	ProjectID       string
	Initialized     bool
	ConfigLoads     bool
	MemoryEnabled   bool
	PersistenceMode string // governed | off | "" (default governed when enabled)
}
