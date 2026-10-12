package opencode

import (
	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// SupportedNativeEvents are OpenCode V2 native hook names Atlas translates.
var SupportedNativeEvents = []string{
	NativeSessionCreated,
	NativeSessionPrompt,
	NativeToolExecuteBefore,
	NativeToolExecuteAfter,
	NativeSessionCompaction,
}

// DeclaredCanonicalEvents is the closed OpenCode → Atlas mapping for negotiation.
var DeclaredCanonicalEvents = []hooks.EventType{
	hooks.EventSessionStart,
	hooks.EventTaskStart,
	hooks.EventBeforeToolExecution,
	hooks.EventAfterToolExecution,
	hooks.EventBeforeContextCompaction,
}

// HooksBridge declares which canonical events the OpenCode adapter can emit.
type HooksBridge struct{}

// NewHooksBridge returns the production OpenCode EventBridge.
func NewHooksBridge() HooksBridge { return HooksBridge{} }

func (HooksBridge) AdapterID() adapters.ID { return adapters.OpenCode }

func (HooksBridge) DeclaredEvents() []hooks.EventType {
	out := make([]hooks.EventType, len(DeclaredCanonicalEvents))
	copy(out, DeclaredCanonicalEvents)
	return out
}

var _ hooks.EventBridge = HooksBridge{}
