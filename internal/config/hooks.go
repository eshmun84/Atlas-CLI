package config

import (
	"fmt"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	adaptercursor "github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	adapteropencode "github.com/eshmun84/Atlas-CLI/internal/adapters/opencode"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/hookproj"
)

func init() {
	hookproj.RegisterDefaultProjector(adaptercursor.NewHooksProjector())
	hookproj.RegisterDefaultProjector(adapteropencode.NewHooksProjector())
}

// HookProjectors returns registered hooks projectors.
func HookProjectors() map[adapters.ID]hookproj.Projector {
	return hookproj.DefaultProjectors()
}

// InspectHookHealth returns read-only hooks projection health for Status/Doctor.
// Bounded to known projector paths; never mutates.
func InspectHookHealth(root string, doc ProjectDocument) hookproj.HookHealth {
	homePath, err := home.Resolve()
	if err != nil {
		return hookproj.HookHealth{
			Projections: []hookproj.ProjectionStatus{{
				State:   hookproj.StateMalformed,
				Message: err.Error(),
			}},
		}
	}
	projectID, idErr := home.ProjectID(root, doc.Project.Name)
	if idErr != nil {
		return hookproj.HookHealth{
			Projections: []hookproj.ProjectionStatus{{
				State:   hookproj.StateMalformed,
				Message: idErr.Error(),
			}},
		}
	}
	own, _ := hookproj.LoadOwnership(homePath, projectID)
	return hookproj.Inspect(hookproj.InspectInput{
		Root:       root,
		HomePath:   homePath,
		ProjectID:  projectID,
		Adapters:   normalizeSelectedAdapters(doc.Adapters.Selected),
		Projectors: HookProjectors(),
		Ownership:  own,
	})
}

// ReconcileHookProjections materializes/removes Atlas-owned native hook projections.
// Must run under mutatelock with Home+Workspace already held by the caller.
// reclaimUnowned is true only after an explicit Init Home reset.
func ReconcileHookProjections(root string, doc ProjectDocument, reclaimUnowned bool) (hookproj.ReconcileResult, error) {
	homePath, err := home.Resolve()
	if err != nil {
		return hookproj.ReconcileResult{}, fmt.Errorf("hooks reconcile: %w", err)
	}
	projectID, err := home.ProjectID(root, doc.Project.Name)
	if err != nil {
		return hookproj.ReconcileResult{}, fmt.Errorf("hooks reconcile: project id: %w", err)
	}
	own, err := hookproj.LoadOwnership(homePath, projectID)
	if err != nil {
		return hookproj.ReconcileResult{}, err
	}
	adaptersSelected := normalizeSelectedAdapters(doc.Adapters.Selected)
	// Only adapters that advertise ClassHooks participate.
	var hooksAdapters []string
	for _, a := range adaptersSelected {
		if adapters.Supports(adapters.ID(a), adapters.ClassHooks) {
			hooksAdapters = append(hooksAdapters, a)
		}
	}
	return hookproj.Reconcile(hookproj.ReconcileInput{
		Root:           root,
		HomePath:       homePath,
		ProjectID:      projectID,
		Adapters:       hooksAdapters,
		Projectors:     HookProjectors(),
		Ownership:      own,
		ReclaimUnowned: reclaimUnowned,
	})
}
