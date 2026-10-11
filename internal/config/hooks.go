package config

import (
	"fmt"

	"github.com/eshmun84/Atlas-CLI/internal/adapters"
	adaptercursor "github.com/eshmun84/Atlas-CLI/internal/adapters/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/hookproj"
)

func init() {
	hookproj.RegisterDefaultProjector(adaptercursor.NewHooksProjector())
}

// HookProjectors returns registered hooks projectors.
func HookProjectors() map[adapters.ID]hookproj.Projector {
	return hookproj.DefaultProjectors()
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
