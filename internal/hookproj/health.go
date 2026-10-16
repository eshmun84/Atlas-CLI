package hookproj

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// Projection states for read-only hooks health (Doctor/Repair authority).
const (
	StateReady       = "ready"
	StateMissing     = "missing"
	StateStale       = "stale"
	StateDrifted     = "drifted"
	StateConflict    = "conflict"
	StateMalformed   = "malformed"
	StateUnsupported = "unsupported"
	StateDisabled    = "disabled"
)

// ProjectionStatus is one adapter's Atlas-owned hooks projection health.
type ProjectionStatus struct {
	Adapter string
	PathRel string
	State   string // ready|missing|stale|drifted|conflict|malformed|unsupported|disabled
	Owned   bool
	Message string
	// Repairable is true when Runtime Repair can restore via ReconcileHookProjections.
	// Digest drift / conflict / malformed are never Repair-owned (Reconcile refuses).
	Repairable bool
}

// HookHealth is a read-only hooks projection snapshot.
type HookHealth struct {
	// Configured is true when at least one selected adapter supports ClassHooks.
	Configured  bool
	Projections []ProjectionStatus
	CheckedAt   time.Time
}

// InspectInput drives read-only hooks health evaluation.
type InspectInput struct {
	Root       string
	HomePath   string
	ProjectID  string
	Adapters   []string // selected adapters (hooks-capable filtered by caller or here)
	Projectors map[adapters.ID]Projector
	Ownership  OwnershipDocument
}

// Inspect evaluates Atlas-owned hooks projections without mutation.
// Bounded to known projector paths only — no repo-wide scanning.
func Inspect(in InspectInput) HookHealth {
	h := HookHealth{CheckedAt: time.Now().UTC()}
	for _, adapter := range in.Adapters {
		id := adapters.ID(strings.ToLower(strings.TrimSpace(adapter)))
		if !adapters.Supports(id, adapters.ClassHooks) {
			continue
		}
		h.Configured = true
		proj, ok := in.Projectors[id]
		if !ok || proj == nil || !proj.SupportsHooks() {
			h.Projections = append(h.Projections, ProjectionStatus{
				Adapter: string(id),
				State:   StateUnsupported,
				Message: "adapter does not support Hooks projections",
			})
			continue
		}
		h.Projections = append(h.Projections, inspectOne(in.Root, in.Ownership, string(id), proj))
	}
	if !h.Configured {
		h.Projections = append(h.Projections, ProjectionStatus{
			State:   StateDisabled,
			Message: "no hooks-capable adapter selected",
		})
	}
	return h
}

func inspectOne(root string, own OwnershipDocument, adapter string, proj Projector) ProjectionStatus {
	artifactRel := proj.ArtifactRel()
	st := ProjectionStatus{
		Adapter: adapter,
		PathRel: artifactRel,
	}

	var ownedEntry OwnedEntry
	var hasOwn bool
	switch proj.Style() {
	case StylePluginFile:
		ownedEntry, hasOwn = own.LookupKind(adapter, KindPlugin)
	default:
		ownedEntry, hasOwn = own.LookupKind(adapter, KindWrapper)
	}
	st.Owned = hasOwn && strings.TrimSpace(ownedEntry.PathRel) == artifactRel

	desired := DigestBytes(append([]byte(nil), proj.ArtifactBytes()...))
	if desired == "" || len(proj.ArtifactBytes()) == 0 {
		st.State = StateMalformed
		st.Message = "hooks artifact asset missing from Atlas Home/bundled assets"
		return st
	}

	raw, exists, err := readProjectFile(root, artifactRel)
	if err != nil {
		if isUnsafePathErr(err) {
			st.State = StateConflict
			st.Message = fmt.Sprintf("hooks path unsafe at %s", artifactRel)
			return st
		}
		st.State = StateMalformed
		st.Message = err.Error()
		return st
	}
	if !exists {
		st.State = StateMissing
		st.Message = "Atlas-owned hooks artifact missing"
		st.Repairable = true
		return st
	}

	info, lerr := fsafety.LstatContained(root, artifactRel)
	if lerr != nil {
		if isUnsafePathErr(lerr) {
			st.State = StateConflict
			st.Message = fmt.Sprintf("hooks path unsafe at %s", artifactRel)
			return st
		}
		st.State = StateMalformed
		st.Message = lerr.Error()
		return st
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		st.State = StateConflict
		st.Message = fmt.Sprintf("hooks path is not a regular file at %s", artifactRel)
		return st
	}

	dig := DigestBytes(raw)
	if !hasOwn {
		st.State = StateConflict
		st.Message = "hooks artifact exists but is not Atlas-owned"
		return st
	}
	if dig != ownedEntry.Digest {
		st.State = StateDrifted
		st.Message = "hooks artifact digest differs from ownership; refusing automatic rewrite"
		return st
	}
	if dig != desired {
		st.State = StateStale
		st.Message = "Atlas-owned hooks artifact stale vs bundled contract"
		st.Repairable = true
		return st
	}

	// Config merge document (Cursor hooks.json) — report malformed/conflict without
	// claiming Memory injection. Plugin-style adapters have no merge doc.
	if configRel := strings.TrimSpace(proj.ConfigMergeRel()); configRel != "" {
		if cfgState, cfgMsg := inspectConfigHealth(root, own, adapter, proj); cfgState != StateReady {
			st.State = cfgState
			st.Message = cfgMsg
			st.Repairable = cfgState == StateMissing || cfgState == StateStale
			return st
		}
	}

	st.State = StateReady
	st.Message = "ok"
	return st
}

