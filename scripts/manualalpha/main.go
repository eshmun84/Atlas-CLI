// Command manualalpha runs the Slice 22 Alpha manual smoke against a project
// root and ATLAS_HOME provided via environment variables. It never writes into
// the Atlas repository root.
//
//	ATLAS_SMOKE_MANUAL_ROOT=/tmp/proj ATLAS_HOME=/tmp/atlas-home-test-22 \
//	  go run ./scripts/manualalpha
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "manualalpha: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("manualalpha: ok")
}

func run() error {
	root := strings.TrimSpace(os.Getenv("ATLAS_SMOKE_MANUAL_ROOT"))
	if root == "" {
		return fmt.Errorf("ATLAS_SMOKE_MANUAL_ROOT required")
	}
	if strings.TrimSpace(os.Getenv("ATLAS_HOME")) == "" {
		return fmt.Errorf("ATLAS_HOME required")
	}

	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# owned\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("bin/\n"), 0o644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "agents"), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "agents", "external-helper.md"), []byte("keep\n"), 0o644); err != nil {
		return err
	}

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   "alpha-22",
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	})
	for _, adapter := range []string{"cursor", "opencode"} {
		if !draft.ToggleMulti("adapters.selected", adapter) {
			return fmt.Errorf("toggle adapter %s", adapter)
		}
	}
	now := fixedNow(2026, 10, 6, 20, 0, 0)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   now,
	}); err != nil {
		return err
	}

	for _, rel := range []string{
		"AGENTS.md",
		".cursor/rules/atlas.mdc",
		".opencode/atlas.md",
		".atlas/agent-registry.md",
		".atlas/runtime-manifest.yaml",
		".atlas/assets.lock.yaml",
		".atlas/contracts/sdd-openspec.md",
	} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			return fmt.Errorf("missing %s: %w", rel, err)
		}
	}
	if len(assets.AtlasAgentFilenames) != 14 {
		return fmt.Errorf("expected 14 Atlas agents, got %d", len(assets.AtlasAgentFilenames))
	}
	for _, path := range config.AtlasAgentRuntimePaths([]string{"cursor", "opencode"}) {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			return fmt.Errorf("missing agent %s: %w", path, err)
		}
	}
	fmt.Println("PASS materialization + 14 agents x2")

	disc, err := workspace.Discover(root)
	if err != nil {
		return err
	}
	plan := atlascontext.BuildUpdatePlan(root, true, disc.Runtime.State, "slice22 manual smoke")
	applied, err := atlascontext.ApplyUpdate(root, plan.Signature(), disc.Runtime.State, "slice22 manual smoke", now)
	if err != nil {
		return err
	}
	for _, path := range []string{applied.CapsulePath, applied.PackPath, atlascontext.IndexPath(plan.HomePath, plan.ProjectID)} {
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("missing context artifact %s: %w", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "context")); !os.IsNotExist(err) {
		return fmt.Errorf("context payload written into product repo")
	}
	fmt.Println("PASS context economy")

	before := walk(root)
	disc, err = workspace.Discover(root)
	if err != nil {
		return err
	}
	_ = screens.Status(disc)
	_ = doctor.Evaluate(disc)
	after := walk(root)
	if len(before) != len(after) {
		return fmt.Errorf("status/doctor mutated tree size")
	}
	for path, content := range before {
		if after[path] != content {
			return fmt.Errorf("status/doctor mutated %s", path)
		}
	}
	fmt.Println("PASS status/doctor read-only")

	capsuleBefore, err := os.ReadFile(applied.CapsulePath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "agents", "atlas-worker.md"), []byte("drift\n"), 0o644); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(root, ".atlas", "contracts", "sdd-openspec.md")); err != nil {
		return err
	}
	disc, err = workspace.Discover(root)
	if err != nil {
		return err
	}
	repair := workspace.BuildRuntimeRepairPlan(root, disc.Runtime)
	if !repair.NeedsApply() {
		return fmt.Errorf("expected repair for drift")
	}
	if _, err := workspace.ApplyRuntimeRepair(root, repair.Signature(), now); err != nil {
		return err
	}

	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || string(readme) != "# owned\n" {
		return fmt.Errorf("README mutated: %q err=%v", readme, err)
	}
	ext, err := os.ReadFile(filepath.Join(root, ".cursor", "agents", "external-helper.md"))
	if err != nil || string(ext) != "keep\n" {
		return fmt.Errorf("external agent mutated: %q err=%v", ext, err)
	}
	capsuleAfter, err := os.ReadFile(applied.CapsulePath)
	if err != nil {
		return err
	}
	if string(capsuleAfter) != string(capsuleBefore) {
		return fmt.Errorf("repair mutated context capsule")
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas", "contracts", "sdd-openspec.md")); err != nil {
		return fmt.Errorf("contract not restored: %w", err)
	}
	want, err := config.RenderAtlasAgent("atlas-worker.md")
	if err != nil {
		return err
	}
	got, err := os.ReadFile(filepath.Join(root, ".cursor", "agents", "atlas-worker.md"))
	if err != nil || string(got) != want {
		return fmt.Errorf("worker not restored")
	}
	fmt.Println("PASS repair + preservation + context intact")

	homeDir := os.Getenv("ATLAS_HOME")
	if _, err := os.Stat(filepath.Join(homeDir, "state", "home.yaml")); err != nil {
		return fmt.Errorf("expected Home writes under ATLAS_HOME: %w", err)
	}
	fmt.Println("PASS ATLAS_HOME used")
	return nil
}

func walk(root string) map[string]string {
	out := map[string]string{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			out[filepath.ToSlash(rel)+"/"] = "dir"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out
}

func fixedNow(y int, m time.Month, d, hh, mm, ss int) func() time.Time {
	t := time.Date(y, m, d, hh, mm, ss, 0, time.UTC)
	return func() time.Time { return t }
}
