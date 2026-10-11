package hooksengine

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// Env configures project resolution. Empty fields use process defaults.
// Resolution is read-only: it never creates Home, config, or project paths.
type Env struct {
	Root  string // workspace root; empty → Getwd
	Home  string // Atlas Home; empty → home.Resolve (identity only; unused for writes)
	Getwd func() (string, error)
}

type projectRef struct {
	Root      string
	Home      string
	ProjectID string
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
			return projectRef{}, fmt.Errorf("hooksengine: working directory: %w", err)
		}
		root = wd
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
	return projectRef{Root: root, Home: homePath, ProjectID: projectID}, nil
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
