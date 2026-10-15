package cli_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
)

func TestResolve_CapabilityPlan(t *testing.T) {
	action := cli.Resolve([]string{"capability", "plan", "update README", "--json"})
	if action.Mode != cli.ModeCapability {
		t.Fatalf("mode=%v", action.Mode)
	}
	if action.Capability.Task != "update README" || !action.Capability.JSON {
		t.Fatalf("cmd=%#v", action.Capability)
	}
	action = cli.Resolve([]string{"capability", "plan", "x", "--json", "--phase", "implement"})
	if action.Capability.Phase != "implement" {
		t.Fatalf("phase=%q", action.Capability.Phase)
	}
}

func TestExecute_CapabilityPlanJSON(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("ATLAS_HOME", home)
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	var buf bytes.Buffer
	if err := cli.Execute(&buf, &bytes.Buffer{}, []string{"capability", "plan", "update README", "--json"}); err != nil {
		t.Fatalf("execute: %v\n%s", err, buf.String())
	}
	var env map[string]any
	if err := json.Unmarshal(buf.Bytes(), &env); err != nil {
		t.Fatalf("json: %v body=%s", err, buf.String())
	}
	if env["command"] != "plan" {
		t.Fatalf("command=%v", env["command"])
	}
	if env["status"] != "success" {
		t.Fatalf("status=%v body=%s", env["status"], buf.String())
	}
	caps, _ := env["capabilities"].([]any)
	foundDocs := false
	for _, c := range caps {
		m, _ := c.(map[string]any)
		if m["id"] == "documentation" {
			foundDocs = true
		}
		if m["id"] == "security-review" {
			t.Fatal("security must not appear for README")
		}
	}
	if !foundDocs {
		t.Fatalf("want documentation capability: %s", buf.String())
	}
}

func TestExecute_CapabilityPlanReadOnly(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("ATLAS_HOME", home)

	// Seed representative trees.
	mustMk := func(parts ...string) string {
		p := filepath.Join(append([]string{root}, parts...)...)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := os.WriteFile(mustMk("go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".atlas", "config.yaml"), []byte("project:\n  name: demo\ngovernance:\n  workflow: sdd\n  spec_engine: openspec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{".cursor", ".opencode", "openspec/changes", ".git"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	memRel := filepath.Join(home, "projects", "x", "memory")
	if err := os.MkdirAll(memRel, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(memRel, "memory.db"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}
	cg := filepath.Join(home, "projects", "x", "codegraph")
	if err := os.MkdirAll(cg, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cg, "graph.db"), []byte("seed"), 0o644); err != nil {
		t.Fatal(err)
	}

	beforeRoot := fingerprintTree(t, root)
	beforeHome := fingerprintTree(t, home)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })

	var buf bytes.Buffer
	if err := cli.Execute(&buf, &bytes.Buffer{}, []string{"capability", "plan", "who calls ParseConfig?", "--json"}); err != nil {
		// blocked is not expected; invalid might be
		if !strings.Contains(buf.String(), `"status"`) {
			t.Fatalf("execute: %v body=%s", err, buf.String())
		}
	}

	afterRoot := fingerprintTree(t, root)
	afterHome := fingerprintTree(t, home)
	if beforeRoot != afterRoot {
		t.Fatalf("project tree mutated\nbefore=%s\nafter=%s", beforeRoot, afterRoot)
	}
	if beforeHome != afterHome {
		t.Fatalf("ATLAS_HOME mutated\nbefore=%s\nafter=%s", beforeHome, afterHome)
	}
}

func fingerprintTree(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		h.Write([]byte(rel))
		h.Write([]byte("|"))
		h.Write([]byte(info.Mode().String()))
		h.Write([]byte("|"))
		if info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			h.Write(data)
		}
		h.Write([]byte("\n"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(h.Sum(nil))
}
