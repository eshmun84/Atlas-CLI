package home_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"gopkg.in/yaml.v3"
)

func TestCheckStateSchema_Current(t *testing.T) {
	err := home.CheckStateSchema(home.StateDocument{SchemaVersion: home.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckStateSchema_Newer(t *testing.T) {
	err := home.CheckStateSchema(home.StateDocument{SchemaVersion: home.CurrentSchemaVersion + 1})
	if err == nil || !errors.Is(err, home.ErrSchemaNewer) {
		t.Fatalf("want ErrSchemaNewer, got %v", err)
	}
	if !strings.Contains(err.Error(), "newer") || !strings.Contains(err.Error(), "no files were modified") {
		t.Fatalf("message: %v", err)
	}
}

func TestCheckStateSchema_OlderUnsupported(t *testing.T) {
	// Simulate unsupported older by checking a fictional older value when current is 1:
	// with CurrentSchemaVersion==1, older means we need a temporary override — use 0 as missing.
	err := home.CheckStateSchema(home.StateDocument{SchemaVersion: 0})
	if err == nil || !errors.Is(err, home.ErrSchemaMissing) {
		t.Fatalf("want ErrSchemaMissing for 0, got %v", err)
	}
}

func TestCheckStateSchema_Malformed(t *testing.T) {
	err := home.CheckStateSchema(home.StateDocument{SchemaVersion: -1})
	if err == nil || !errors.Is(err, home.ErrSchemaMalformed) {
		t.Fatalf("want ErrSchemaMalformed, got %v", err)
	}
}

func TestEnsureAndMirror_RefusesNewerSchema_NoMutation(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	before := listRel(t, homeDir)

	// Artificially bump Home schema.
	statePath := filepath.Join(homeDir, "state", "home.yaml")
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var doc home.StateDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc.SchemaVersion = home.CurrentSchemaVersion + 1
	doc.AtlasVersion = "9.9.9"
	out, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, out, 0o600); err != nil {
		t.Fatal(err)
	}
	beforePoison := listRel(t, homeDir)

	_, err = home.EnsureAndMirror(time.Now().UTC())
	if err == nil || !errors.Is(err, home.ErrSchemaNewer) {
		t.Fatalf("expected refuse remirror on newer schema, got %v", err)
	}
	after := listRel(t, homeDir)
	if strings.Join(after, "\n") != strings.Join(beforePoison, "\n") {
		t.Fatalf("Home mutated despite schema block\nbefore=%v\nafter=%v\norig=%v", beforePoison, after, before)
	}
	// Schema stamp unchanged.
	raw2, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var doc2 home.StateDocument
	if err := yaml.Unmarshal(raw2, &doc2); err != nil {
		t.Fatal(err)
	}
	if doc2.SchemaVersion != home.CurrentSchemaVersion+1 {
		t.Fatalf("schema rewritten: %d", doc2.SchemaVersion)
	}
}

func TestEnsureAndMirror_AllowsOlderAtlasVersionSameSchema(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(homeDir, "state", "home.yaml")
	raw, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var doc home.StateDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	doc.AtlasVersion = "0.0.9"
	doc.SchemaVersion = home.CurrentSchemaVersion
	out, err := yaml.Marshal(&doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, out, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatalf("product version mismatch alone must not block: %v", err)
	}
	st := home.Inspect()
	if st.SchemaError != "" {
		t.Fatalf("schema error unexpected: %s", st.SchemaError)
	}
	if !home.Healthy(st) {
		t.Fatalf("home should be healthy: %#v", st)
	}
}

func TestRequireHealthy_SchemaIncompatible(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(homeDir, "state", "home.yaml")
	raw, _ := os.ReadFile(statePath)
	var doc home.StateDocument
	_ = yaml.Unmarshal(raw, &doc)
	doc.SchemaVersion = home.CurrentSchemaVersion + 1
	out, _ := yaml.Marshal(&doc)
	_ = os.WriteFile(statePath, out, 0o600)

	st, err := home.RequireHealthy()
	if err == nil {
		t.Fatal("expected unhealthy")
	}
	if st.SchemaError == "" {
		t.Fatal("expected SchemaError on status")
	}
	if home.NeedsRepair(st) {
		t.Fatal("schema incompatibility must not advertise Runtime Repair")
	}
	if !strings.Contains(err.Error(), "schema") {
		t.Fatalf("error: %v", err)
	}
}

func listRel(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if rel == "." {
			return nil
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}
