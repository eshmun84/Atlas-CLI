// Disposable Block D / D6 physical polish dogfood (A–D).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("ALL PHYSICAL D6 POLISH CHECKS PASSED")
}

func run() error {
	repo, err := os.Getwd()
	if err != nil {
		return err
	}
	dog := filepath.Join(os.TempDir(), fmt.Sprintf("atlas-d6-polish-%d", time.Now().UnixNano()))
	homeDir := filepath.Join(dog, "home")
	prefix := filepath.Join(dog, "prefix")
	project := filepath.Join(dog, "project")
	for _, d := range []string{homeDir, prefix, project} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := os.Setenv(home.EnvAtlasHome, homeDir); err != nil {
		return err
	}
	fmt.Println("DOGFOOD_DIR=", dog)

	// D. make install PREFIX=<temp>
	fmt.Println("== D. make install ==")
	if out, err := exec.Command("make", "build").CombinedOutput(); err != nil {
		return fmt.Errorf("make build: %v\n%s", err, out)
	}
	if out, err := exec.Command("make", "install", "PREFIX="+prefix).CombinedOutput(); err != nil {
		return fmt.Errorf("make install: %v\n%s", err, out)
	}
	bin := filepath.Join(prefix, "bin", "atlas")
	ver, err := exec.Command(bin, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("atlas --version: %v", err)
	}
	if !strings.Contains(string(ver), "0.1.0") {
		return fmt.Errorf("version %q", ver)
	}
	fmt.Println("PASS D:", bin)

	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		return err
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "d6-polish", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
		OpenCodeDetected: true, ToolOpenCodeAvailable: true,
	})
	if !draft.SetValue("adapters.selected", "cursor,opencode") {
		return fmt.Errorf("set adapters")
	}
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: project, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 15, 20, 0, 0, 0, time.UTC) },
	}); err != nil {
		return fmt.Errorf("init apply: %w", err)
	}

	before, err := homeFingerprint(homeDir)
	if err != nil {
		return err
	}

	// A. project Repair
	fmt.Println("== A. project Repair Home fingerprint ==")
	if err := os.Remove(filepath.Join(project, "AGENTS.md")); err != nil {
		return err
	}
	insp, err := inspect.Inspect(project)
	if err != nil {
		return err
	}
	plan := runtime.BuildRuntimeRepairPlan(project, insp.Runtime)
	for _, t := range plan.Targets {
		if t.Kind == runtime.RepairKindHome {
			return fmt.Errorf("unexpected Home target in project-only plan")
		}
	}
	res, err := runtime.ApplyRuntimeRepair(project, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 15, 20, 1, 0, 0, time.UTC)
	})
	if err != nil {
		return err
	}
	if res.Blocked || res.Noop {
		return fmt.Errorf("repair result %#v", res)
	}
	if _, err := os.Stat(filepath.Join(project, "AGENTS.md")); err != nil {
		return err
	}
	after, err := homeFingerprint(homeDir)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("Home fingerprint changed on project Repair")
	}
	fmt.Println("PASS A")

	// B. hook missing → Doctor + Repair
	fmt.Println("== B. hook drift ==")
	wrapper := filepath.Join(project, ".cursor", "hooks", "atlas", "dispatch")
	if err := os.Remove(wrapper); err != nil {
		return err
	}
	insp, err = inspect.Inspect(project)
	if err != nil {
		return err
	}
	report := doctor.Evaluate(insp)
	found := false
	for _, c := range report.Checks {
		if c.Name == "hooks projections" {
			found = true
			if c.Severity != doctor.SeverityFail || c.Repairability != doctor.RepairAtlas {
				return fmt.Errorf("hooks doctor %#v", c)
			}
		}
	}
	if !found {
		return fmt.Errorf("missing hooks projections check")
	}
	plan = runtime.BuildRuntimeRepairPlan(project, insp.Runtime)
	hasHook := false
	for _, t := range plan.Targets {
		if t.Kind == runtime.RepairKindHook {
			hasHook = true
		}
	}
	if !hasHook {
		return fmt.Errorf("repair plan missing hook target")
	}
	res, err = runtime.ApplyRuntimeRepair(project, plan.Signature(), func() time.Time {
		return time.Date(2026, 10, 15, 20, 2, 0, 0, time.UTC)
	})
	if err != nil {
		return err
	}
	if _, err := os.Stat(wrapper); err != nil {
		return fmt.Errorf("wrapper not restored: %w", err)
	}
	after, err = homeFingerprint(homeDir)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("Home fingerprint changed on hooks Repair")
	}
	fmt.Println("PASS B")
	_ = repo

	// C. missing OpenSpec tool projection
	fmt.Println("== C. OpenSpec projection missing ==")
	for _, d := range []string{
		filepath.Join(project, "openspec", "specs"),
		filepath.Join(project, "openspec", "changes", "archive"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(project, "openspec", "config.yaml"), []byte("schema: 1\n"), 0o644); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(project, ".cursor", "commands", "opsx-propose.md"))
	_ = os.Remove(filepath.Join(project, ".cursor", "skills", "openspec-propose", "SKILL.md"))

	insp, err = inspect.Inspect(project)
	if err != nil {
		return err
	}
	insp.OpenSpecProvider = openspec.Health{
		State: openspec.StateReady, Installed: "1.0.0",
		RequiredPin: insp.Runtime.Document.Governance.OpenSpecVersion,
		Project:     openspec.ProjectReady,
	}
	insp.OpenSpecProviderProbed = true
	insp.Runtime.DependsOnSDDContract = true
	insp.Runtime.ConfigLoads = true
	insp.Runtime.SDDContractPresent = true
	insp.Runtime.SDDContractMatches = true

	report = doctor.Evaluate(insp)
	found = false
	for _, c := range report.Checks {
		if c.Name == "openspec tool projections" {
			found = true
			if c.Severity != doctor.SeverityFail || c.Repairability != doctor.RepairManual {
				return fmt.Errorf("openspec projections %#v", c)
			}
			if !strings.Contains(strings.ToLower(c.RecommendedAction), "configure") {
				return fmt.Errorf("want Configure guidance, got %q", c.RecommendedAction)
			}
			fmt.Println("doctor:", c.Message)
		}
	}
	if !found {
		return fmt.Errorf("missing openspec tool projections check")
	}
	plan = runtime.BuildRuntimeRepairPlan(project, insp.Runtime)
	for _, t := range plan.Targets {
		if t.Kind == runtime.RepairKindOpenSpec {
			return fmt.Errorf("Repair claimed OpenSpec authority")
		}
	}
	after, err = homeFingerprint(homeDir)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("Home fingerprint changed during OpenSpec Doctor path")
	}
	fmt.Println("PASS C")
	return nil
}

func homeFingerprint(homePath string) (string, error) {
	h := sha256.New()
	state, err := os.ReadFile(filepath.Join(homePath, "state", "home.yaml"))
	if err != nil {
		return "", err
	}
	h.Write(state)
	err = filepath.Walk(filepath.Join(homePath, "assets"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(homePath, path)
		h.Write([]byte(filepath.ToSlash(rel)))
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		h.Write(sum[:])
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
