package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
	"gopkg.in/yaml.v3"
)

func TestResolve_HooksCursor(t *testing.T) {
	action := cli.Resolve([]string{"hooks", "cursor", "--json"})
	if action.Mode != cli.ModeHooks || action.Hooks.Name != "cursor" || !action.Hooks.JSON {
		t.Fatalf("%#v", action)
	}
}

// stubDenyAction is a blocking action that always denies tool execution.
type stubDenyAction struct{}

func (stubDenyAction) ID() string { return "test-deny" }
func (stubDenyAction) Events() []hooks.EventType {
	return []hooks.EventType{hooks.EventBeforeToolExecution}
}
func (stubDenyAction) Mode() hooks.ExecutionMode { return hooks.ModeBlocking }
func (stubDenyAction) Run(context.Context, hooks.Event) (hooks.Result, error) {
	return hooks.Result{Outcome: hooks.OutcomeDenied, DenyContinuation: true}, nil
}

func TestExecute_HooksCursor_PreToolUseDeny(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("TUI"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	root := t.TempDir()
	writeAtlasProject(t, root, "hooks-cursor-deny")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(wd) }()

	reg, err := hooks.NewRegistry([]hooks.Registration{{
		Action:        stubDenyAction{},
		FailurePolicy: hooks.FailAbort,
	}})
	if err != nil {
		t.Fatal(err)
	}
	prevEng := cli.HooksEngine
	cli.HooksEngine = func() hooksengine.Engine {
		return hooksengine.Engine{Registry: reg, Env: hooksengine.Env{Root: root}}
	}
	defer func() { cli.HooksEngine = prevEng }()

	prevIn := cli.HooksInput
	cli.HooksInput = func() io.Reader {
		return strings.NewReader(`{"hook_event_name":"preToolUse","tool_name":"Shell","tool_use_id":"t1","conversation_id":"c1"}`)
	}
	defer func() { cli.HooksInput = prevIn }()

	var stdout bytes.Buffer
	if err := cli.Execute(&stdout, bytes.NewBuffer(nil), []string{"hooks", "cursor", "--json"}); err != nil {
		t.Fatalf("cursor command must exit 0 on deny-via-json: %v out=%s", err, stdout.String())
	}
	var body map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["permission"] != "deny" {
		t.Fatalf("body=%v out=%s", body, stdout.String())
	}
}

func TestExecute_HooksCursor_FailOpenUnsupported(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("TUI"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	prevIn := cli.HooksInput
	cli.HooksInput = func() io.Reader {
		return strings.NewReader(`{"hook_event_name":"stop","conversation_id":"c1"}`)
	}
	defer func() { cli.HooksInput = prevIn }()

	prevEng := cli.HooksEngine
	cli.HooksEngine = func() hooksengine.Engine {
		t.Fatal("engine must not run for unsupported native event")
		return hooksengine.Engine{}
	}
	defer func() { cli.HooksEngine = prevEng }()

	var stdout bytes.Buffer
	if err := cli.Execute(&stdout, bytes.NewBuffer(nil), []string{"hooks", "cursor", "--json"}); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(stdout.String()) != "{}" {
		t.Fatalf("out=%q", stdout.String())
	}
}

func writeAtlasProject(t *testing.T, root, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig(name)
	data, err := yaml.Marshal(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(config.FileConfig)), data, 0o644); err != nil {
		t.Fatal(err)
	}
}
