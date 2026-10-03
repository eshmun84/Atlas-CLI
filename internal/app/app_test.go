package app_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/app"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

func TestApp_RunVersion(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	application := &app.App{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	if err := application.Run([]string{"--version"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	if got != version.Version {
		t.Fatalf("got version %q, want %q", got, version.Version)
	}
}

func TestApp_RunInitPlaceholder(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	application := &app.App{Stdout: &stdout}

	if err := application.Run([]string{"init"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got := strings.TrimSpace(stdout.String())
	want := "project initialization is not implemented yet"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
