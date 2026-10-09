package mcp

import (
	"fmt"
	"strings"
)

// CustomSpec is a transport-neutral custom MCP input used to build DesiredState
// without importing config types (avoids dependency cycles).
type CustomSpec struct {
	ID           string
	Name         string
	Transport    string
	CommandOrURL string
	Args         []string
	EnvRefs      []string
	HeaderRefs   map[string]HeaderValueRef
	Auth         AuthRequirement
	Enabled      bool
}

// Selection is the enablement map for builtins plus custom specs.
type Selection struct {
	BuiltinEnabled map[string]bool
	Custom         []CustomSpec
}

// BuildDesiredState expands catalog + custom specs into a validated DesiredState.
func BuildDesiredState(sel Selection) (DesiredState, error) {
	state := DesiredState{}
	for _, builtin := range BuiltinCatalog() {
		def := builtin
		def.Enabled = sel.BuiltinEnabled[builtin.ID]
		state.Definitions = append(state.Definitions, def)
	}
	for i, custom := range sel.Custom {
		id := strings.TrimSpace(custom.ID)
		if id == "" {
			id = fmt.Sprintf("custom-%d", i+1)
		}
		transport := NormalizeTransport(custom.Transport)
		auth := custom.Auth
		if auth == "" {
			auth = AuthNone
			if transport == TransportStreamableHTTP || transport == TransportSSE {
				if len(custom.EnvRefs) > 0 || len(custom.HeaderRefs) > 0 {
					auth = AuthEnvironmentReference
				}
			}
		}
		def := Definition{
			ID:              id,
			DisplayName:     strings.TrimSpace(custom.Name),
			Source:          SourceCustom,
			Transport:       transport,
			EnvRefs:         append([]string(nil), custom.EnvRefs...),
			HeaderRefs:      copyHeaderRefs(custom.HeaderRefs),
			AuthRequirement: auth,
			Enabled:         custom.Enabled,
			Materializable:  true,
		}
		switch transport {
		case TransportStdio:
			def.Command = strings.TrimSpace(custom.CommandOrURL)
			def.Args = append([]string(nil), custom.Args...)
			def.PrerequisiteCommand = firstPathToken(def.Command)
		case TransportStreamableHTTP, TransportSSE:
			def.Endpoint = strings.TrimSpace(custom.CommandOrURL)
		}
		state.Definitions = append(state.Definitions, def)
	}
	if err := ValidateDesiredState(state); err != nil {
		return DesiredState{}, err
	}
	return state, nil
}

func copyHeaderRefs(in map[string]HeaderValueRef) map[string]HeaderValueRef {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]HeaderValueRef, len(in))
	for k, v := range in {
		out[k] = HeaderValueRef{Env: v.Env, Prefix: v.Prefix}
	}
	return out
}

func firstPathToken(command string) string {
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}
	// Bare command name only (no path traversal checks here — validate covers shell concat).
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ""
	}
	base := parts[0]
	if strings.Contains(base, "/") || strings.Contains(base, `\`) {
		return ""
	}
	return base
}
