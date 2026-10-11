package cli_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/cli"
	"github.com/eshmun84/Atlas-CLI/internal/hooksengine"
	"github.com/eshmun84/Atlas-CLI/internal/tui"
)

func TestResolve_HooksDispatch(t *testing.T) {
	action := cli.Resolve([]string{"hooks", "dispatch", "--json"})
	if action.Mode != cli.ModeHooks || action.Hooks.Name != "dispatch" || !action.Hooks.JSON {
		t.Fatalf("%#v", action)
	}
	action = cli.Resolve([]string{"hooks"})
	if action.Mode != cli.ModeHooks || action.UnknownCommand != "hooks" {
		t.Fatalf("%#v", action)
	}
	action = cli.Resolve([]string{"hooks", "dispatch"})
	if action.Mode != cli.ModeHooks || action.Hooks.Name != "dispatch" || action.Hooks.JSON {
		t.Fatalf("%#v", action)
	}
	action = cli.Resolve([]string{"hooks", "nope", "--json"})
	if action.Mode != cli.ModeHooks || action.UnknownCommand == "" {
		t.Fatalf("%#v", action)
	}
}

func TestExecute_HooksRequiresJSONNoTUI(t *testing.T) {
	prev := cli.RunTUI
	cli.RunTUI = func(tui.Options) error {
		t.Fatal("TUI must not launch for hooks")
		return nil
	}
	defer func() { cli.RunTUI = prev }()

	var stdout, stderr bytes.Buffer
	err := cli.Execute(&stdout, &stderr, []string{"hooks", "dispatch"})
	if err == nil {
		t.Fatal("expected non-zero exit error")
	}
	var resp hooksengine.Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v out=%s", err, stdout.String())
	}
	if resp.Status != hooksengine.StatusInvalidUsage {
		t.Fatalf("status=%q", resp.Status)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr=%q", stderr.String())
	}
}

func TestExecute_HooksInvalidBody(t *testing.T) {
	prevTUI := cli.RunTUI
	cli.RunTUI = func(tui.Options) error {
		t.Fatal("TUI must not launch")
		return nil
	}
	defer func() { cli.RunTUI = prevTUI }()

	prevIn := cli.HooksInput
	cli.HooksInput = func() io.Reader {
		return strings.NewReader(`{"extra":true}`)
	}
	defer func() { cli.HooksInput = prevIn }()

	prevEng := cli.HooksEngine
	cli.HooksEngine = func() hooksengine.Engine {
		t.Fatal("engine must not run on decode failure")
		return hooksengine.Engine{}
	}
	defer func() { cli.HooksEngine = prevEng }()

	var stdout bytes.Buffer
	err := cli.Execute(&stdout, bytes.NewBuffer(nil), []string{"hooks", "dispatch", "--json"})
	if err == nil {
		t.Fatal("expected error")
	}
	var resp hooksengine.Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("json: %v", err)
	}
	if resp.Status != hooksengine.StatusInvalidRequest {
		t.Fatalf("status=%q out=%s", resp.Status, stdout.String())
	}
}
