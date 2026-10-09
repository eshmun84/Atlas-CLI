package codeintel

// ProviderID identifies a Code Intelligence backend.
type ProviderID string

const (
	// ProviderCodeGraph is the first external Code Intelligence provider.
	ProviderCodeGraph ProviderID = "codegraph"
)

// State is the Atlas-owned provider / lifecycle state.
type State string

const (
	StateMissing      State = "missing"
	StateAvailable    State = "available"
	StateReady        State = "ready"
	StateStale        State = "stale"
	StateIncompatible State = "incompatible"
	StateUnavailable  State = "unavailable"
	StateError        State = "error"
)

// Freshness is Atlas-owned graph freshness relative to source fingerprint.
type Freshness string

const (
	FreshnessMissing Freshness = "missing"
	FreshnessReady   Freshness = "ready"
	FreshnessStale   Freshness = "stale"
	FreshnessError   Freshness = "error"
)

// RefreshMode is how a mutating refresh executed (or would execute).
type RefreshMode string

const (
	RefreshModeNoop        RefreshMode = "noop"
	RefreshModeInitial     RefreshMode = "initial"
	RefreshModeIncremental RefreshMode = "incremental"
	RefreshModeFull        RefreshMode = "full"
)

// MetadataSchemaVersion is the Atlas-owned metadata.json schema.
const MetadataSchemaVersion = 1

// Project identifies a workspace for provider Status / Refresh.
// Paths are inputs only; Probe/Status never create them.
type Project struct {
	Root     string
	ID       string
	HomePath string
}

// Capability is the Atlas-owned probe result for one provider.
type Capability struct {
	Provider     ProviderID
	State        State
	Version      string
	Executable   string
	Capabilities []string
	Message      string
}

// ProjectStatus is a read-only provider status for one project.
type ProjectStatus struct {
	Capability

	ProjectID       string
	StorageDir      string
	GraphDBPath     string
	MetadataPath    string
	GraphPresent    bool
	MetadataPresent bool
}

// Metadata is Atlas-owned provider metadata written only by explicit refresh.
type Metadata struct {
	SchemaVersion       int    `json:"schemaVersion"`
	Provider            string `json:"provider"`
	ProviderVersion     string `json:"providerVersion,omitempty"`
	ProjectID           string `json:"projectID"`
	ProjectRootIdentity string `json:"projectRootIdentity,omitempty"`
	GraphDBPath         string `json:"graphDBPath,omitempty"`
	RefreshedAt         string `json:"refreshedAt,omitempty"`
	RefreshMode         string `json:"refreshMode,omitempty"`
	SourceFingerprint   string `json:"sourceFingerprint,omitempty"`
	NodesTotal          int    `json:"nodesTotal,omitempty"`
	FilesTotal          int    `json:"filesTotal,omitempty"`
}

// Snapshot is a discovery-friendly Code Intelligence summary for Status/Doctor.
// Probe/Status/Inspect never mutate. Freshness is derived from metadata + source.
type Snapshot struct {
	Applicable      bool
	Provider        ProviderID
	State           State
	Version         string
	Executable      string
	Message         string
	StorageDir      string
	GraphPresent    bool
	MetadataPresent bool
	MetadataValid   bool
	Freshness       Freshness
	FreshnessReason string
	RefreshedAt     string
	RefreshMode     string
	GraphDBPath     string
	MetadataPath    string
}
