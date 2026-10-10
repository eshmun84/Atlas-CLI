package memoryretrieve

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

var (
	// ErrNotInitialized reports missing Atlas project identity/config for Memory CLI.
	ErrNotInitialized = errors.New("memoryretrieve: atlas project not initialized")
	// ErrUnsafeFilesystem reports a fail-closed path safety refusal (symlink/traversal).
	ErrUnsafeFilesystem = errors.New("memoryretrieve: unsafe filesystem")
)

// Env configures read-only resolution. Empty fields use process defaults.
type Env struct {
	Root  string // workspace root; empty → Getwd
	Home  string // Atlas Home; empty → home.Resolve
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
			return projectRef{}, fmt.Errorf("memoryretrieve: working directory: %w", err)
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
		// Identity derivation failures are unexpected after a valid config load.
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

// mapConfigLoadError classifies config.LoadAt failures without string matching.
//
//	missing file          → ErrNotInitialized (+ os.ErrNotExist)
//	fsafety path refusal  → ErrUnsafeFilesystem (+ fsafety sentinel)
//	parse/validate/other  → original error (classify → internal_error)
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
