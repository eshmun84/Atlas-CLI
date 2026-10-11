// Package hookproj materializes Atlas-owned native hook projections.
// Adapter-neutral ownership/reconcile; Cursor projector lives under adapters/cursor.
package hookproj

import "github.com/eshmun84/Atlas-CLI/internal/adapters"

const (
	OwnershipSchemaVersion = 1

	KindWrapper = "wrapper"
	KindConfig  = "config"

	SourceBundled = "bundled"
)

// Projector is the adapter-owned hooks projection contract.
type Projector interface {
	ID() adapters.ID
	SupportsHooks() bool
	HooksJSONRel() string
	WrapperRel() string
	WrapperBytes() []byte
	ManagedCommand() string
	ManagedEvents() []string
}

// OwnershipDocument is machine-local Atlas ownership for hook projections.
type OwnershipDocument struct {
	SchemaVersion int                `yaml:"schema_version"`
	ProjectID     string             `yaml:"project_id"`
	UpdatedAt     string             `yaml:"updated_at,omitempty"`
	Adapters      []AdapterOwnership `yaml:"adapters"`
}

// AdapterOwnership lists Atlas-owned hook artifacts for one adapter.
type AdapterOwnership struct {
	Adapter string       `yaml:"adapter"`
	Entries []OwnedEntry `yaml:"entries"`
}

// OwnedEntry is evidence-based ownership for one projected artifact.
// Path/content equality never establishes ownership — only this record does.
type OwnedEntry struct {
	Kind      string   `yaml:"kind"`
	PathRel   string   `yaml:"path_rel"`
	Digest    string   `yaml:"digest"`
	Events    []string `yaml:"events,omitempty"`
	Source    string   `yaml:"source,omitempty"`
	UpdatedAt string   `yaml:"updated_at,omitempty"`
}

// ReconcileInput drives hooks projection reconciliation.
type ReconcileInput struct {
	Root           string
	HomePath       string
	ProjectID      string
	Adapters       []string
	Projectors     map[adapters.ID]Projector
	Ownership      OwnershipDocument
	DryRun         bool
	ReclaimUnowned bool
}

// ReconcileResult is the outcome of planning and optional apply.
type ReconcileResult struct {
	Ownership OwnershipDocument
	Applied   bool
	Blocked   bool
	Errors    []string
	Actions   []string
	Mutation  MutationState
}
