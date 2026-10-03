package app_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/app"
	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

func TestApp_RunVersion(t *testing.T) {
	prev := cli.RunTUI
	cli.RunTUI = func(tui.Options) error {
		t.Fatal("version must not launch TUI")
		return nil
	}
	defer func() { cli.RunTUI = prev }()

	var stdout, stderr bytes.Buffer
	application := &app.App{Stdout: &stdout, Stderr: &stderr}
	if err := application.Run([]string{"--version"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	if got != version.Version {
		t.Fatalf("got %q, want %q", got, version.Version)
	}
}

func TestApp_RunInitLaunchesTUI(t *testing.T) {
	var launched tui.Route
	prev := cli.RunTUI
	cli.RunTUI = func(opts tui.Options) error {
		launched = opts.Route
		return nil
	}
	defer func() { cli.RunTUI = prev }()

	var stdout bytes.Buffer
	application := &app.App{Stdout: &stdout}
	if err := application.Run([]string{"init"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if launched != tui.RouteInitPlan {
		t.Fatalf("route = %v, want init", launched)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected no console report, got %q", stdout.String())
	}
}
