package openspec_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

// DOGFOOD-022: capability probes with SkipCapProbes=false (shim runners).

func capableRunner(version string) fakeRunner {
	help := func(cmd string) func([]string, string) (openspec.RunResult, error) {
		return func(args []string, _ string) (openspec.RunResult, error) {
			body := "Usage: openspec " + cmd + " [--json] [--tools]\n"
			return openspec.RunResult{Stdout: []byte(body)}, nil
		}
	}
	return fakeRunner{handlers: map[string]func([]string, string) (openspec.RunResult, error){
		"version --json": func([]string, string) (openspec.RunResult, error) {
			b, _ := json.Marshal(map[string]any{"version": version})
			return openspec.RunResult{Stdout: b}, nil
		},
		"list --json": func([]string, string) (openspec.RunResult, error) {
			b, _ := json.Marshal(map[string]any{"root": nil, "changes": []any{}, "status": []any{}})
			return openspec.RunResult{Stdout: b}, nil
		},
		"init --help":         help("init"),
		"update --help":       help("update"),
		"status --help":       help("status"),
		"instructions --help": help("instructions"),
		"validate --help":     help("validate"),
		"archive --help":      help("archive"),
		"init":                help("init"),
		"update":              help("update"),
		"status":              help("status"),
		"instructions":        help("instructions"),
		"validate":            help("validate"),
		"archive":             help("archive"),
	}}
}

func TestProbe_CapabilitiesOK_Ready(t *testing.T) {
	root := t.TempDir()
	writeReadyOpenSpecTree(t, root)
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		Root:          root,
		ProjectPin:    "1.14.1",
		LookPath:      func(string) (string, error) { return "/bin/openspec", nil },
		Runner:        capableRunner("1.14.1"),
		SkipCapProbes: false,
	})
	if h.State != openspec.StateReady || !h.CapabilitiesOK || !h.GovernedSDDOK {
		t.Fatalf("%#v", h)
	}
}

func TestProbe_MissingCapability_ListJSON(t *testing.T) {
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		LookPath: func(string) (string, error) { return "/bin/openspec", nil },
		Runner: fakeRunner{handlers: map[string]func([]string, string) (openspec.RunResult, error){
			"version --json": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte(`{"version":"1.14.1"}`)}, nil
			},
			"list --json": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{}, errors.New("list failed")
			},
		}},
		SkipCapProbes: false,
	})
	if h.State != openspec.StateIncompatible || h.IncompatKind != openspec.IncompatCapability {
		t.Fatalf("%#v", h)
	}
	if !strings.Contains(h.Message, "list --json") {
		t.Fatalf("message=%q", h.Message)
	}
}

func TestProbe_MissingCapability_InitToolsHelp(t *testing.T) {
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		LookPath: func(string) (string, error) { return "/bin/openspec", nil },
		Runner: fakeRunner{handlers: map[string]func([]string, string) (openspec.RunResult, error){
			"version --json": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte(`{"version":"1.14.1"}`)}, nil
			},
			"list --json": func([]string, string) (openspec.RunResult, error) {
				b, _ := json.Marshal(map[string]any{"root": nil})
				return openspec.RunResult{Stdout: b}, nil
			},
			"init --help": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte("Usage: openspec init\n")}, nil // no --tools
			},
			"update --help": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte("Usage: openspec update\n")}, nil
			},
			"status --help": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte("Usage: openspec status --json\n")}, nil
			},
			"instructions --help": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte("Usage: openspec instructions --json\n")}, nil
			},
			"validate --help": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte("Usage: openspec validate --json\n")}, nil
			},
			"archive --help": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte("Usage: openspec archive --json\n")}, nil
			},
		}},
		SkipCapProbes: false,
	})
	if h.State != openspec.StateIncompatible || h.IncompatKind != openspec.IncompatCapability {
		t.Fatalf("%#v", h)
	}
	if !strings.Contains(h.Message, "init --tools") {
		t.Fatalf("message=%q", h.Message)
	}
}

func TestProbe_NewerThanPin_ExactMismatch(t *testing.T) {
	// Exact pin wins even when newer CLI remains Atlas-compatible.
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		ProjectPin: "1.14.1",
		LookPath:   func(string) (string, error) { return "/bin/openspec", nil },
		Runner: fakeRunner{handlers: map[string]func([]string, string) (openspec.RunResult, error){
			"version --json": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte(`{"version":"1.99.0"}`)}, nil
			},
		}},
		SkipCapProbes: true,
	})
	if h.State != openspec.StateIncompatible || h.IncompatKind != openspec.IncompatProjectMismatch {
		t.Fatalf("%#v", h)
	}
	if openspec.AtlasCompatible(mustParse(t, "1.99.0")) != true {
		t.Fatal("1.99.0 should remain Atlas-compatible; mismatch is pin-exact only")
	}
}

func TestProbe_NoRepinOnMismatch(t *testing.T) {
	root := t.TempDir()
	writeReadyOpenSpecTree(t, root)
	cfg := filepath.Join(root, ".atlas")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	pinFile := filepath.Join(cfg, "pin.txt")
	if err := os.WriteFile(pinFile, []byte("1.14.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(pinFile)
	if err != nil {
		t.Fatal(err)
	}
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		Root:       root,
		ProjectPin: "1.14.1",
		LookPath:   func(string) (string, error) { return "/bin/openspec", nil },
		Runner: fakeRunner{handlers: map[string]func([]string, string) (openspec.RunResult, error){
			"version --json": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte(`{"version":"1.14.0"}`)}, nil
			},
		}},
		SkipCapProbes: true,
	})
	if h.State != openspec.StateIncompatible {
		t.Fatalf("%#v", h)
	}
	after, err := os.ReadFile(pinFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("Probe must not mutate pin side-channel files")
	}
}

func mustParse(t *testing.T, raw string) openspec.ParsedVersion {
	t.Helper()
	v, err := openspec.ParseVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
