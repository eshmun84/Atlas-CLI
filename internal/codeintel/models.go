package codeintel

import "strings"

// ProviderID identifies a Code Intelligence backend.
type ProviderID string

const (
	// ProviderCodeGraph is the first external Code Intelligence provider.
	ProviderCodeGraph ProviderID = "codegraph"
)

// State is the provider lifecycle state owned by Atlas.
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

// Project identifies a workspace for provider Status checks.
// Paths are inputs only; Probe/Status never create them.
type Project struct {
	Root     string
	ID       string
	HomePath string
}

// Capability is the Atlas-owned probe result for one provider.
// Capabilities lists only features actually verified during Probe, never a
// static wishlist of expected provider commands/flags.
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

// Metadata is Atlas-owned provider metadata for a future explicit build/update
// lifecycle. Slice 31 defines the shape only; Probe/Status never write it.
type Metadata struct {
	Provider        string `json:"provider"`
	ProviderVersion string `json:"provider_version,omitempty"`
	ProjectIdentity string `json:"project_identity,omitempty"`
	SourceRevision  string `json:"source_revision,omitempty"`
	BuiltAt         string `json:"built_at,omitempty"`
	Freshness       string `json:"freshness,omitempty"`
}

// Snapshot is a discovery-friendly Code Intelligence summary for Status/Doctor.
// State reflects provider availability/compatibility only. GraphPresent is
// separate Atlas-owned filesystem evidence (no freshness inference in Slice 31).
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
}

// DisplayState returns a stable lowercase state string for UI/diagnostics.
func (c Capability) DisplayState() string {
	s := strings.TrimSpace(string(c.State))
	if s == "" {
		return string(StateUnavailable)
	}
	return s
}
