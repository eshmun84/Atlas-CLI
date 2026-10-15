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

type fakeRunner struct {
	handlers map[string]func(args []string, dir string) (openspec.RunResult, error)
}

func (f fakeRunner) Run(_ context.Context, _ string, args []string, dir string) (openspec.RunResult, error) {
	key := strings.Join(args, " ")
	if h, ok := f.handlers[key]; ok {
		return h(args, dir)
	}
	// Prefix match for help/list variants.
	for k, h := range f.handlers {
		if strings.HasPrefix(key, k) || strings.HasPrefix(k, args[0]) {
			if strings.HasPrefix(key, args[0]) {
				return h(args, dir)
			}
		}
	}
	if h, ok := f.handlers[args[0]]; ok {
		return h(args, dir)
	}
	return openspec.RunResult{}, errors.New("unhandled " + key)
}

func TestProbe_Missing(t *testing.T) {
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		LookPath: func(string) (string, error) { return "", errors.New("missing") },
	})
	if h.State != openspec.StateMissing || h.GovernedSDDOK {
		t.Fatalf("%#v", h)
	}
}

func TestProbe_UnsupportedVersion(t *testing.T) {
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		LookPath: func(string) (string, error) { return "/bin/openspec", nil },
		Runner: fakeRunner{handlers: map[string]func([]string, string) (openspec.RunResult, error){
			"version --json": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte(`{"version":"1.13.0"}`)}, nil
			},
		}},
		SkipCapProbes: true,
	})
	if h.State != openspec.StateIncompatible || h.IncompatKind != openspec.IncompatUnsupported {
		t.Fatalf("%#v", h)
	}
}

func TestProbe_PinMismatch(t *testing.T) {
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		ProjectPin: "1.14.2",
		LookPath:   func(string) (string, error) { return "/bin/openspec", nil },
		Runner: fakeRunner{handlers: map[string]func([]string, string) (openspec.RunResult, error){
			"version --json": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte(`{"version":"1.14.1"}`)}, nil
			},
		}},
		SkipCapProbes: true,
	})
	if h.State != openspec.StateIncompatible || h.IncompatKind != openspec.IncompatProjectMismatch {
		t.Fatalf("%#v", h)
	}
}

func TestProbe_PinMismatchPrerelease(t *testing.T) {
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		ProjectPin: "1.14.2",
		LookPath:   func(string) (string, error) { return "/bin/openspec", nil },
		Runner: fakeRunner{handlers: map[string]func([]string, string) (openspec.RunResult, error){
			"version --json": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte(`{"version":"1.14.2-beta.1"}`)}, nil
			},
		}},
		SkipCapProbes: true,
	})
	if h.State != openspec.StateIncompatible || h.IncompatKind != openspec.IncompatProjectMismatch {
		t.Fatalf("%#v", h)
	}
	if !strings.Contains(h.Message, "1.14.2-beta.1") {
		t.Fatalf("message should preserve prerelease: %q", h.Message)
	}
}

func TestProbe_Ready(t *testing.T) {
	root := t.TempDir()
	writeReadyOpenSpecTree(t, root)
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		Root:       root,
		ProjectPin: "1.14.2",
		LookPath:   func(string) (string, error) { return "/bin/openspec", nil },
		Runner: fakeRunner{handlers: map[string]func([]string, string) (openspec.RunResult, error){
			"version --json": func([]string, string) (openspec.RunResult, error) {
				b, _ := json.Marshal(map[string]any{"version": "1.14.2"})
				return openspec.RunResult{Stdout: b}, nil
			},
		}},
		SkipCapProbes: true,
	})
	if h.State != openspec.StateReady || !h.GovernedSDDOK {
		t.Fatalf("%#v", h)
	}
}

func TestProbe_LegacyMkdirAvailable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := openspec.Probe(context.Background(), openspec.ProbeInput{
		Root:     root,
		LookPath: func(string) (string, error) { return "/bin/openspec", nil },
		Runner: fakeRunner{handlers: map[string]func([]string, string) (openspec.RunResult, error){
			"version --json": func([]string, string) (openspec.RunResult, error) {
				return openspec.RunResult{Stdout: []byte(`{"version":"1.14.2"}`)}, nil
			},
		}},
		SkipCapProbes: true,
	})
	if h.State != openspec.StateAvailable || h.Project != openspec.ProjectLegacyMkdir || h.GovernedSDDOK {
		t.Fatalf("%#v", h)
	}
}
