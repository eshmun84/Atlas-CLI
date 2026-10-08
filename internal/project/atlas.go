package project

import (
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

// Atlas project setup states for Status overview.
const (
	AtlasStateNotInitialized = "Not initialized"
	AtlasStateInitialized    = "Initialized"
	AtlasStatePartialSetup   = "Partial setup"
	AtlasStateInvalidConfig  = "Invalid config"
)

// AtlasStatus is the read-only Atlas setup evaluation for a workspace.
type AtlasStatus struct {
	State      string
	ConfigPath string
	Config     config.Config
	HasConfig  bool
}

// EvaluateAtlasStatus inspects Atlas setup without mutating the workspace.
func EvaluateAtlasStatus(root string, files FileInfo) AtlasStatus {
	status := AtlasStatus{
		ConfigPath: filepath.Join(root, config.FileConfig),
		HasConfig:  files.HasAtlasConfig,
	}

	switch {
	case files.HasAtlasConfig:
		cfg, err := config.Load(status.ConfigPath)
		if err != nil {
			status.State = AtlasStateInvalidConfig
			return status
		}
		status.State = AtlasStateInitialized
		status.Config = cfg
		return status
	case files.HasAtlasDir:
		status.State = AtlasStatePartialSetup
		return status
	default:
		status.State = AtlasStateNotInitialized
		return status
	}
}

// Initialized reports whether a valid Atlas config is present.
func (s AtlasStatus) Initialized() bool {
	return s.State == AtlasStateInitialized
}
