package config_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

type configureOSRunner struct {
	calls [][]string
	fn    func(args []string, dir string) (openspec.RunResult, error)
}

func (r *configureOSRunner) Run(_ context.Context, _ string, args []string, dir string) (openspec.RunResult, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	if r.fn != nil {
		return r.fn(args, dir)
	}
	return openspec.RunResult{ExitCode: 0}, nil
}

func writeOpenSpecAdapterMarkers(t *testing.T, root, adapter string) {
	t.Helper()
	markers := map[string][]string{
		"cursor": {
			".cursor/commands/opsx-propose.md",
			".cursor/skills/openspec-propose/SKILL.md",
		},
		"opencode": {
			".opencode/commands/opsx-propose.md",
			".opencode/skills/openspec-propose/SKILL.md",
		},
	}[adapter]
	if len(markers) == 0 {
		t.Fatalf("unknown adapter %q", adapter)
	}
	for _, rel := range markers {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf("# openspec %s marker\n", adapter)
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func initCursorOpenSpecProject(t *testing.T, name string) (root, pin string, homeDir string) {
	t.Helper()
	homeDir = t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	root = t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:         name,
		ProjectMode:         "new",
		DefaultRemote:       "origin",
		CursorDetected:      true,
		ToolCursorAvailable: true,
	})
	if !draft.SetValue("adapters.selected", "cursor") {
		t.Fatal("adapters")
	}
	fixed := time.Date(2026, 10, 15, 20, 0, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return fixed },
	}); err != nil {
		t.Fatalf("init apply: %v", err)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	pin = strings.TrimSpace(doc.Governance.OpenSpecVersion)
	if pin == "" {
		t.Fatal("expected OpenSpec pin after Init")
	}
	if st, reason := openspec.ClassifyProject(root); st != openspec.ProjectReady {
		t.Fatalf("openspec not ready after init: %s (%s)", st, reason)
	}
	if missing := openspec.MissingToolProjections(root, []string{"cursor"}); len(missing) > 0 {
		t.Fatalf("cursor projections missing after init: %v", missing)
	}
	return root, pin, homeDir
}

func configureDraftCursorOpenCode(t *testing.T, name string) config.ConfigDraft {
	t.Helper()
	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName:           name,
		ProjectMode:           "new",
		DefaultRemote:         "origin",
		CursorDetected:        true,
		OpenCodeDetected:      true,
		ToolCursorAvailable:   true,
		ToolOpenCodeAvailable: true,
	})
	if !draft.SetValue("adapters.selected", "cursor,opencode") {
		t.Fatal("adapters")
	}
	return draft
}

func providerFingerprint(t *testing.T, root string) string {
	t.Helper()
	snaps, err := openspec.CaptureProviderFileSnapshots(root)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, s := range snaps {
		fmt.Fprintf(&b, "%s:%x:%o\n", s.Rel, s.Data, s.Mode)
	}
	return b.String()
}

func TestPersistConfigure_NoProviderMutationWhenProjectionsHealthy(t *testing.T) {
	root, pin, _ := initCursorOpenSpecProject(t, "cfg-os-noprov")
	before := providerFingerprint(t, root)

	runner := &configureOSRunner{fn: func([]string, string) (openspec.RunResult, error) {
		t.Fatal("OpenSpec runner must not be invoked when projections are healthy")
		return openspec.RunResult{}, nil
	}}
	config.SetConfigureOpenSpecRunnerForTest(runner)
	t.Cleanup(func() { config.SetConfigureOpenSpecRunnerForTest(nil) })

	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName:         "cfg-os-noprov",
		ProjectMode:         "new",
		DefaultRemote:       "origin",
		CursorDetected:      true,
		ToolCursorAvailable: true,
	})
	if !draft.SetValue("adapters.selected", "cursor") {
		t.Fatal("adapters")
	}
	mcp := config.EmptyMCPDraft()
	mcp.EnableBuiltin(config.MCPBuiltinFilesystem)

	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unexpected openspec calls: %v", runner.calls)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Governance.OpenSpecVersion != pin {
		t.Fatalf("pin changed: got %q want %q", doc.Governance.OpenSpecVersion, pin)
	}
	if after := providerFingerprint(t, root); after != before {
		t.Fatalf("provider footprint changed without mutation\nbefore=%s\nafter=%s", before, after)
	}
}

