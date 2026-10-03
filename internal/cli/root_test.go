package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

func TestResolve_Routes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		args    []string
		mode    cli.Mode
		route   tui.Route
		unknown string
	}{
		{nil, cli.ModeTUI, tui.DefaultRoute, ""},
		{[]string{}, cli.ModeTUI, tui.DefaultRoute, ""},
		{[]string{"help"}, cli.ModeTUI, tui.RouteHelp, ""},
		{[]string{"--help"}, cli.ModeTUI, tui.RouteHelp, ""},
		{[]string{"-h"}, cli.ModeTUI, tui.RouteHelp, ""},
		{[]string{"init"}, cli.ModeTUI, tui.RouteInitPlan, ""},
		{[]string{"init", "--dry-run"}, cli.ModeTUI, tui.RouteInitPlan, ""},
		{[]string{"status"}, cli.ModeTUI, tui.RouteStatus, ""},
		{[]string{"doctor"}, cli.ModeTUI, tui.RouteDoctor, ""},
		{[]string{"start"}, cli.ModeTUI, tui.RouteError, "start"},
		{[]string{"change"}, cli.ModeTUI, tui.RouteError, "change"},
		{[]string{"change", "new"}, cli.ModeTUI, tui.RouteError, "change new"},
		{[]string{"--version"}, cli.ModeVersion, tui.DefaultRoute, ""},
	}

	for _, tc := range cases {
		action := cli.Resolve(tc.args)
		if action.Mode != tc.mode {
			t.Fatalf("args %v: mode = %v, want %v", tc.args, action.Mode, tc.mode)
		}
		if tc.mode == cli.ModeTUI && action.Route != tc.route {
			t.Fatalf("args %v: route = %v, want %v", tc.args, action.Route, tc.route)
		}
		if action.UnknownCommand != tc.unknown {
			t.Fatalf("args %v: unknown = %q, want %q", tc.args, action.UnknownCommand, tc.unknown)
		}
	}
}

func TestExecute_VersionOnlyConsoleOutput(t *testing.T) {
	prev := cli.RunTUI
	cli.RunTUI = func(tui.Options) error {
		t.Fatal("TUI should not launch for --version")
		return nil
	}
	defer func() { cli.RunTUI = prev }()

	var stdout, stderr bytes.Buffer
	if err := cli.Execute(&stdout, &stderr, []string{"--version"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := strings.TrimSpace(stdout.String())
	if got != version.Version {
		t.Fatalf("got %q, want %q", got, version.Version)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
}

func TestExecute_LaunchesTUIRoutes(t *testing.T) {
	cases := []struct {
		args    []string
		route   tui.Route
		unknown string
	}{
		{nil, tui.DefaultRoute, ""},
		{[]string{"help"}, tui.RouteHelp, ""},
		{[]string{"--help"}, tui.RouteHelp, ""},
		{[]string{"-h"}, tui.RouteHelp, ""},
		{[]string{"init"}, tui.RouteInitPlan, ""},
		{[]string{"init", "--dry-run"}, tui.RouteInitPlan, ""},
		{[]string{"status"}, tui.RouteStatus, ""},
		{[]string{"doctor"}, tui.RouteDoctor, ""},
		{[]string{"start"}, tui.RouteError, "start"},
		{[]string{"change"}, tui.RouteError, "change"},
		{[]string{"change", "new"}, tui.RouteError, "change new"},
	}

	for _, tc := range cases {
		var launched tui.Options
		prev := cli.RunTUI
		cli.RunTUI = func(opts tui.Options) error {
			launched = opts
			return nil
		}

		var stdout bytes.Buffer
		err := cli.Execute(&stdout, nil, tc.args)
		cli.RunTUI = prev

		if err != nil {
			t.Fatalf("%v: unexpected error: %v", tc.args, err)
		}
		if stdout.Len() != 0 {
			t.Fatalf("%v: expected no console report, got %q", tc.args, stdout.String())
		}
		if launched.Route != tc.route {
			t.Fatalf("%v: route = %v, want %v", tc.args, launched.Route, tc.route)
		}
		if launched.UnknownCommand != tc.unknown {
			t.Fatalf("%v: unknown = %q, want %q", tc.args, launched.UnknownCommand, tc.unknown)
		}
	}
}

func TestExecute_NoLongConsoleReports(t *testing.T) {
	prev := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { return nil }
	defer func() { cli.RunTUI = prev }()

	for _, args := range [][]string{
		{"init"},
		{"status"},
		{"doctor"},
		{"help"},
	} {
		var stdout bytes.Buffer
		if err := cli.Execute(&stdout, nil, args); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		out := stdout.String()
		for _, banned := range []string{
			"Atlas Init Plan",
			"Atlas Status",
			"Atlas Doctor",
			"Atlas Help",
			"project initialization is not implemented yet",
			"status inspection is not implemented yet",
			"diagnostics are not implemented yet",
		} {
			if strings.Contains(out, banned) {
				t.Fatalf("%v leaked console content %q: %q", args, banned, out)
			}
		}
	}
}
