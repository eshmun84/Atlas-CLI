package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/hooks"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
)

func TestResolve_HooksOpenCode(t *testing.T) {
	action := cli.Resolve([]string{"hooks", "opencode", "--json"})
	if action.Mode != cli.ModeHooks || action.Hooks.Name != "opencode" || !action.Hooks.JSON {
		t.Fatalf("%#v", action)
	}
}

// TestExecute_HooksOpenCode_ToolDeny proves the production deny chain using a
// test-only Registry injected via cli.HooksEngine (never DefaultRegistry):
// Hooks Engine OutcomeDenied → TranslateResponse decision=deny → CLI JSON.
//
// Interactive OpenCode re-validation (optional, not required for this suite):
// build a local throwaway atlas that assigns HooksEngine the same injected
// Registry pattern as this test; do not commit env backdoors or production Actions.
func TestExecute_HooksOpenCode_ToolDeny(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("TUI"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	root := t.TempDir()
	writeAtlasProject(t, root, "hooks-oc-deny")
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
		return strings.NewReader(fmt.Sprintf(
			`{"hook_event_name":"tool.execute.before","tool_name":"bash","tool_call_id":"c1","session_id":"s1","session_directory":%q}`,
			root,
		))
	}
	defer func() { cli.HooksInput = prevIn }()

	var stdout bytes.Buffer
	if err := cli.Execute(&stdout, bytes.NewBuffer(nil), []string{"hooks", "opencode", "--json"}); err != nil {
		t.Fatalf("opencode command must exit 0 on deny-via-json: %v out=%s", err, stdout.String())
	}
	var body map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["decision"] != "deny" || body["status"] != "denied" {
		t.Fatalf("body=%v", body)
	}
	if body["message"] != "blocked by atlas hooks" {
		t.Fatalf("message=%v", body["message"])
	}
	enc := stdout.String()
	if strings.Contains(enc, "bash") || strings.Contains(enc, "secret") {
		t.Fatalf("leaked sensitive: %s", enc)
	}
}

func TestExecute_HooksOpenCode_FailOpenUnsupported(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("TUI"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	prevIn := cli.HooksInput
	cli.HooksInput = func() io.Reader {
		return strings.NewReader(`{"hook_event_name":"session.idle","session_id":"s1"}`)
	}
	defer func() { cli.HooksInput = prevIn }()

	prevEng := cli.HooksEngine
	cli.HooksEngine = func() hooksengine.Engine {
		t.Fatal("engine must not run for unsupported native event")
		return hooksengine.Engine{}
	}
	defer func() { cli.HooksEngine = prevEng }()

	var stdout bytes.Buffer
	if err := cli.Execute(&stdout, bytes.NewBuffer(nil), []string{"hooks", "opencode", "--json"}); err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["decision"] != "allow" {
		t.Fatalf("out=%s", stdout.String())
	}
}

func TestExecute_HooksOpenCode_ObservationalPrompt(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error { t.Fatal("TUI"); return nil }
	defer func() { cli.RunTUI = prevTUI }()

	root := t.TempDir()
	writeAtlasProject(t, root, "hooks-oc-prompt")
	wd, _ := os.Getwd()
	_ = os.Chdir(root)
	defer func() { _ = os.Chdir(wd) }()

	var sawType hooks.EventType
	reg, err := hooks.NewRegistry([]hooks.Registration{{
		Action: stubCaptureAction{fn: func(_ context.Context, ev hooks.Event) (hooks.Result, error) {
			sawType = ev.Type
			return hooks.Result{Outcome: hooks.OutcomeOK}, nil
		}},
		FailurePolicy: hooks.FailContinue,
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
		return strings.NewReader(fmt.Sprintf(
			`{"hook_event_name":"session.prompt","session_id":"s9","session_directory":%q,"prompt":"DO-NOT-LEAK"}`,
			root,
		))
	}
	defer func() { cli.HooksInput = prevIn }()

	var stdout bytes.Buffer
	if err := cli.Execute(&stdout, bytes.NewBuffer(nil), []string{"hooks", "opencode", "--json"}); err != nil {
		t.Fatal(err)
	}
	if sawType != hooks.EventTaskStart {
		t.Fatalf("saw=%q", sawType)
	}
	if strings.Contains(stdout.String(), "DO-NOT-LEAK") {
		t.Fatal("prompt leaked")
	}
	_ = filepath.Separator
}

type stubCaptureAction struct {
	fn func(context.Context, hooks.Event) (hooks.Result, error)
}

func (a stubCaptureAction) ID() string { return "test-capture" }
func (a stubCaptureAction) Events() []hooks.EventType {
	return []hooks.EventType{hooks.EventTaskStart}
}
func (a stubCaptureAction) Mode() hooks.ExecutionMode { return hooks.ModeNonBlocking }
func (a stubCaptureAction) Run(ctx context.Context, ev hooks.Event) (hooks.Result, error) {
	return a.fn(ctx, ev)
}
