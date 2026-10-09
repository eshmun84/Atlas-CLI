package mcp

import (
	"os"
	"strings"
)

// Source identifies where a definition originates.
type Source string

const (
	SourceBuiltin Source = "builtin"
	SourceCustom  Source = "custom"
)

// Transport is the MCP connection kind Atlas manages.
type Transport string

const (
	TransportStdio          Transport = "stdio"
	TransportStreamableHTTP Transport = "streamable_http"
	// TransportSSE is legacy-only: load/normalize, never offer as a new selectable transport.
	TransportSSE Transport = "sse"
)

// AuthRequirement describes how credentials are expected without storing secrets.
type AuthRequirement string

const (
	AuthNone                 AuthRequirement = "none"
	AuthEnvironmentReference AuthRequirement = "environment_reference"
	AuthOAuthExternal        AuthRequirement = "oauth_external"
	AuthProviderManaged      AuthRequirement = "provider_managed"
)

// HeaderValueRef references an environment variable for a header value.
// Secrets are never stored — only the env name and an optional non-secret prefix
// (for example "Bearer ").
type HeaderValueRef struct {
	Env    string `yaml:"env"`
	Prefix string `yaml:"prefix,omitempty"`
}

// IsZero reports whether the ref carries no env name.
func (h HeaderValueRef) IsZero() bool {
	return strings.TrimSpace(h.Env) == ""
}

// RenderCursor interpolates the env reference for Cursor MCP headers.
func (h HeaderValueRef) RenderCursor() string {
	name := EnvRefName(h.Env)
	return h.Prefix + "${env:" + name + "}"
}

// RenderOpenCode interpolates the env reference for OpenCode MCP headers.
func (h HeaderValueRef) RenderOpenCode() string {
	name := EnvRefName(h.Env)
	return h.Prefix + "{env:" + name + "}"
}

// ActionKind is one reconciliation plan action.
type ActionKind string

const (
	ActionCreate    ActionKind = "create"
	ActionUpdate    ActionKind = "update"
	ActionUnchanged ActionKind = "unchanged"
	ActionRemove    ActionKind = "remove"
	ActionBlocked   ActionKind = "blocked"
)

// AdapterID identifies a projection target.
type AdapterID string

const (
	AdapterCursor   AdapterID = "cursor"
	AdapterOpenCode AdapterID = "opencode"
)

// Definition is a normalized, agent-neutral MCP definition.
type Definition struct {
	ID              string
	DisplayName     string
	Source          Source
	Transport       Transport
	Command         string
	Args            []string
	Endpoint        string
	EnvRefs         []string
	HeaderRefs      map[string]HeaderValueRef // header name -> env ref (+ optional prefix); no secret values
	AuthRequirement AuthRequirement
	Enabled         bool
	// Materializable is false for builtins that persist selection but lack a verified projection.
	Materializable bool
	// PrerequisiteCommand is checked by Doctor (e.g. "npx"); Atlas never installs it.
	PrerequisiteCommand string
	// Compatibility notes for Status/Doctor display.
	CompatibilityNote string
}

// DesiredState is the Atlas-owned MCP desired set for a project.
type DesiredState struct {
	Definitions []Definition
}

// Selected returns enabled definitions.
func (s DesiredState) Selected() []Definition {
	out := make([]Definition, 0, len(s.Definitions))
	for _, d := range s.Definitions {
		if d.Enabled {
			out = append(out, d)
		}
	}
	return out
}

// SelectedMaterializable returns enabled definitions that can be projected.
func (s DesiredState) SelectedMaterializable() []Definition {
	out := make([]Definition, 0, len(s.Definitions))
	for _, d := range s.Definitions {
		if d.Enabled && d.Materializable {
			out = append(out, d)
		}
	}
	return out
}

// NativeEntry is a projector-built payload for one MCP server key.
type NativeEntry struct {
	Key     string
	Payload map[string]any
}

// OpenCodeShape identifies how MCP servers are nested in opencode.json.
type OpenCodeShape string

const (
	OpenCodeShapeEmpty     OpenCodeShape = "empty"
	OpenCodeShapeFlat      OpenCodeShape = "flat"      // mcp.<name>
	OpenCodeShapeNested    OpenCodeShape = "nested"    // mcp.servers.<name>
	OpenCodeShapeAmbiguous OpenCodeShape = "ambiguous" // both representations present
)

// NativeSnapshot is a read-only view of an adapter's MCP config file.
type NativeSnapshot struct {
	Adapter      AdapterID
	Path         string
	Exists       bool
	Malformed    bool
	MalformError string
	Servers      map[string]map[string]any
	Raw          []byte
	// Mode is the preexisting file permission bits when Exists is true.
	Mode os.FileMode
	// OpenCodeShape is set by the OpenCode projector; empty for others.
	OpenCodeShape OpenCodeShape
}

// OwnedEntry records one Atlas-managed native key.
type OwnedEntry struct {
	DefinitionID string `yaml:"definition_id"`
	NativeKey    string `yaml:"native_key"`
}

// AdapterOwnership is ownership for one adapter projection.
type AdapterOwnership struct {
	Adapter AdapterID    `yaml:"adapter"`
	Entries []OwnedEntry `yaml:"entries"`
}

// OwnershipDocument is machine-local MCP ownership under Atlas Home.
type OwnershipDocument struct {
	SchemaVersion int                `yaml:"schema_version"`
	ProjectID     string             `yaml:"project_id"`
	Adapters      []AdapterOwnership `yaml:"adapters"`
}

const OwnershipSchemaVersion = 1

// PlanAction is one explicit reconciliation step.
type PlanAction struct {
	Adapter      AdapterID
	DefinitionID string
	NativeKey    string
	Kind         ActionKind
	Reason       string
}

// Plan is a testeable reconciliation plan.
type Plan struct {
	Actions []PlanAction
	Blocked bool
}

// HasKind reports whether any action matches kind.
func (p Plan) HasKind(kind ActionKind) bool {
	for _, a := range p.Actions {
		if a.Kind == kind {
			return true
		}
	}
	return false
}

// NeedsWrite reports whether apply would mutate native config.
func (p Plan) NeedsWrite() bool {
	for _, a := range p.Actions {
		switch a.Kind {
		case ActionCreate, ActionUpdate, ActionRemove:
			return true
		}
	}
	return false
}

// ProjectionStatus summarizes one adapter's MCP health for Status/Doctor.
type ProjectionStatus string

const (
	ProjectionMissing      ProjectionStatus = "missing"
	ProjectionMaterialized ProjectionStatus = "materialized"
	ProjectionDrifted      ProjectionStatus = "drifted"
	ProjectionMalformed    ProjectionStatus = "malformed"
	ProjectionBlocked      ProjectionStatus = "blocked"
	ProjectionUnsupported  ProjectionStatus = "unsupported"
	ProjectionEmpty        ProjectionStatus = "empty"
)

// AdapterHealth is read-only MCP health for one adapter.
type AdapterHealth struct {
	Adapter           AdapterID
	SupportsMCP       bool
	Selected          int
	Materialized      int
	Status            ProjectionStatus
	NativePath        string
	NativeExists      bool
	Malformed         bool
	MalformError      string
	DriftedKeys       []string
	MissingKeys       []string
	OwnershipConflict []string
	Warnings          []string
	Issues            []string
}

// Health is the aggregate MCP health snapshot (read-only).
type Health struct {
	SelectedCount int
	Adapters      []AdapterHealth
	DefinitionErr []string
	Ready         bool
}
