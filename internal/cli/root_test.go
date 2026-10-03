package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

func TestExecute_RootHelp(t *testing.T) {
	t.Parallel()

	cases := [][]string{
		nil,
		{},
		{"--help"},
		{"-h"},
		{"help"},
	}

	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		err := cli.Execute(&stdout, &stderr, args)
		if err != nil {
			t.Fatalf("args %v: unexpected error: %v", args, err)
		}
		out := stdout.String()
		if !strings.Contains(out, "Usage:") {
			t.Fatalf("args %v: expected usage output, got %q", args, out)
		}
		if !strings.Contains(out, "init") || !strings.Contains(out, "status") || !strings.Contains(out, "doctor") {
			t.Fatalf("args %v: expected command list in help, got %q", args, out)
		}
		if stderr.Len() != 0 {
			t.Fatalf("args %v: expected empty stderr, got %q", args, stderr.String())
		}
	}
}

func TestExecute_Version(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	err := cli.Execute(&stdout, &stderr, []string{"--version"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	if got != version.Version {
		t.Fatalf("got version %q, want %q", got, version.Version)
	}
}

func TestExecute_PlaceholderCommands(t *testing.T) {
	t.Parallel()

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"init"}, "project initialization is not implemented yet"},
		{[]string{"doctor"}, "diagnostics are not implemented yet"},
	}

	for _, tc := range cases {
		var stdout, stderr bytes.Buffer
		err := cli.Execute(&stdout, &stderr, tc.args)
		if err != nil {
			t.Fatalf("%v: unexpected error: %v", tc.args, err)
		}
		got := strings.TrimSpace(stdout.String())
		if got != tc.want {
			t.Fatalf("%v: got %q, want %q", tc.args, got, tc.want)
		}
		if stderr.Len() != 0 {
			t.Fatalf("%v: expected empty stderr, got %q", tc.args, stderr.String())
		}
	}
}

func TestExecute_CommandHelp(t *testing.T) {
	t.Parallel()

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"init", "--help"}, "Initialize Atlas in a project"},
		{[]string{"status", "-h"}, "Show Atlas project status"},
		{[]string{"doctor", "--help"}, "Run Atlas diagnostics"},
	}

	for _, tc := range cases {
		var stdout, stderr bytes.Buffer
		err := cli.Execute(&stdout, &stderr, tc.args)
		if err != nil {
			t.Fatalf("%v: unexpected error: %v", tc.args, err)
		}
		if !strings.Contains(stdout.String(), tc.want) {
			t.Fatalf("%v: expected help containing %q, got %q", tc.args, tc.want, stdout.String())
		}
	}
}

func TestExecute_UnknownCommand(t *testing.T) {
	t.Parallel()

	for _, cmd := range []string{"start", "change"} {
		var stdout, stderr bytes.Buffer
		err := cli.Execute(&stdout, &stderr, []string{cmd})
		if err == nil {
			t.Fatalf("expected error for unknown command %q", cmd)
		}
		if !strings.Contains(err.Error(), "unknown command: "+cmd) {
			t.Fatalf("unexpected error for %q: %v", cmd, err)
		}
	}
}
