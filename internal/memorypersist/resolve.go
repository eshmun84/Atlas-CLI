package memorypersist

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// Env configures project resolution. Empty fields use process defaults.
type Env struct {
	Root  string // workspace root; empty → Getwd
	Home  string // Atlas Home; empty → home.Resolve
	Getwd func() (string, error)
	Clock func() time.Time
}

type projectRef struct {
	Root      string
	Home      string
	ProjectID string
	Mode      string // persistence mode from config
}

func (e Env) resolveProject() (projectRef, error) {
	root := strings.TrimSpace(e.Root)
	if root == "" {
		getwd := e.Getwd
		if getwd == nil {
			getwd = os.Getwd
		}
		wd, err := getwd()
		if err != nil {
			return projectRef{}, fmt.Errorf("memorypersist: working directory: %w", err)
		}
		root = wd
	}

	if err := config.RequireProjectSchemaCompatible(root); err != nil {
		return projectRef{}, mapConfigLoadError(err)
	}
	cfg, err := config.LoadAt(root)
	if err != nil {
		return projectRef{}, mapConfigLoadError(err)
	}
	name := strings.TrimSpace(cfg.Project.Name)
	if name == "" {
		return projectRef{}, ErrNotInitialized
	}
	projectID, err := home.ProjectID(root, name)
	if err != nil {
		return projectRef{}, err
	}

	homePath := strings.TrimSpace(e.Home)
	if homePath == "" {
		homePath, err = home.Resolve()
		if err != nil {
			return projectRef{}, err
		}
	}

	mode := strings.TrimSpace(cfg.Memory.Persistence.Mode)
	if mode == "" {
		mode = config.MemoryPersistenceGoverned
	}
	if !cfg.Memory.Enabled {
		mode = config.MemoryPersistenceOff
	}

	return projectRef{Root: root, Home: homePath, ProjectID: projectID, Mode: mode}, nil
}

func mapConfigLoadError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, os.ErrNotExist):
		return fmt.Errorf("%w: %w", ErrNotInitialized, err)
	case errors.Is(err, fsafety.ErrSymlink),
		errors.Is(err, fsafety.ErrPathTraversal),
		errors.Is(err, fsafety.ErrPathEscape),
		errors.Is(err, fsafety.ErrAbsolutePath),
		errors.Is(err, fsafety.ErrNotRegularFile):
		return fmt.Errorf("%w: %w", ErrUnsafeFilesystem, err)
	default:
		return err
	}
}

func (e Env) now() time.Time {
	if e.Clock != nil {
		return e.Clock().UTC()
	}
	return time.Now().UTC()
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}
