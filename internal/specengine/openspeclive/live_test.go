//go:build openspec_live

// Package openspeclive is the opt-in contract against a real OpenSpec CLI.
//
// It is excluded from go test ./...
//
//	go test -tags openspec_live ./internal/specengine/openspeclive -count=1
//
// A missing openspec binary fails the test. This package does not skip, and it
// does not install the contract double. Passing it is the only standard claim
// of compatibility with the CLI on PATH.
package openspeclive

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
	"github.com/eshmun84/Atlas-CLI/internal/testharness"
)

func TestLiveOpenSpec_ProbeAndInit(t *testing.T) {
	t.Setenv("ATLAS_HOME", t.TempDir())
	exe, err := exec.LookPath(openspec.ExecutableName)
	if err != nil {
		t.Fatalf("openspec_live requires the real CLI on PATH: %v", err)
	}
	if strings.Contains(exe, "atlas-contract-bin-") {
		t.Fatalf("refusing contract double at %s; run this tag without the standard-suite PATH prefix", exe)
	}

	root := t.TempDir()
	health := openspec.Probe(context.Background(), openspec.ProbeInput{Root: root})
	if health.State == openspec.StateMissing || health.State == openspec.StateError || health.State == openspec.StateIncompatible {
		t.Fatalf("live probe state=%s message=%s", health.State, health.Message)
	}
	if !health.AtlasCompatible || !health.CapabilitiesOK {
		t.Fatalf("live CLI is not an Atlas-compatible probe: %#v", health)
	}

	res, err := openspec.RunInit(context.Background(), nil, exe, root, "none")
	if err != nil {
		t.Fatalf("live init: %v stdout=%s stderr=%s", err, res.Stdout, res.Stderr)
	}
	if st, reason := openspec.ClassifyProject(root); st != openspec.ProjectReady {
		t.Fatalf("live classify = %s (%s)", st, reason)
	}
	if health.Installed == testharness.ContractOpenSpecVersion && strings.Contains(exe, filepath.Clean(os.TempDir())) {
		t.Fatalf("live executable looks disposable: %s", exe)
	}
}
