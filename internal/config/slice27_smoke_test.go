package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/initplan"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

// Slice 27 Configure/Context/MCP/docs smoke (temporary ATLAS_HOME). Run with:
//
//	go test ./internal/config -run TestSlice27ConfigureContextMCPDocsSmoke -count=1 -v
func TestSlice27ConfigureContextMCPDocsSmoke(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("proj\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	pass := func(msg string) { t.Log("PASS", msg) }

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:         "smoke27",
		ProjectMode:         config.ModeExisting,
		ToolCursorAvailable: true,
	})
	if !draft.ToggleMulti("adapters.selected", "cursor") {
		t.Fatal("select cursor")
	}
	if _, err := config.ApplyConfig(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()}); err != nil {
		t.Fatalf("init apply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); err != nil {
		t.Fatal("init missing AGENTS.md")
	}
	pass("Init materializes runtime files")

	agentsBefore, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	cursorBefore, err := os.ReadFile(filepath.Join(root, ".cursor/rules/atlas.mdc"))
	if err != nil {
		t.Fatal(err)
	}

	homeSnap := mustWalk(t, homeDir)
	atlasSnap := mustWalk(t, filepath.Join(root, ".atlas"))
	disc, err := inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	_ = doctor.Evaluate(disc)
	if !walkEqual(homeSnap, mustWalk(t, homeDir)) || !walkEqual(atlasSnap, mustWalk(t, filepath.Join(root, ".atlas"))) {
		t.Fatal("Status/Doctor mutated state after Init")
	}
	pass("Status/Doctor remain read-only after Init")

	cfgDraft := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName:           "smoke27",
		ProjectMode:           config.ModeExisting,
		ToolCursorAvailable:   true,
		ToolOpenCodeAvailable: true,
	})
	_ = cfgDraft.ToggleMulti("adapters.selected", "cursor")
	_ = cfgDraft.ToggleMulti("adapters.selected", "opencode")
	res, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfgDraft, MCP: config.EmptyMCPDraft()})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Configuration changes are saved to .atlas/config.yaml.",
		"Runtime files are not repaired automatically.",
		"Runtime Repair",
	} {
		if !strings.Contains(res.Notice, want) {
			t.Fatalf("adapter configure notice missing %q: %q", want, res.Notice)
		}
	}
	if !res.Impact.RuntimeRepairNeeded {
		t.Fatal("expected RuntimeRepairNeeded")
	}
	agentsAfter, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	cursorAfter, _ := os.ReadFile(filepath.Join(root, ".cursor/rules/atlas.mdc"))
	if string(agentsAfter) != string(agentsBefore) || string(cursorAfter) != string(cursorBefore) {
		t.Fatal("Configure silently changed runtime files")
	}
	if _, err := os.Stat(filepath.Join(root, ".opencode/atlas.md")); !os.IsNotExist(err) {
		t.Fatal("Configure silently materialized OpenCode")
	}
	pass("Configure adapter change: config+MCP reconcile + Runtime Repair recommended; non-MCP runtime untouched")

	mcp := config.EmptyMCPDraft()
	mcp.EnableBuiltin(config.MCPBuiltinFilesystem)
	mcp.EnableBuiltin(config.MCPBuiltinGitHub)
	agentsBeforeMCP, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	cursorBeforeMCP, _ := os.ReadFile(filepath.Join(root, ".cursor/rules/atlas.mdc"))
	res, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfgDraft, MCP: mcp})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Notice, "MCP selections were reconciled") {
		t.Fatalf("missing MCP projection notice: %q", res.Notice)
	}
	doc, err := config.LoadProjectDocumentAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.MCP.Builtins.Filesystem.Enabled || !doc.MCP.Builtins.GitHub.Enabled {
		t.Fatalf("MCP prefs not recorded: %#v", doc.MCP.Builtins)
	}
	if _, err := os.Stat(filepath.Join(root, ".cursor/mcp.json")); err != nil {
		t.Fatalf("expected Cursor MCP projection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "opencode.json")); err != nil {
		t.Fatalf("expected OpenCode MCP projection: %v", err)
	}
	agentsAfterMCP, _ := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	cursorAfterMCP, _ := os.ReadFile(filepath.Join(root, ".cursor/rules/atlas.mdc"))
	if string(agentsAfterMCP) != string(agentsBeforeMCP) || string(cursorAfterMCP) != string(cursorBeforeMCP) {
		t.Fatal("Configure MCP rematerialized non-MCP runtime")
	}
	pass("Configure MCP: projections reconciled; non-MCP runtime untouched")

	plan := initplan.BuildReview(initplan.ReviewInput{Draft: draft, MCP: mcp, Root: root})
	var warnings string
	for _, w := range plan.Warnings {
		warnings += w.Message + "\n"
	}
	if !strings.Contains(warnings, "Context Economy v0") ||
		!strings.Contains(warnings, "CodeGraph is an optional") ||
		!strings.Contains(warnings, "Atlas Context Graph is NOT IMPLEMENTED") {
		t.Fatalf("review Context honesty missing: %s", warnings)
	}
	pass("Context Economy / CodeGraph / Context Graph language is honest")

	doc, _ = config.LoadProjectDocumentAt(root)
	if doc.Project.DocsScaffold {
		t.Fatal("docs scaffold unexpectedly on")
	}
	if _, err := os.Stat(filepath.Join(root, config.FileProjectDocsREADME)); !os.IsNotExist(err) {
		t.Fatal("docs scaffold created while off")
	}
	pass("Project docs scaffold off by default")

	if !cfgDraft.SetValue("project.docs_scaffold", "true") {
		t.Fatal("enable docs scaffold")
	}
	res, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfgDraft, MCP: mcp})
	if err != nil {
		t.Fatal(err)
	}
	docsPath := filepath.Join(root, config.FileProjectDocsREADME)
	body, err := os.ReadFile(docsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "project-owned") {
		t.Fatalf("scaffold content: %s", body)
	}
	if !strings.Contains(res.Notice, "Created project docs scaffold") {
		t.Fatalf("missing created notice: %q", res.Notice)
	}
	pass("Optional docs scaffold creates docs/atlas/README.md when selected")

	custom := "keep my architecture notes\n"
	if err := os.WriteFile(docsPath, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(root, "docs", "existing.md")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, []byte("pre-existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfgDraft, MCP: mcp})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(docsPath)
	if string(got) != custom {
		t.Fatal("Configure overwrote existing docs/atlas/README.md")
	}
	gotOther, _ := os.ReadFile(other)
	if string(gotOther) != "pre-existing\n" {
		t.Fatal("Configure touched unrelated docs/")
	}
	if !strings.Contains(res.Notice, "left untouched") {
		t.Fatalf("missing skipped notice: %q", res.Notice)
	}
	pass("Existing docs/ not overwritten by Configure")

	disc, err = inspect.Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	rplan := runtime.BuildRuntimeRepairPlan(root, disc.Runtime)
	if _, err := runtime.ApplyRuntimeRepair(root, rplan.Signature(), func() time.Time { return time.Now().UTC() }); err != nil {
		t.Fatalf("runtime repair: %v", err)
	}
	got, _ = os.ReadFile(docsPath)
	if string(got) != custom {
		t.Fatal("Runtime Repair overwrote project docs")
	}
	gotOther, _ = os.ReadFile(other)
	if string(gotOther) != "pre-existing\n" {
		t.Fatal("Runtime Repair touched unrelated docs/")
	}
	pass("Runtime Repair preserves developer-owned docs")

	homePath, err := home.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	pid, err := home.ProjectID(root, "smoke27")
	if err != nil {
		t.Fatal(err)
	}
	if !home.ProjectDataPresent(homePath, pid) {
		if err := os.MkdirAll(filepath.Join(homePath, "projects", pid, "context"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	resetDraft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:         "smoke27",
		ProjectMode:         config.ModeExisting,
		ToolCursorAvailable: true,
	})
	_ = resetDraft.ToggleMulti("adapters.selected", "cursor")
	_, err = config.ApplyConfig(config.ApplyInput{Root: root, Draft: resetDraft, MCP: config.EmptyMCPDraft(), AcceptHomeReset: false})
	if err == nil || !strings.Contains(err.Error(), "reset acceptance") {
		t.Fatalf("expected Home reset gate, got %v", err)
	}
	rplan2 := initplan.BuildReview(initplan.ReviewInput{Draft: resetDraft, MCP: config.EmptyMCPDraft(), Root: root})
	if !rplan2.HomeDataDetected {
		t.Fatal("review should detect Home data")
	}
	if rplan2.AcceptHomeReset {
		t.Fatal("AcceptHomeReset must default false")
	}
	pass("Init Home reset gate from Slice 26 still requires explicit acceptance")

	homeSnap = mustWalk(t, homeDir)
	docsBefore, _ := os.ReadFile(docsPath)
	disc, _ = inspect.Inspect(root)
	_ = doctor.Evaluate(disc)
	if !walkEqual(homeSnap, mustWalk(t, homeDir)) {
		t.Fatal("Status/Doctor mutated home after Configure")
	}
	docsAfter, _ := os.ReadFile(docsPath)
	if string(docsAfter) != string(docsBefore) {
		t.Fatal("Status/Doctor mutated docs")
	}
	pass("Status/Doctor remain read-only after Configure")

	t.Log("ALL MANUAL SMOKE CHECKS PASSED")
	fmt.Println("ALL MANUAL SMOKE CHECKS PASSED")
}

func mustWalk(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		if info.IsDir() {
			out[rel+"/"] = "dir"
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(b)
		return nil
	})
	return out
}

func walkEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