func inspectConfigHealth(root string, own OwnershipDocument, adapter string, proj Projector) (state, message string) {
	configRel := proj.ConfigMergeRel()
	command := proj.ManagedCommand()
	events := proj.ManagedEvents()
	ownedConfig, hasConfigOwn := own.LookupKind(adapter, KindConfig)

	raw, exists, err := readProjectFile(root, configRel)
	if err != nil {
		if isUnsafePathErr(err) {
			return StateConflict, fmt.Sprintf("hooks config unsafe at %s", configRel)
		}
		return StateMalformed, err.Error()
	}
	if !exists {
		return StateMissing, "hooks config missing: " + configRel
	}
	info, lerr := fsafety.LstatContained(root, configRel)
	if lerr != nil {
		if isUnsafePathErr(lerr) {
			return StateConflict, fmt.Sprintf("hooks config unsafe at %s", configRel)
		}
		return StateMalformed, lerr.Error()
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return StateConflict, fmt.Sprintf("hooks config unsafe at %s", configRel)
	}
	doc, perr := parseHooksJSON(raw)
	if perr != nil {
		return StateMalformed, fmt.Sprintf("hooks config malformed at %s", configRel)
	}
	if hasManagedCommand(doc, command) && !hasConfigOwn {
		return StateConflict, fmt.Sprintf("hooks config conflict at %s (managed command without ownership)", configRel)
	}
	if !hasConfigOwn {
		return StateMissing, "hooks config ownership missing for managed entries"
	}
	currentFP := managedFingerprint(doc, command, ownedConfig.Events)
	if len(ownedConfig.Events) == 0 {
		currentFP = managedFingerprint(doc, command, events)
	}
	if currentFP != ownedConfig.Digest {
		return StateDrifted, fmt.Sprintf("hooks config digest drift at %s; refusing automatic rewrite", configRel)
	}
	desiredFP := managedFingerprint(mergeManaged(doc, command, events), command, events)
	if ownedConfig.Digest != desiredFP {
		return StateStale, "Atlas-owned hooks config stale vs managed contract"
	}
	return StateReady, "ok"
}

// SummaryCounts returns counts by projection state.
func (h HookHealth) SummaryCounts() map[string]int {
	out := map[string]int{}
	for _, p := range h.Projections {
		out[p.State]++
	}
	return out
}

// RepairableCount returns projections Runtime Repair can restore.
func (h HookHealth) RepairableCount() int {
	n := 0
	for _, p := range h.Projections {
		if p.Repairable {
			n++
		}
	}
	return n
}

// ManualCount returns projections that require manual remediation.
func (h HookHealth) ManualCount() int {
	n := 0
	for _, p := range h.Projections {
		switch p.State {
		case StateDrifted, StateConflict, StateMalformed, StateUnsupported:
			n++
		}
	}
	return n
}
