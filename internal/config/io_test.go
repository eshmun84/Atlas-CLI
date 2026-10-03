package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	original := config.DefaultConfig("roundtrip")
	original.Project.Description = "slice-2 roundtrip"
	original.Project.Technologies = []string{"go"}

	if err := config.Save(path, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := config.Load(path)
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

func TestSaveCreatesParentDirectories(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, ".atlas", "nested", "config.yaml")

	cfg := config.DefaultConfig("nested")
	if err := config.Save(path, cfg); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat saved config: %v", err)
	}
	if info.IsDir() {
		t.Fatal("expected a file, got directory")
	}
}

func TestSaveRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := config.DefaultConfig("bad")
	cfg.Project.Name = ""

	if err := config.Save(path, cfg); err == nil {
		t.Fatal("expected save to reject invalid config")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("config file should not exist, stat err = %v", err)
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	invalidYAML := []byte("project:\n  name: \"\"\n  mode: greenfield\n")
	if err := os.WriteFile(path, invalidYAML, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if _, err := config.Load(path); err == nil {
		t.Fatal("expected load to reject invalid config")
	}
}