func TestPersistConfigure_CursorToOpenCode_ProviderSuccess(t *testing.T) {
	root, pin, _ := initCursorOpenSpecProject(t, "cfg-os-success")
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "commands", "opsx-propose.md")); !os.IsNotExist(err) {
		t.Fatal("opencode provider projection must be absent before configure")
	}

	runner := &configureOSRunner{fn: func(args []string, dir string) (openspec.RunResult, error) {
		if len(args) < 1 || args[0] != "init" {
			t.Fatalf("expected init --tools, got %v", args)
		}
		writeOpenSpecAdapterMarkers(t, dir, "cursor")
		writeOpenSpecAdapterMarkers(t, dir, "opencode")
		return openspec.RunResult{ExitCode: 0}, nil
	}}
	config.SetConfigureOpenSpecRunnerForTest(runner)
	t.Cleanup(func() { config.SetConfigureOpenSpecRunnerForTest(nil) })

	draft := configureDraftCursorOpenCode(t, "cfg-os-success")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one openspec call, got %v", runner.calls)
	}
	if missing := openspec.MissingToolProjections(root, []string{"cursor", "opencode"}); len(missing) > 0 {
		t.Fatalf("missing projections: %v", missing)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Governance.OpenSpecVersion != pin {
		t.Fatalf("pin changed on success: got %q want %q", doc.Governance.OpenSpecVersion, pin)
	}
	if got := config.JoinChips(doc.Adapters.Selected); !strings.Contains(got, "cursor") || !strings.Contains(got, "opencode") {
		t.Fatalf("adapters = %q", got)
	}
}

func TestPersistConfigure_ProviderPartialFailure_RollsBackExact(t *testing.T) {
	root, pin, _ := initCursorOpenSpecProject(t, "cfg-os-partial")
	beforeFP := providerFingerprint(t, root)
	beforeCfg, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	runner := &configureOSRunner{fn: func(args []string, dir string) (openspec.RunResult, error) {
		writeOpenSpecAdapterMarkers(t, dir, "opencode")
		// Also mutate an existing cursor provider file.
		cur := filepath.Join(dir, ".cursor", "commands", "opsx-propose.md")
		if err := os.WriteFile(cur, []byte("partial-mutation\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return openspec.RunResult{ExitCode: 2, Stderr: []byte("shim boom")}, fmt.Errorf("openspec: exit 2: %w", openspec.ErrProcessFailed)
	}}
	config.SetConfigureOpenSpecRunnerForTest(runner)
	t.Cleanup(func() { config.SetConfigureOpenSpecRunnerForTest(nil) })

	draft := configureDraftCursorOpenCode(t, "cfg-os-partial")
	_, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()})
	if err == nil {
		t.Fatal("expected configure failure")
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "commands", "opsx-propose.md")); !os.IsNotExist(err) {
		t.Fatalf("opencode projection must be removed on rollback, err=%v", err)
	}
	gotCur, err := os.ReadFile(filepath.Join(root, ".cursor", "commands", "opsx-propose.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotCur) == "partial-mutation\n" {
		t.Fatal("cursor provider file not restored")
	}
	afterCfg, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterCfg) != string(beforeCfg) {
		t.Fatal("config.yaml not restored")
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Governance.OpenSpecVersion != pin {
		t.Fatalf("pin not restored: got %q want %q", doc.Governance.OpenSpecVersion, pin)
	}
	if afterFP := providerFingerprint(t, root); afterFP != beforeFP {
		t.Fatalf("provider fingerprint mismatch after rollback\nbefore=%s\nafter=%s", beforeFP, afterFP)
	}
}

func TestPersistConfigure_ProviderSuccessLaterAtlasFailure_RollsBackProvider(t *testing.T) {
	root, pin, _ := initCursorOpenSpecProject(t, "cfg-os-later")
	beforeFP := providerFingerprint(t, root)
	beforeCfg, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	runner := &configureOSRunner{fn: func(args []string, dir string) (openspec.RunResult, error) {
		writeOpenSpecAdapterMarkers(t, dir, "cursor")
		writeOpenSpecAdapterMarkers(t, dir, "opencode")
		return openspec.RunResult{ExitCode: 0}, nil
	}}
	config.SetConfigureOpenSpecRunnerForTest(runner)
	config.SetAfterOpenSpecConfigureHookForTest(func() error {
		if _, err := os.Lstat(filepath.Join(root, ".opencode", "commands", "opsx-propose.md")); err != nil {
			t.Fatalf("expected opencode projection before atlas failure: %v", err)
		}
		return fmt.Errorf("injected post-openspec atlas failure")
	})
	t.Cleanup(func() {
		config.SetConfigureOpenSpecRunnerForTest(nil)
		config.SetAfterOpenSpecConfigureHookForTest(nil)
	})

	draft := configureDraftCursorOpenCode(t, "cfg-os-later")
	_, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()})
	if err == nil {
		t.Fatal("expected failure")
	}
	if !strings.Contains(err.Error(), "injected post-openspec atlas failure") {
		t.Fatalf("missing cause: %v", err)
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "commands", "opsx-propose.md")); !os.IsNotExist(err) {
		t.Fatalf("opencode projection must be rolled back, err=%v", err)
	}
	afterCfg, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterCfg) != string(beforeCfg) {
		t.Fatal("config not restored")
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Governance.OpenSpecVersion != pin {
		t.Fatalf("pin not restored: %q", doc.Governance.OpenSpecVersion)
	}
	if afterFP := providerFingerprint(t, root); afterFP != beforeFP {
		t.Fatalf("provider fingerprint mismatch\nbefore=%s\nafter=%s", beforeFP, afterFP)
	}
}

