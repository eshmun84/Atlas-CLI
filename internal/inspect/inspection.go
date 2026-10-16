package inspect

import (
	"context"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/sdd"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

// Inspection is the canonical read-only application snapshot for a workspace root.
//
// It is the single composed evidence consumed by Init, Status, and Doctor:
//
//	project.Inspect → runtime.EvaluateHealth → sdd.Evaluate → Inspection
//
// Discover/Inspect once; consumers must not re-probe Git, tools, CodeGraph,
// Spec Engines, or runtime files. Runtime Repair may use runtime.Health
// directly when that is the natural domain boundary.
//
// Note: this type lives in inspect (not app) because app currently imports cli,
// which imports tui; placing the snapshot in app would create an import cycle
// once TUI consumes it. app remains the thin CLI entrypoint for now.
type Inspection struct {
	project.Snapshot
	Runtime runtime.Health
	SDD     sdd.Overview
	// OpenSpecProvider is the bounded provider probe for governed SDD.
	// Populated once per Inspect when DependsOnSDDContract; Status/Doctor
	// must consume this snapshot instead of probing again.
	OpenSpecProvider       openspec.Health
	OpenSpecProviderProbed bool
}

// ProjectSnapshot returns the project-only half of the inspection.
func (i Inspection) ProjectSnapshot() project.Snapshot { return i.Snapshot }

// Inspect builds the canonical snapshot for root.
// Read-only: never mutates the project, Git, or Atlas Home.
// Project inspection runs once; runtime health and SDD evaluate that same root.
func Inspect(root string) (Inspection, error) {
	snap, err := project.Inspect(root)
	if err != nil {
		return Inspection{}, err
	}
	health := runtime.EvaluateHealth(snap.RootPath, snap.Atlas, snap.Files)
	snap.Warnings = append(append([]string{}, snap.Warnings...), health.Warnings...)
	overview := evaluateSDD(snap.RootPath, health)
	out := Inspection{
		Snapshot: snap,
		Runtime:  health,
		SDD:      overview,
	}
	if health.ConfigLoads && health.DependsOnSDDContract {
		pin := strings.TrimSpace(health.Document.Governance.OpenSpecVersion)
		out.OpenSpecProvider = openspec.Probe(context.Background(), openspec.ProbeInput{
			Root:       snap.RootPath,
			ProjectPin: pin,
		})
		out.OpenSpecProviderProbed = true
	}
	return out, nil
}

// evaluateSDD is the composition root that wires Spec Engine adapters.
// internal/sdd stays free of provider imports; adapters are selected here.
func evaluateSDD(root string, health runtime.Health) sdd.Overview {
	projectID := ""
	if health.ConfigLoads {
		name := strings.TrimSpace(health.Document.Project.Name)
		if id, err := home.ProjectID(root, name); err == nil {
			projectID = id
		}
	}
	svc := sdd.NewService(openspec.New())
	return svc.Evaluate(sdd.InspectRequest{Root: root, ProjectID: projectID})
}
