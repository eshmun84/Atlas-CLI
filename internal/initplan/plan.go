package initplan

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
)

// Step statuses for the dry-run init plan.
const (
	StatusCreate       = "create"
	StatusSkipExisting = "skip-existing"
	StatusFuture       = "future"
	StatusWarning      = "warning"
)

// Plan is a read-only Atlas initialization plan. It never materializes files.
type Plan struct {
	RootPath          string
	ProjectName       string
	ProjectMode       string
	ConfigStorageMode string
	MemoryProvider    string
	Steps             []Step
	Warnings          []string
}

// Step is one planned materialization action.
type Step struct {
	Action string
	Path   string
	Status string
	Reason string
}

type plannedFile struct {
	path     string
	reason   string
	status   string
	fallback string
}

// Options tunes dry-run plan inference. Empty Options preserves default behavior.
type Options struct {
	// ModeOverride forces project mode for plan preview: "", "auto", "new", or "existing".
	ModeOverride string
	// ProjectName overrides the inferred folder-based project name when non-empty.
	ProjectName string
}

// Build infers a dry-run init plan from the canonical inspection.
func Build(root string, result inspect.Inspection) (Plan, error) {
	return BuildWithOptions(root, result, Options{})
}

// BuildWithOptions infers a dry-run init plan, optionally overriding project mode.
func BuildWithOptions(root string, result inspect.Inspection, opts Options) (Plan, error) {
	if root == "" {
		return Plan{}, fmt.Errorf("root path is required")
	}

	absRoot := root
	if result.RootPath != "" {
		absRoot = result.RootPath
	}

	name := inferProjectName(absRoot)
	if opts.ProjectName != "" {
		name = opts.ProjectName
	}
	detected, err := inferProjectMode(absRoot, result.Files)
	if err != nil {
		return Plan{}, err
	}
	mode, err := applyModeOverride(detected, opts.ModeOverride)
	if err != nil {
		return Plan{}, err
	}

	defaults := config.DefaultConfig(name)
	defaults.Project.Mode = mode

	warnings := []string{
		"This is a dry-run plan.",
		"No files were created.",
		"Materialization will require explicit approval in a later slice.",
	}
	if len(result.RuntimeArtifacts) > 0 {
		warnings = append(warnings,
			"Existing runtime/adaptor artifacts were detected.",
			"Atlas will not merge them; future init will back them up and replace them.",
			"AGENTS.md will use Atlas-managed sections plus a preserved user section on later updates.",
		)
	}

	plan := Plan{
		RootPath:          absRoot,
		ProjectName:       name,
		ProjectMode:       mode,
		ConfigStorageMode: defaults.Governance.StorageMode,
		MemoryProvider:    defaults.Memory.Provider,
		Warnings:          warnings,
	}

	files := []plannedFile{
		{path: "AGENTS.md", reason: "project runtime governance entrypoint", fallback: StatusCreate},
		{path: config.FileConfig, reason: "project Atlas configuration", fallback: StatusCreate},
		{path: config.FileLocal, reason: "local machine/user settings", fallback: StatusCreate},
		{path: config.FileState, reason: "generated setup state", fallback: StatusCreate},
		{path: config.FileAssetsLock, reason: "installed assets lock", fallback: StatusCreate},
		{path: config.FileCapsule, reason: "compact agent context", fallback: StatusCreate},
		{path: config.FileMemoryDB, reason: "SQLite memory store", fallback: StatusFuture},
	}

	for _, file := range files {
		plan.Steps = append(plan.Steps, stepFor(absRoot, file))
	}

	return plan, nil
}

func inferProjectName(root string) string {
	name := filepath.Base(filepath.Clean(root))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "project"
	}
	return name
}

func inferProjectMode(root string, files project.FileInfo) (string, error) {
	if files.HasAtlasConfig {
		return config.ModeExisting, nil
	}

	empty, err := isEmptyDir(root)
	if err != nil {
		return "", err
	}
	if empty {
		return config.ModeGreenfield, nil
	}
	return config.ModeExisting, nil
}

func applyModeOverride(detected, override string) (string, error) {
	switch override {
	case "", "auto":
		return detected, nil
	case "new":
		return config.ModeGreenfield, nil
	case "existing":
		return config.ModeExisting, nil
	default:
		return "", fmt.Errorf("invalid mode override %q", override)
	}
}

func isEmptyDir(root string) (bool, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false, fmt.Errorf("inspect workspace: %w", err)
	}
	return len(entries) == 0, nil
}

func stepFor(root string, file plannedFile) Step {
	status := file.fallback
	reason := file.reason
	if exists(filepath.Join(root, file.path)) {
		status = StatusSkipExisting
		if file.path == config.FileConfig {
			reason = "existing Atlas config detected"
		} else {
			reason = "already exists"
		}
	}

	return Step{
		Action: status,
		Path:   file.path,
		Status: status,
		Reason: reason,
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
