package cursor

import (
	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
)

// SupportedNativeEvents are Cursor native hook names Atlas translates in this slice.
var SupportedNativeEvents = []string{
	NativeSessionStart,
	NativeBeforeSubmitPrompt,
	NativePreToolUse,
	NativePostToolUse,
	NativePreCompact,
}

// DeclaredCanonicalEvents is the closed Cursor → Atlas mapping for negotiation.
var DeclaredCanonicalEvents = []hooks.EventType{
	hooks.EventSessionStart,
	hooks.EventTaskStart,
	hooks.EventBeforeToolExecution,
	hooks.EventAfterToolExecution,
	hooks.EventBeforeContextCompaction,
}

// HooksBridge declares which canonical events the Cursor adapter can emit.
type HooksBridge struct{}

// NewHooksBridge returns the production Cursor EventBridge.
func NewHooksBridge() HooksBridge { return HooksBridge{} }

func (HooksBridge) AdapterID() adapters.ID { return adapters.Cursor }

func (HooksBridge) DeclaredEvents() []hooks.EventType {
	out := make([]hooks.EventType, len(DeclaredCanonicalEvents))
	copy(out, DeclaredCanonicalEvents)
	return out
}

var _ hooks.EventBridge = HooksBridge{}
