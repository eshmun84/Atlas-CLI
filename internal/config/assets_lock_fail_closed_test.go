package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestBuildAssetsLockDocumentFor_ManifestRenderError(t *testing.T) {
	config.SetRenderRuntimeManifestYAMLForTest(func(projectName string, selected []string) (string, error) {
		return "", errors.New("injected manifest render failure")
	})
	t.Cleanup(func() { config.SetRenderRuntimeManifestYAMLForTest(nil) })

	doc := config.BuildProjectDocument(config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "lock-render",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	}), config.EmptyMCPDraft())
	lock, err := config.BuildAssetsLockDocumentFor("/tmp/atlas-home", doc, "0.1.0")
	if err == nil || !strings.Contains(err.Error(), "manifest render") {
		t.Fatalf("expected manifest render error, got %v", err)
	}
	if len(lock.Assets) != 0 {
		t.Fatalf("must not emit partial lock on render failure: %#v", lock)
	}
}

func TestRenderAssetsLockYAMLFor_PropagatesManifestError(t *testing.T) {
	config.SetRenderRuntimeManifestYAMLForTest(func(projectName string, selected []string) (string, error) {
		return "", errors.New("injected manifest render failure")
	})
	t.Cleanup(func() { config.SetRenderRuntimeManifestYAMLForTest(nil) })

	doc := config.BuildProjectDocument(config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "lock-yaml",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	}), config.EmptyMCPDraft())
	out, err := config.RenderAssetsLockYAMLFor("/tmp/atlas-home", doc)
	if err == nil || !strings.Contains(err.Error(), "manifest render") {
		t.Fatalf("expected manifest render error, got %v", err)
	}
	if out != "" {
		t.Fatalf("must not emit YAML on render failure: %q", out)
	}
}

func TestBuildAssetsLockDocumentFor_BundledAssetReadFailure(t *testing.T) {
	config.SetBundledAssetReadersForTest(
		func(path string) ([]byte, error) {
			return nil, errors.New("injected canonical read failure")
		},
		func(path string) ([]byte, error) {
			return nil, errors.New("injected embedded read failure")
		},
	)
	t.Cleanup(func() { config.SetBundledAssetReadersForTest(nil, nil) })

	doc := config.BuildProjectDocument(config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "lock-bundled",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	}), config.EmptyMCPDraft())
	lock, err := config.BuildAssetsLockDocumentFor("/tmp/atlas-home", doc, "0.1.0")
	if err == nil || !strings.Contains(err.Error(), "bundled asset") {
		t.Fatalf("expected bundled asset read error, got %v", err)
	}
	if len(lock.Assets) != 0 {
		t.Fatalf("must not emit partial lock on bundled read failure: %#v", lock)
	}
}
