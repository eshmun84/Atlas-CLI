// Physical disposable Configure + OpenSpec tool-sync dogfood (DOGFOOD-020).
// Usage: go run ./scripts/dogfoodconfiguretx
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
}

func has(root, rel string) bool {
	_, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
}

func main() {
	homeDir, err := os.MkdirTemp("", "atlas-d3-home-*")
	must(err)
	defer func() { _ = os.RemoveAll(homeDir) }()
	must(os.Setenv("ATLAS_HOME", homeDir))
	_, err = home.EnsureAndMirror(time.Now().UTC())
	must(err)

	root, err := os.MkdirTemp("", "atlas-d3-proj-*")
	must(err)
	defer func() { _ = os.RemoveAll(root) }()
	fmt.Println("HOME", homeDir)
	fmt.Println("ROOT", root)

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "d3-physical", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
	})
	if !draft.SetValue("adapters.selected", "cursor") {
		must(fmt.Errorf("set adapters"))
	}
	fixed := time.Date(2026, 10, 15, 21, 0, 0, 0, time.UTC)
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	})
	must(err)

	doc, err := config.LoadProjectDocumentAt(root)
	must(err)
	pin := strings.TrimSpace(doc.Governance.OpenSpecVersion)
	fmt.Println("PIN", pin)
	st, reason := openspec.ClassifyProject(root)
	fmt.Println("OPENSPEC", st, reason)
	if st != openspec.ProjectReady {
		must(fmt.Errorf("not ready: %s", reason))
	}
	fmt.Println("BEFORE cursor opsx", has(root, ".cursor/commands/opsx-propose.md"))
	fmt.Println("BEFORE opencode opsx", has(root, ".opencode/commands/opsx-propose.md"))
	if has(root, ".opencode/commands/opsx-propose.md") {
		must(fmt.Errorf("opencode should be absent before configure"))
	}

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "d3-physical", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, OpenCodeDetected: true,
		ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	if !cfg.SetValue("adapters.selected", "cursor,opencode") {
		must(fmt.Errorf("set configure adapters"))
	}
	_, err = config.PersistConfigure(config.ApplyInput{
		Root: root, Draft: cfg, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed.Add(time.Minute) },
	})
	must(err)

	doc2, err := config.LoadProjectDocumentAt(root)
	must(err)
	fmt.Println("AFTER pin", doc2.Governance.OpenSpecVersion)
	fmt.Println("AFTER adapters", doc2.Adapters.Selected)
	fmt.Println("AFTER cursor opsx", has(root, ".cursor/commands/opsx-propose.md"))
	fmt.Println("AFTER opencode opsx", has(root, ".opencode/commands/opsx-propose.md"))
	if doc2.Governance.OpenSpecVersion != pin {
		must(fmt.Errorf("pin changed: %q -> %q", pin, doc2.Governance.OpenSpecVersion))
	}
	if !has(root, ".cursor/commands/opsx-propose.md") || !has(root, ".opencode/commands/opsx-propose.md") {
		must(fmt.Errorf("expected both adapter projections"))
	}
	if missing := openspec.MissingToolProjections(root, []string{"cursor", "opencode"}); len(missing) > 0 {
		must(fmt.Errorf("missing %s", strings.Join(missing, ",")))
	}

	// Post-provider Atlas failure + rollback is covered by
	// TestPersistConfigure_ProviderSuccessLaterAtlasFailure_RollsBackProvider
	// (deterministic injected seam). This script proves real OpenSpec 1.14.x
	// tool sync under Configure succeeds transactionally.
	fmt.Println("PHYSICAL_OK")
}
