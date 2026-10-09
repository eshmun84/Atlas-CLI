package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"gopkg.in/yaml.v3"
)

func TestLoadRoundTripFromFixture(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}

	original := config.DefaultConfig("roundtrip")
	original.Project.Description = "slice-2 roundtrip"
	original.Project.Technologies = []string{"go"}
	data, err := yaml.Marshal(&original)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := config.LoadAt(root)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Project.Name != original.Project.Name {
		t.Fatalf("name = %q, want %q", loaded.Project.Name, original.Project.Name)
	}
	if loaded.Project.Description != original.Project.Description {
		t.Fatalf("description = %q, want %q", loaded.Project.Description, original.Project.Description)
	}
	if loaded.Governance.StorageMode != original.Governance.StorageMode {
		t.Fatalf("storage_mode = %q, want %q", loaded.Governance.StorageMode, original.Governance.StorageMode)
	}
	if loaded.Memory.SQLitePath != original.Memory.SQLitePath {
		t.Fatalf("sqlite_path = %q, want %q", loaded.Memory.SQLitePath, original.Memory.SQLitePath)
	}
	if len(loaded.Project.Technologies) != 1 || loaded.Project.Technologies[0] != "go" {
		t.Fatalf("technologies = %#v, want [go]", loaded.Project.Technologies)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}

	invalidYAML := []byte("project:\n  name: \"\"\n  mode: greenfield\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), invalidYAML, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := config.LoadAt(root); err == nil {
		t.Fatal("expected load to reject invalid config")
	}
}

func TestLoadStateDocument_RoundTrip(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte("schema_version: 1\ninitialized: true\nruntime_materialized: true\nruntime_materialized_at: \"2026-10-05T12:00:00Z\"\napplied_at: \"2026-10-05T12:00:00Z\"\natlas_version: 0.1.0\nproject_name: demo\n")
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileState)), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	state, err := config.LoadStateDocumentAt(root)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if !state.Initialized || !state.RuntimeMaterialized || state.ProjectName != "demo" {
		t.Fatalf("state = %#v", state)
	}
}

func TestInspectAgentsMarkers(t *testing.T) {
	t.Parallel()

	complete := config.InspectAgentsMarkers([]byte(mustRenderAgentsMD(t, "demo", true, nil, nil)))
	if !complete.Complete() {
		t.Fatalf("complete markers = %#v", complete)
	}
	partial := config.InspectAgentsMarkers([]byte("<!-- ATLAS:BASE:BEGIN -->\n"))
	if partial.Complete() || !partial.BaseBegin || partial.UserEnd {
		t.Fatalf("partial markers = %#v", partial)
	}
}
