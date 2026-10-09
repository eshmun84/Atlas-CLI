package config_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestInspectMCPHealth_ResolveError(t *testing.T) {
	config.SetResolveHomeForTest(func() (string, error) {
		return "", errors.New("injected resolve failure")
	})
	t.Cleanup(func() { config.SetResolveHomeForTest(nil) })

	doc := config.BuildProjectDocument(config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "mcp-ro",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	}), config.EmptyMCPDraft())
	h, err := config.InspectMCPHealth(t.TempDir(), doc)
	if err == nil || !strings.Contains(err.Error(), "resolve Atlas Home") {
		t.Fatalf("expected resolve error, got %v", err)
	}
	if h.Ready {
		t.Fatalf("Ready must be false on resolve error: %#v", h)
	}
}

func TestInspectMCPHealth_ProjectIDError(t *testing.T) {
	config.SetProjectIDForTest(func(root, projectName string) (string, error) {
		return "", errors.New("injected project id failure")
	})
	t.Cleanup(func() { config.SetProjectIDForTest(nil) })

	doc := config.BuildProjectDocument(config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "mcp-pid",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	}), config.EmptyMCPDraft())
	h, err := config.InspectMCPHealth(t.TempDir(), doc)
	if err == nil || !strings.Contains(err.Error(), "project id") {
		t.Fatalf("expected project id error, got %v", err)
	}
	if h.Ready {
		t.Fatalf("Ready must be false on project id error: %#v", h)
	}
}
