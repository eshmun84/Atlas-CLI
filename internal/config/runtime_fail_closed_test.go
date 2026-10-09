package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func TestRenderAgentsMD_SelectedAdapterAssetFailure(t *testing.T) {
	config.SetLoadAgentsAssetForTest(func(rel string) (string, error) {
		if strings.Contains(rel, "adapters/cursor") {
			return "", errors.New("injected cursor adapter asset failure")
		}
		return config.LoadAgentsAssetDefaultForTest(rel)
	})
	t.Cleanup(func() { config.SetLoadAgentsAssetForTest(nil) })

	out, err := config.RenderAgentsMD("demo", true, []string{"cursor"}, nil)
	if err == nil || !strings.Contains(err.Error(), "cursor") {
		t.Fatalf("expected adapter failure, got out=%q err=%v", out, err)
	}
	if out != "" {
		t.Fatalf("partial AGENTS.md must not be returned: %q", out)
	}
}

func TestRenderAgentsMD_BaseAssetFailure(t *testing.T) {
	config.SetLoadAgentsAssetForTest(func(rel string) (string, error) {
		if rel == "agents/base.md" {
			return "", errors.New("injected base asset failure")
		}
		return config.LoadAgentsAssetDefaultForTest(rel)
	})
	t.Cleanup(func() { config.SetLoadAgentsAssetForTest(nil) })

	out, err := config.RenderAgentsMD("demo", true, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "base") {
		t.Fatalf("expected base failure, got out=%q err=%v", out, err)
	}
	if out != "" {
		t.Fatalf("partial AGENTS.md must not be returned: %q", out)
	}
}

func TestRenderAgentsMD_HealthyUnchanged(t *testing.T) {
	got := mustRenderAgentsMD(t, "Demo", true, []string{"cursor"}, nil)
	if !strings.Contains(got, config.AgentsBaseBegin) || !strings.Contains(got, config.AdapterBlockBegin("cursor")) {
		t.Fatalf("unexpected healthy output:\n%s", got)
	}
}

func TestApplyConfig_AdapterAssetFailureBlocks(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(home.EnvAtlasHome, homeDir)
	root := t.TempDir()
	config.SetLoadAgentsAssetForTest(func(rel string) (string, error) {
		if strings.Contains(rel, "adapters/cursor") {
			return "", errors.New("injected cursor adapter asset failure")
		}
		return config.LoadAgentsAssetDefaultForTest(rel)
	})
	t.Cleanup(func() { config.SetLoadAgentsAssetForTest(nil) })

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "asset-fail",
		ProjectMode:   "new",
		DefaultRemote: "origin",
	})
	if !draft.ToggleMulti("adapters.selected", "cursor") {
		t.Fatal("toggle cursor")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC) },
	})
	if err == nil || !strings.Contains(err.Error(), "cursor") {
		t.Fatalf("expected apply blocked on adapter asset, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("AGENTS.md must not be written: %v", err)
	}
}

func TestRenderAtlasAgent_EmbeddedErrorContext(t *testing.T) {
	config.SetRenderAtlasAgentReadersForTest(
		func(path string) ([]byte, error) {
			return nil, errors.New("canonical gone")
		},
		func(name string) (string, error) {
			return "", errors.New("embedded gone")
		},
	)
	t.Cleanup(func() { config.SetRenderAtlasAgentReadersForTest(nil, nil) })

	_, err := config.RenderAtlasAgent("atlas-worker.md")
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "embedded") || !strings.Contains(msg, "embedded gone") {
		t.Fatalf("must surface embedded failure, got %v", err)
	}
	if !strings.Contains(msg, "canonical gone") {
		t.Fatalf("must retain home canonical context, got %v", err)
	}
}