func TestPersistConfigure_UnrelatedAtlasChange_PreservesProviderTree(t *testing.T) {
	root, pin, _ := initCursorOpenSpecProject(t, "cfg-os-preserve")
	before := providerFingerprint(t, root)

	runner := &configureOSRunner{fn: func([]string, string) (openspec.RunResult, error) {
		t.Fatal("must not mutate provider for unrelated configure")
		return openspec.RunResult{}, nil
	}}
	config.SetConfigureOpenSpecRunnerForTest(runner)
	t.Cleanup(func() { config.SetConfigureOpenSpecRunnerForTest(nil) })

	draft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName:         "cfg-os-preserve",
		ProjectMode:         "new",
		DefaultRemote:       "origin",
		CursorDetected:      true,
		ToolCursorAvailable: true,
	})
	if !draft.SetValue("adapters.selected", "cursor") {
		t.Fatal("adapters")
	}
	if !draft.SetValue("project.docs_scaffold", "true") {
		// optional; docs scaffold may already be default false
		_ = draft.SetValue("project.docs_scaffold", "true")
	}
	mcp := config.EmptyMCPDraft()
	mcp.EnableBuiltin(config.MCPBuiltinFilesystem)
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: mcp}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unexpected provider calls: %v", runner.calls)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Governance.OpenSpecVersion != pin {
		t.Fatalf("pin changed: %q", doc.Governance.OpenSpecVersion)
	}
	if after := providerFingerprint(t, root); after != before {
		t.Fatalf("provider tree changed\nbefore=%s\nafter=%s", before, after)
	}
}

func TestPersistConfigure_ProviderTimeout_AttemptsRestore(t *testing.T) {
	root, pin, _ := initCursorOpenSpecProject(t, "cfg-os-timeout")
	beforeFP := providerFingerprint(t, root)

	runner := &configureOSRunner{fn: func(args []string, dir string) (openspec.RunResult, error) {
		writeOpenSpecAdapterMarkers(t, dir, "opencode")
		return openspec.RunResult{ExitCode: -1}, fmt.Errorf("openspec: command timed out after 1s: %w", openspec.ErrTimeout)
	}}
	config.SetConfigureOpenSpecRunnerForTest(runner)
	t.Cleanup(func() { config.SetConfigureOpenSpecRunnerForTest(nil) })

	draft := configureDraftCursorOpenCode(t, "cfg-os-timeout")
	_, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()})
	if err == nil {
		t.Fatal("expected timeout failure")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout context: %v", err)
	}
	if !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback after timeout: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "commands", "opsx-propose.md")); !os.IsNotExist(err) {
		t.Fatalf("opencode projection must be restored absent, err=%v", err)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Governance.OpenSpecVersion != pin {
		t.Fatalf("pin changed: %q", doc.Governance.OpenSpecVersion)
	}
	if afterFP := providerFingerprint(t, root); afterFP != beforeFP {
		t.Fatalf("provider fingerprint mismatch\nbefore=%s\nafter=%s", beforeFP, afterFP)
	}
}

func TestPersistConfigure_ProviderMutation_LockHeld(t *testing.T) {
	root, _, homeDir := initCursorOpenSpecProject(t, "cfg-os-lock")
	var sawLock bool
	runner := &configureOSRunner{fn: func(args []string, dir string) (openspec.RunResult, error) {
		// Project mutation lock must still be held while provider mutates.
		_, err := mutatelock.Acquire(mutatelock.Options{HomePath: homeDir, Workspace: root})
		if err == nil {
			t.Fatal("expected lock contention during provider mutation")
		}
		sawLock = true
		writeOpenSpecAdapterMarkers(t, dir, "cursor")
		writeOpenSpecAdapterMarkers(t, dir, "opencode")
		return openspec.RunResult{ExitCode: 0}, nil
	}}
	config.SetConfigureOpenSpecRunnerForTest(runner)
	t.Cleanup(func() { config.SetConfigureOpenSpecRunnerForTest(nil) })

	draft := configureDraftCursorOpenCode(t, "cfg-os-lock")
	if _, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()}); err != nil {
		t.Fatalf("configure: %v", err)
	}
	if !sawLock {
		t.Fatal("lock check did not run")
	}
}

