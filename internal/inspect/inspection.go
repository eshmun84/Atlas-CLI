package inspect

import (
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

// Inspection is the canonical read-only application snapshot for a workspace root.
//
// It is the single composed evidence consumed by Init, Status, and Doctor:
//
//	project.Inspect → runtime.EvaluateHealth → Inspection
//
// Discover/Inspect once; consumers must not re-probe Git, tools, CodeGraph, or
// runtime files. Runtime Repair may use runtime.Health directly when that is
// the natural domain boundary.
//
// Note: this type lives in inspect (not app) because app currently imports cli,
// which imports tui; placing the snapshot in app would create an import cycle
// once TUI consumes it. app remains the thin CLI entrypoint for now.
type Inspection struct {
	project.Snapshot
	Runtime runtime.Health
}

// ProjectSnapshot returns the project-only half of the inspection.
func (i Inspection) ProjectSnapshot() project.Snapshot { return i.Snapshot }

// Inspect builds the canonical snapshot for root.
// Read-only: never mutates the project, Git, or Atlas Home.
// Project inspection runs once; runtime health evaluates that same evidence.
func Inspect(root string) (Inspection, error) {
	snap, err := project.Inspect(root)
	if err != nil {
		return Inspection{}, err
	}
	health := runtime.EvaluateHealth(snap.RootPath, snap.Atlas, snap.Files)
	snap.Warnings = append(append([]string{}, snap.Warnings...), health.Warnings...)
	return Inspection{
		Snapshot: snap,
		Runtime:  health,
	}, nil
}
