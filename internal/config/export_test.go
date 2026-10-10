package config

import (
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

// SetAfterSkillsReconcileHookForTest installs a test-only seam after successful
// skill projection reconcile in ApplyConfig / PersistConfigure.
func SetAfterSkillsReconcileHookForTest(fn func() error) {
	afterSkillsReconcileHook = fn
}

// RestoreConfigureFilesForTest exposes configure rollback for fail-closed tests.
func RestoreConfigureFilesForTest(root string, homePath, projectID string, ownershipExists bool, ownershipRaw []byte, ownershipMode os.FileMode) error {
	return restoreConfigureFiles(root, configureMutationSnapshot{
		homePath:        homePath,
		projectID:       projectID,
		ownershipExists: ownershipExists,
		ownershipRaw:    ownershipRaw,
		ownershipMode:   ownershipMode,
	})
}

// CaptureOwnershipBaselineForTest exposes Home-rooted ownership capture.
func CaptureOwnershipBaselineForTest(homePath, projectID string) (exists bool, raw []byte, mode os.FileMode, err error) {
	return captureOwnershipBaseline(homePath, projectID)
}

// SetResolveHomeForTest installs a test-only Atlas Home resolve seam.
func SetResolveHomeForTest(fn func() (string, error)) {
	if fn == nil {
		resolveHomeFn = home.Resolve
		return
	}
	resolveHomeFn = fn
}

// SetProjectIDForTest installs a test-only ProjectID seam.
func SetProjectIDForTest(fn func(root, projectName string) (string, error)) {
	if fn == nil {
		projectIDFn = home.ProjectID
		return
	}
	projectIDFn = fn
}

// SetRenderRuntimeManifestYAMLForTest installs a test-only manifest render seam.
func SetRenderRuntimeManifestYAMLForTest(fn func(projectName string, selected []string) (string, error)) {
	if fn == nil {
		renderRuntimeManifestYAMLFn = RenderRuntimeManifestYAML
		return
	}
	renderRuntimeManifestYAMLFn = fn
}

// SetBundledAssetReadersForTest installs test-only canonical/embedded read seams.
func SetBundledAssetReadersForTest(
	canonical func(path string) ([]byte, error),
	embedded func(path string) ([]byte, error),
) {
	if canonical == nil {
		readCanonicalAssetFn = home.ReadCanonical
	} else {
		readCanonicalAssetFn = canonical
	}
	if embedded == nil {
		readEmbeddedAssetFn = func(path string) ([]byte, error) {
			return assets.Content.ReadFile(path)
		}
	} else {
		readEmbeddedAssetFn = embedded
	}
}

// SetLoadAgentsAssetForTest installs a test-only agents/projection asset reader.
func SetLoadAgentsAssetForTest(fn func(rel string) (string, error)) {
	if fn == nil {
		loadAgentsAssetFn = loadAgentsAssetDefault
		return
	}
	loadAgentsAssetFn = fn
}

// LoadAgentsAssetDefaultForTest exposes the production asset loader for partial stubs.
func LoadAgentsAssetDefaultForTest(rel string) (string, error) {
	return loadAgentsAssetDefault(rel)
}

// SetRenderAtlasAgentReadersForTest installs test-only agent content readers.
func SetRenderAtlasAgentReadersForTest(
	canonical func(path string) ([]byte, error),
	embedded func(name string) (string, error),
) {
	if canonical == nil {
		readCanonicalAgentFn = home.ReadCanonical
	} else {
		readCanonicalAgentFn = canonical
	}
	if embedded == nil {
		readRuntimeAgentFn = assets.ReadRuntimeAgent
	} else {
		readRuntimeAgentFn = embedded
	}
}