func TestPersistConfigure_ProviderSymlinkRoot_FailClosed(t *testing.T) {
	root, _, _ := initCursorOpenSpecProject(t, "cfg-os-symlink")
	// Replace openspec/ with a symlink to force fail-closed snapshot.
	osDir := filepath.Join(root, "openspec")
	outside := t.TempDir()
	if err := os.RemoveAll(osDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, osDir); err != nil {
		t.Fatal(err)
	}
	// Force tool sync path: remove cursor markers so NeedsToolUpdate is true,
	// but probe may classify malformed — still exercise snapshot fail-closed
	// when mutation is required. Seed ready markers under a fake layout is hard
	// once openspec is a symlink; instead call Capture directly.
	_, err := openspec.CaptureProviderFileSnapshots(root)
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink refusal, got %v", err)
	}
}

func TestPersistConfigure_RollbackFailure_CompositeError(t *testing.T) {
	root, _, _ := initCursorOpenSpecProject(t, "cfg-os-rbfail")

	runner := &configureOSRunner{fn: func(args []string, dir string) (openspec.RunResult, error) {
		writeOpenSpecAdapterMarkers(t, dir, "opencode")
		return openspec.RunResult{ExitCode: 0}, nil
	}}
	config.SetConfigureOpenSpecRunnerForTest(runner)
	config.SetAfterOpenSpecConfigureHookForTest(func() error {
		// Poison a created provider file so identity-aware rollback cannot remove it.
		path := filepath.Join(root, ".opencode", "commands", "opsx-propose.md")
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		return fmt.Errorf("injected apply failure after provider success")
	})
	t.Cleanup(func() {
		config.SetConfigureOpenSpecRunnerForTest(nil)
		config.SetAfterOpenSpecConfigureHookForTest(nil)
	})

	draft := configureDraftCursorOpenCode(t, "cfg-os-rbfail")
	_, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()})
	if err == nil {
		t.Fatal("expected composite failure")
	}
	msg := err.Error()
	if !strings.Contains(msg, "injected apply failure after provider success") {
		t.Fatalf("missing apply cause: %v", err)
	}
	if !strings.Contains(msg, "rollback failed") {
		t.Fatalf("expected rollback failure: %v", err)
	}
	if !strings.Contains(msg, "manual inspection required") {
		t.Fatalf("expected manual inspection wording: %v", err)
	}
}

func TestPersistConfigure_EmptyPinPreservedOnFailedToolSync(t *testing.T) {
	// Configure does not adopt pins; if previous pin is empty, failed Apply must
	// leave it empty (no half-adoption via config write).
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	// Seed Atlas config without pin but with ready OpenSpec + cursor markers.
	if err := os.MkdirAll(filepath.Join(root, ".atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := []byte(`project:
  name: cfg-os-emptypin
  mode: new
governance:
  workflow: sdd
  spec_engine: openspec
adapters:
  selected: [cursor]
source_control:
  mode: none
  default_remote: origin
  branch_strategy: manual
  governance_files: local_only
memory:
  strategy: sqlite
context:
  graph:
    enabled: false
mcp:
  builtins:
    jira:
      enabled: false
    context7:
      enabled: false
    chrome_devtools:
      enabled: false
  custom: []
`)
	if err := os.WriteFile(filepath.Join(root, ".atlas", "config.yaml"), cfg, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{
		"openspec/specs", "openspec/changes/archive",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "openspec", "config.yaml"), []byte("schema: spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeOpenSpecAdapterMarkers(t, root, "cursor")

	runner := &configureOSRunner{fn: func(args []string, dir string) (openspec.RunResult, error) {
		writeOpenSpecAdapterMarkers(t, dir, "opencode")
		return openspec.RunResult{ExitCode: 0}, nil
	}}
	config.SetConfigureOpenSpecRunnerForTest(runner)
	config.SetAfterOpenSpecConfigureHookForTest(func() error {
		return errors.New("injected failure before commit")
	})
	t.Cleanup(func() {
		config.SetConfigureOpenSpecRunnerForTest(nil)
		config.SetAfterOpenSpecConfigureHookForTest(nil)
	})

	draft := configureDraftCursorOpenCode(t, "cfg-os-emptypin")
	_, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()})
	if err == nil {
		t.Fatal("expected failure")
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(doc.Governance.OpenSpecVersion) != "" {
		t.Fatalf("empty pin must be restored, got %q", doc.Governance.OpenSpecVersion)
	}
	if _, err := os.Lstat(filepath.Join(root, ".opencode", "commands", "opsx-propose.md")); !os.IsNotExist(err) {
		t.Fatalf("opencode projection must be rolled back, err=%v", err)
	}
}
