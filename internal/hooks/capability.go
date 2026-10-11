package hooks

import (
	"fmt"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
)

// EventSupport is the negotiated level for one canonical event on an adapter.
type EventSupport string

const (
	// SupportUnsupported means the adapter cannot translate this event today.
	SupportUnsupported EventSupport = "unsupported"
	// SupportSupported means the adapter can emit the event into Atlas dispatch.
	SupportSupported EventSupport = "supported"
)

// Capability is the negotiated hooks surface for one adapter.
// Coarse gating uses adapters.ClassHooks; fine-grained events come from declared.
type Capability struct {
	Adapter         adapters.ID
	HooksCapability bool
	Events          map[EventType]EventSupport
}

// Supports reports whether the adapter can emit t into Atlas dispatch.
func (c Capability) Supports(t EventType) bool {
	if !c.HooksCapability {
		return false
	}
	return c.Events[t] == SupportSupported
}

// SupportedEvents returns supported event types in catalog order.
func (c Capability) SupportedEvents() []EventType {
	if !c.HooksCapability {
		return nil
	}
	out := make([]EventType, 0, len(c.Events))
	for _, t := range KnownEventTypes() {
		if c.Events[t] == SupportSupported {
			out = append(out, t)
		}
	}
	return out
}

// EventBridge is the adapter-owned declaration of which canonical events can be
// translated from the native runtime. Implementations live under
// internal/adapters/<provider>/ in later slices — not in this foundation package.
type EventBridge interface {
	AdapterID() adapters.ID
	// DeclaredEvents lists canonical events this adapter can translate.
	// Unknown types are ignored during negotiation; empty means none.
	DeclaredEvents() []EventType
}

// Negotiate builds capability for an adapter from the Core capability table
// and the adapter-declared event set. Unknown adapters and Hooks:false adapters
// yield HooksCapability=false with every catalog event unsupported.
func Negotiate(id adapters.ID, declared []EventType) Capability {
	return buildCapability(id, adapters.Supports(id, adapters.ClassHooks), declared)
}

// NegotiateBridge is Negotiate using an EventBridge declaration.
func NegotiateBridge(bridge EventBridge) Capability {
	if bridge == nil {
		return buildCapability("", false, nil)
	}
	return Negotiate(bridge.AdapterID(), bridge.DeclaredEvents())
}

func buildCapability(id adapters.ID, hooksCapable bool, declared []EventType) Capability {
	cap := Capability{
		Adapter:         id,
		HooksCapability: hooksCapable,
		Events:          make(map[EventType]EventSupport, len(KnownEventTypes())),
	}
	for _, t := range KnownEventTypes() {
		cap.Events[t] = SupportUnsupported
	}
	if !hooksCapable {
		return cap
	}
	for _, t := range declared {
		if !IsKnownEventType(t) {
			continue
		}
		cap.Events[t] = SupportSupported
	}
	return cap
}

// RequireSupported validates that the adapter capability allows ev.
func RequireSupported(cap Capability, ev Event) error {
	if err := ValidateEvent(ev); err != nil {
		return err
	}
	if ev.Adapter != "" && cap.Adapter != "" && ev.Adapter != cap.Adapter {
		return fmt.Errorf("%w: event adapter %q != capability adapter %q", ErrInvalidEvent, ev.Adapter, cap.Adapter)
	}
	if !cap.HooksCapability {
		return fmt.Errorf("%w: %s", ErrHooksDisabled, cap.Adapter)
	}
	if !cap.Supports(ev.Type) {
		return fmt.Errorf("%w: %s/%s", ErrUnsupportedEvent, cap.Adapter, ev.Type)
	}
	return nil
}
