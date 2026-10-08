package codeintel

import (
	"fmt"
	"strings"
)

// RefreshPlan is a read-only preview for the TUI Review → Apply flow.
type RefreshPlan struct {
	Blocked      bool
	Blockers     []string
	Warnings     []string
	Noop         bool
	Mode         RefreshMode
	ForceFull    bool
	Provider     ProviderID
	Version      string
	GraphPresent bool
	Freshness    Freshness
	Reason       string
	GraphDBPath  string
	MetadataPath string
	Fingerprint  string
	ProjectID    string
	HomePath     string
}

// NeedsApply reports whether Apply would mutate.
func (p RefreshPlan) NeedsApply() bool {
	return !p.Blocked && !p.Noop
}

// BuildRefreshPlan previews lifecycle refresh without mutating.
func BuildRefreshPlan(snap Snapshot, project Project, forceFull bool, fingerprint string) RefreshPlan {
	plan := RefreshPlan{
		Blockers:     []string{},
		Warnings:     []string{},
		ForceFull:    forceFull,
		Provider:     snap.Provider,
		Version:      snap.Version,
		GraphPresent: snap.GraphPresent,
		Freshness:    snap.Freshness,
		Reason:       snap.FreshnessReason,
		GraphDBPath:  snap.GraphDBPath,
		MetadataPath: snap.MetadataPath,
		Fingerprint:  fingerprint,
		ProjectID:    project.ID,
		HomePath:     project.HomePath,
	}
	switch snap.State {
	case StateUnavailable, StateMissing:
		plan.Blocked = true
		plan.Blockers = append(plan.Blockers, "Code Intelligence provider unavailable")
		return plan
	case StateIncompatible:
		plan.Blocked = true
		plan.Blockers = append(plan.Blockers, "Code Intelligence provider incompatible")
		return plan
	case StateError:
		plan.Blocked = true
		msg := snap.Message
		if msg == "" {
			msg = "Code Intelligence provider error"
		}
		plan.Blockers = append(plan.Blockers, msg)
		return plan
	}
	if strings.TrimSpace(project.ID) == "" || strings.TrimSpace(project.HomePath) == "" {
		plan.Blocked = true
		plan.Blockers = append(plan.Blockers, "Atlas Home project identity unresolved")
		return plan
	}
	if !forceFull && snap.GraphPresent && snap.Freshness == FreshnessReady {
		plan.Noop = true
		plan.Mode = RefreshModeNoop
		plan.Reason = "graph already fresh"
		return plan
	}
	plan.Mode = DecideRefreshMode(snap.GraphPresent, forceFull)
	plan.Reason = fmt.Sprintf("planned %s refresh", plan.Mode)
	return plan
}
