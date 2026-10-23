package hookactions

import (
	"fmt"
	"os"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

// ProjectRef is a resolved, initialized Atlas project identity.
type ProjectRef struct {
	Root      string
	Home      string
	ProjectID string
	Name      string
	State     config.StateDocument
	// SessionID is optional. Set during admission so the worker can finalize
	// per-session fences (Phase B.8). Never used as a project identity.
	SessionID string
	// AdmissionID is an ephemeral ownership token for one Phase B lifecycle.
	// Empty identifies legacy callers only; it is never synthesized on recovery.
	AdmissionID string
}

// ResolveFunc resolves the workspace for a maintenance Action.
// Tests inject a fixed root/home; production uses process defaults.
type ResolveFunc func() (ProjectRef, error)

// DefaultResolve loads config + state from the working directory and Atlas Home.
// Read-only resolution except for interpreting existing files; it never creates
// Home or project paths.
func DefaultResolve() (ProjectRef, error) {
	return ResolveAt("", "")
}

// ResolveAt resolves an explicit workspace root and optional Home override.
func ResolveAt(root, homePath string) (ProjectRef, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return ProjectRef{}, fmt.Errorf("hookactions: working directory: %w", err)
		}
		root = wd
	}
	if err := config.RequireProjectSchemaCompatible(root); err != nil {
		return ProjectRef{}, fmt.Errorf("hookactions: project schema: %w", err)
	}
	cfg, err := config.LoadAt(root)
	if err != nil {
		return ProjectRef{}, fmt.Errorf("hookactions: not initialized: %w", err)
	}
	name := strings.TrimSpace(cfg.Project.Name)
	if name == "" {
		return ProjectRef{}, fmt.Errorf("hookactions: project name missing")
	}

	state, err := config.LoadStateDocumentAt(root)
	if err != nil {
		return ProjectRef{}, fmt.Errorf("hookactions: state unavailable: %w", err)
	}
	if !state.Initialized {
		return ProjectRef{}, fmt.Errorf("hookactions: project not initialized")
	}
	if strings.TrimSpace(state.ProjectName) == "" {
		state.ProjectName = name
	}

	projectID, err := home.ProjectID(root, state.ProjectName)
	if err != nil {
		return ProjectRef{}, fmt.Errorf("hookactions: project id: %w", err)
	}

	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		homePath, err = home.Resolve()
		if err != nil {
			return ProjectRef{}, fmt.Errorf("hookactions: Atlas Home: %w", err)
		}
	}
	return ProjectRef{
		Root:      root,
		Home:      homePath,
		ProjectID: projectID,
		Name:      state.ProjectName,
		State:     state,
	}, nil
}

func fixedResolve(ref ProjectRef) ResolveFunc {
	return func() (ProjectRef, error) { return ref, nil }
}
