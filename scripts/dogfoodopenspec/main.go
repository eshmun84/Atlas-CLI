// Command dogfoodopenspec runs Block D / D5 physical OpenSpec + team-clone
// dogfood against a real OpenSpec CLI and isolated PATH shims.
//
//	make build
//	ATLAS_BIN=./bin/atlas go run ./scripts/dogfoodopenspec
//
// Uses only temporary directories. Never mutates ~/.atlas or the Atlas repo root.
// Does not install OpenSpec. Requires openspec on PATH for real scenarios.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
	"github.com/eshmun84/Atlas-CLI/internal/version"
	"gopkg.in/yaml.v3"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dogfoodopenspec: FAIL: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("dogfoodopenspec: ALL SCENARIOS PASS")
}

func run() error {
	openspecPath, err := exec.LookPath("openspec")
	if err != nil {
		return fmt.Errorf("real openspec not on PATH: %w", err)
	}
	verOut, err := exec.Command(openspecPath, "version", "--json").CombinedOutput()
	if err != nil {
		return fmt.Errorf("openspec version --json: %w (%s)", err, truncate(string(verOut), 200))
	}
	var verObj struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(verOut, &verObj); err != nil || strings.TrimSpace(verObj.Version) == "" {
		return fmt.Errorf("parse openspec version JSON: %w (%s)", err, truncate(string(verOut), 200))
	}
	realVersion := verObj.Version
	fmt.Printf("REAL_OPENSPEC path=%s version=%s atlas=%s\n", openspecPath, realVersion, version.Version)

	probe := openspec.Probe(context.Background(), openspec.ProbeInput{})
	fmt.Printf("PROBE_CLI_ONLY state=%s installed=%s atlas_compat=%v caps_ok=%v msg=%q\n",
		probe.State, probe.Installed, probe.AtlasCompatible, probe.CapabilitiesOK, probe.Message)
	if probe.State != openspec.StateAvailable && probe.State != openspec.StateReady {
		return fmt.Errorf("real OpenSpec probe failed: %#v", probe)
	}

	root := os.Getenv("ATLAS_D5_ROOT")
	if root == "" {
		root, err = os.MkdirTemp("/tmp", "atlas-d5-")
		if err != nil {
			return err
		}
	}
	fmt.Printf("D5_ROOT=%s\n", root)

	atlasBin := os.Getenv("ATLAS_BIN")
	if atlasBin == "" {
		atlasBin = filepath.Join(mustAbs("."), "bin", "atlas")
	}
	wantAtlasVer := strings.TrimSpace(version.Version)
	out, err := exec.Command(atlasBin, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("atlas --version: %w (%s)", err, out)
	}
	gotAtlasVer := strings.TrimSpace(string(out))
	if gotAtlasVer != wantAtlasVer {
		return fmt.Errorf("atlas binary version mismatch: --version=%q, source version.Version=%q (binary under test must match build)",
			gotAtlasVer, wantAtlasVer)
	}
	fmt.Printf("ATLAS_BIN=%s version=%s (matches version.Version)\n", atlasBin, gotAtlasVer)

	// --- Scenario A: Developer A Init (Cursor first) ---
	homeA := filepath.Join(root, "home-a")
	projA := filepath.Join(root, "dev-a", "project")
	if err := os.MkdirAll(projA, 0o755); err != nil {
		return err
	}
	if err := os.Setenv("ATLAS_HOME", homeA); err != nil {
		return err
	}
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		return fmt.Errorf("EnsureAndMirror A: %w", err)
	}
	if err := gitInit(projA); err != nil {
		return err
	}
	if err := applyInit(projA, []string{"cursor"}, "d5-dev-a"); err != nil {
		return fmt.Errorf("scenario A init: %w", err)
	}
	pin, err := readOpenSpecPin(projA)
	if err != nil {
		return err
	}
	if pin != realVersion {
		return fmt.Errorf("scenario A pin=%q want installed %q", pin, realVersion)
	}
	if err := assertReadyOpenSpecTree(projA); err != nil {
		return fmt.Errorf("scenario A tree: %w", err)
	}
	for _, rel := range []string{
		".cursor/commands/opsx-propose.md",
		".cursor/skills/openspec-propose/SKILL.md",
		"AGENTS.md",
		".cursor/rules/atlas.mdc",
		".atlas/contracts/sdd-openspec.md",
	} {
		if _, err := os.Stat(filepath.Join(projA, rel)); err != nil {
			return fmt.Errorf("scenario A missing %s: %w", rel, err)
		}
	}
	if openspec.NeedsToolUpdate(projA, []string{"cursor"}) {
		return fmt.Errorf("scenario A: NeedsToolUpdate true after init")
	}
	repA, err := evaluateDoctor(projA)
	if err != nil {
		return err
	}
	if err := assertDoctorOpenSpec(repA, doctor.SeverityPass, "ready", doctor.RepairUnspecified); err != nil {
		return fmt.Errorf("scenario A doctor: %w", err)
	}
	fmt.Printf("PASS A init pin=%s doctor_openspec=PASS\n", pin)

	// Commit versioned state for clone (local_only gitignore excludes Atlas runtime —
	// force-add required OpenSpec + Atlas contract surfaces that team shares).
	if err := commitCanonical(projA); err != nil {
		return fmt.Errorf("commit canonical: %w", err)
	}

	// --- Scenario J prep + both-adapters coexistence later uses disposable copies ---

	// --- Team clone (Developer B) ---
	homeB := filepath.Join(root, "home-b")
	cloneB := filepath.Join(root, "dev-b", "project")
	if err := os.MkdirAll(filepath.Dir(cloneB), 0o755); err != nil {
		return err
	}
	if err := gitClone(projA, cloneB); err != nil {
		return fmt.Errorf("clone: %w", err)
	}
	if err := os.Setenv("ATLAS_HOME", homeB); err != nil {
		return err
	}
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		return fmt.Errorf("EnsureAndMirror B: %w", err)
	}
	fmt.Printf("PASS team clone home_b=%s clone=%s\n", homeB, cloneB)

	// --- Scenario B: exact pin match ---
	fpBefore := fingerprintTree(cloneB)
	repB, err := evaluateDoctor(cloneB)
	if err != nil {
		return err
	}
	if err := assertDoctorOpenSpec(repB, doctor.SeverityPass, "ready", doctor.RepairUnspecified); err != nil {
		return fmt.Errorf("scenario B doctor: %w", err)
	}
	fpAfter := fingerprintTree(cloneB)
	if !fingerprintsEqual(fpBefore, fpAfter) {
		return fmt.Errorf("scenario B: Status/Doctor mutated project tree")
	}
	pinB, _ := readOpenSpecPin(cloneB)
	if pinB != pin {
		return fmt.Errorf("scenario B pin changed %q -> %q", pin, pinB)
	}
	fmt.Println("PASS B exact pin + read-only fingerprint")

	// --- Scenario C: missing CLI ---
	if err := withIsolatedPATH(root, map[string]string{}, func(pathEnv string) error {
		prev := os.Getenv("PATH")
		defer os.Setenv("PATH", prev)
		_ = os.Setenv("PATH", pathEnv)
		fp0 := fingerprintTree(cloneB)
		rep, err := evaluateDoctor(cloneB)
		if err != nil {
			return err
		}
		if err := assertDoctorOpenSpec(rep, doctor.SeverityFail, "not found", doctor.RepairManual); err != nil {
			return err
		}
		plan := runtime.BuildRuntimeRepairPlan(cloneB, mustInspect(cloneB).Runtime)
		for _, t := range plan.Targets {
			if t.Kind == runtime.RepairKindOpenSpec {
				return fmt.Errorf("Repair planned OpenSpec bootstrap: %#v", t)
			}
		}
		if pin2, _ := readOpenSpecPin(cloneB); pin2 != pin {
			return fmt.Errorf("missing CLI repinned: %q", pin2)
		}
		if !fingerprintsEqual(fp0, fingerprintTree(cloneB)) {
			return fmt.Errorf("missing CLI mutated tree")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("scenario C: %w", err)
	}
	fmt.Println("PASS C missing CLI")

	// --- Scenario D: older CLI shim ---
	if err := withVersionShim(root, "1.14.0", func(pathEnv string) error {
		prev := os.Getenv("PATH")
		defer os.Setenv("PATH", prev)
		_ = os.Setenv("PATH", pathEnv)
		fp0 := fingerprintTree(cloneB)
		rep, err := evaluateDoctor(cloneB)
		if err != nil {
			return err
		}
		if err := assertDoctorOpenSpec(rep, doctor.SeverityFail, "mismatch", doctor.RepairManual); err != nil {
			return err
		}
		if pin2, _ := readOpenSpecPin(cloneB); pin2 != pin {
			return fmt.Errorf("older CLI repinned")
		}
		if !fingerprintsEqual(fp0, fingerprintTree(cloneB)) {
			return fmt.Errorf("older CLI mutated tree")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("scenario D: %w", err)
	}
	fmt.Println("PASS D older CLI mismatch")

	// --- Scenario E: newer CLI shim (still Atlas-compatible major) ---
	if err := withVersionShim(root, "1.99.0", func(pathEnv string) error {
		prev := os.Getenv("PATH")
		defer os.Setenv("PATH", prev)
		_ = os.Setenv("PATH", pathEnv)
		fp0 := fingerprintTree(cloneB)
		rep, err := evaluateDoctor(cloneB)
		if err != nil {
			return err
		}
		if err := assertDoctorOpenSpec(rep, doctor.SeverityFail, "mismatch", doctor.RepairManual); err != nil {
			return err
		}
		if pin2, _ := readOpenSpecPin(cloneB); pin2 != pin {
			return fmt.Errorf("newer CLI repinned")
		}
		if !fingerprintsEqual(fp0, fingerprintTree(cloneB)) {
			return fmt.Errorf("newer CLI mutated tree")
		}
		return nil
	}); err != nil {
		return fmt.Errorf("scenario E: %w", err)
	}
	fmt.Println("PASS E newer CLI mismatch (exact pin wins)")

	// --- Scenario F: provider projections preserved under RO ---
	provBefore := fingerprintSubset(cloneB, []string{".cursor/commands", ".cursor/skills/openspec-propose"})
	_, _ = evaluateDoctor(cloneB)
	provAfter := fingerprintSubset(cloneB, []string{".cursor/commands", ".cursor/skills/openspec-propose"})
	if !fingerprintsEqual(provBefore, provAfter) {
		return fmt.Errorf("scenario F: provider projections mutated by RO inspect")
	}
	// Skills coexist: openspec-* must remain
	if _, err := os.Stat(filepath.Join(cloneB, ".cursor/skills/openspec-propose/SKILL.md")); err != nil {
		return fmt.Errorf("scenario F: openspec-propose skill missing: %w", err)
	}
	fmt.Println("PASS F provider projections preserved")

	// --- Scenario G: missing provider projection ---
	gCopy := filepath.Join(root, "scenario-g")
	if err := copyDir(cloneB, gCopy); err != nil {
		return err
	}
	marker := filepath.Join(gCopy, ".cursor/commands/opsx-propose.md")
	if err := os.Remove(marker); err != nil {
		return err
	}
	fpG0 := fingerprintTree(gCopy)
	repG, err := evaluateDoctor(gCopy)
	if err != nil {
		return err
	}
	// Doctor currently does not FAIL specifically on missing opsx markers.
	// NeedsToolUpdate must detect it for Init/Configure tool sync.
	if !openspec.NeedsToolUpdate(gCopy, []string{"cursor"}) {
		return fmt.Errorf("scenario G: NeedsToolUpdate false after deleting opsx-propose")
	}
	if !fingerprintsEqual(fpG0, fingerprintTree(gCopy)) {
		return fmt.Errorf("scenario G: Doctor mutated tree")
	}
	_ = repG
	fmt.Println("PASS G missing projection: NeedsToolUpdate=true; Doctor RO; no silent fix")

	// --- Scenario H: legacy mkdir ---
	hDir := filepath.Join(root, "scenario-h")
	if err := copyDir(projA, hDir); err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Join(hDir, "openspec"))
	if err := os.MkdirAll(filepath.Join(hDir, "openspec", "changes"), 0o755); err != nil {
		return err
	}
	st, reason := openspec.ClassifyProject(hDir)
	if st != openspec.ProjectLegacyMkdir {
		return fmt.Errorf("scenario H classify=%s reason=%q", st, reason)
	}
	if err := os.Setenv("ATLAS_HOME", homeA); err != nil {
		return err
	}
	repH, err := evaluateDoctor(hDir)
	if err != nil {
		return err
	}
	if err := assertDoctorOpenSpec(repH, doctor.SeverityFail, "legacy", doctor.RepairManual); err != nil {
		return fmt.Errorf("scenario H doctor: %w", err)
	}
	planH := runtime.BuildRuntimeRepairPlan(hDir, mustInspect(hDir).Runtime)
	for _, t := range planH.Targets {
		if t.Kind == runtime.RepairKindOpenSpec {
			return fmt.Errorf("scenario H Repair bootstrap planned")
		}
	}
	fmt.Println("PASS H legacy mkdir fail-closed")

	// --- Scenario I: malformed (symlink root) ---
	iDir := filepath.Join(root, "scenario-i")
	if err := os.MkdirAll(iDir, 0o755); err != nil {
		return err
	}
	real := filepath.Join(root, "scenario-i-real")
	if err := os.MkdirAll(filepath.Join(real, "x"), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(real, filepath.Join(iDir, "openspec")); err != nil {
		return err
	}
	stI, _ := openspec.ClassifyProject(iDir)
	if stI != openspec.ProjectMalformed {
		return fmt.Errorf("scenario I classify=%s want malformed", stI)
	}
	fmt.Println("PASS I malformed symlink fail-closed")

	// --- Scenario J: re-init healthy (second Apply without AcceptHomeReset) ---
	jCopy := filepath.Join(root, "scenario-j")
	if err := copyDir(projA, jCopy); err != nil {
		return err
	}
	if err := os.Setenv("ATLAS_HOME", homeA); err != nil {
		return err
	}
	pinJ0, _ := readOpenSpecPin(jCopy)
	fpJ0 := fingerprintTree(jCopy)
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "d5-dev-a", ProjectMode: "existing", DefaultRemote: "origin",
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.SetValue("source_control.mode", "git_local")
	_, applyErr := config.ApplyConfig(config.ApplyInput{
		Root: jCopy, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return time.Date(2026, 10, 15, 18, 0, 0, 0, time.UTC) },
	})
	pinJ1, _ := readOpenSpecPin(jCopy)
	if pinJ1 != pinJ0 {
		return fmt.Errorf("scenario J pin changed %q -> %q", pinJ0, pinJ1)
	}
	if err := assertReadyOpenSpecTree(jCopy); err != nil {
		return fmt.Errorf("scenario J OpenSpec not ready after re-init attempt: %w", err)
	}
	if applyErr != nil {
		errLower := strings.ToLower(applyErr.Error())
		if !strings.Contains(errLower, "home") && !strings.Contains(errLower, "reset") {
			return fmt.Errorf("scenario J unexpected Apply error: %w", applyErr)
		}
		// Home-reset gate: pin + ProjectReady already verified; provider tree not corrupted.
		fmt.Printf("PASS J re-init Home-reset gate (pin=%s, OpenSpec ready)\n", pinJ1)
	} else {
		// Successful Apply must be idempotent on the project tree.
		fpJ1 := fingerprintTree(jCopy)
		if !fingerprintsEqual(fpJ0, fpJ1) {
			return fmt.Errorf("scenario J Apply succeeded but project fingerprint changed:\n%s",
				fingerprintDiff(fpJ0, fpJ1))
		}
		fmt.Printf("PASS J re-init idempotent Apply (pin=%s, fingerprint unchanged)\n", pinJ1)
	}

	// --- Cursor + OpenCode coexistence ---
	homeBoth := filepath.Join(root, "home-both")
	projBoth := filepath.Join(root, "both-adapters")
	if err := os.MkdirAll(projBoth, 0o755); err != nil {
		return err
	}
	if err := os.Setenv("ATLAS_HOME", homeBoth); err != nil {
		return err
	}
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		return err
	}
	if err := gitInit(projBoth); err != nil {
		return err
	}
	if err := applyInit(projBoth, []string{"cursor", "opencode"}, "d5-both"); err != nil {
		return fmt.Errorf("both adapters init: %w", err)
	}
	for _, rel := range []string{
		".cursor/commands/opsx-propose.md",
		".cursor/skills/openspec-propose/SKILL.md",
		".opencode/commands/opsx-propose.md",
		".opencode/skills/openspec-propose/SKILL.md",
		".cursor/rules/atlas.mdc",
		".opencode/atlas.md",
	} {
		if _, err := os.Stat(filepath.Join(projBoth, rel)); err != nil {
			return fmt.Errorf("both adapters missing %s: %w", rel, err)
		}
	}
	if openspec.NeedsToolUpdate(projBoth, []string{"cursor", "opencode"}) {
		return fmt.Errorf("both adapters NeedsToolUpdate true")
	}
	fmt.Println("PASS Cursor+OpenCode coexistence")

	// --- Runtime Repair boundary on absent OpenSpec ---
	absDir := filepath.Join(root, "repair-absent")
	if err := copyDir(projA, absDir); err != nil {
		return err
	}
	_ = os.RemoveAll(filepath.Join(absDir, "openspec"))
	if err := os.Setenv("ATLAS_HOME", homeA); err != nil {
		return err
	}
	planAbs := runtime.BuildRuntimeRepairPlan(absDir, mustInspect(absDir).Runtime)
	for _, t := range planAbs.Targets {
		if t.Kind == runtime.RepairKindOpenSpec || strings.HasPrefix(t.Path, "openspec/") {
			return fmt.Errorf("Repair bootstrap target: %#v", t)
		}
	}
	if _, err := os.Stat(filepath.Join(absDir, "openspec")); !os.IsNotExist(err) {
		return fmt.Errorf("openspec/ created by Repair plan")
	}
	fmt.Println("PASS Runtime Repair refuses OpenSpec bootstrap")

	fmt.Printf("EVIDENCE_ROOT=%s (preserved for report; safe to rm -rf)\n", root)
	return nil
}

func applyInit(root string, adapters []string, name string) error {
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:   name,
		ProjectMode:   "existing",
		DefaultRemote: "origin",
	})
	for _, a := range adapters {
		if !draft.ToggleMulti("adapters.selected", a) {
			return fmt.Errorf("toggle adapter %s", a)
		}
	}
	if !draft.SetValue("source_control.mode", "git_local") {
		return fmt.Errorf("set git_local")
	}
	_, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return time.Date(2026, 10, 15, 17, 30, 0, 0, time.UTC) },
	})
	return err
}

func evaluateDoctor(root string) (doctor.Report, error) {
	insp, err := inspect.Inspect(root)
	if err != nil {
		return doctor.Report{}, err
	}
	return doctor.Evaluate(insp), nil
}

func mustInspect(root string) inspect.Inspection {
	insp, err := inspect.Inspect(root)
	if err != nil {
		panic(err)
	}
	return insp
}

func assertDoctorOpenSpec(rep doctor.Report, sev doctor.Severity, msgSub string, wantRepair doctor.Repairability) error {
	for _, c := range rep.Checks {
		if c.Name != "openspec provider" {
			continue
		}
		if c.Severity != sev {
			return fmt.Errorf("severity severity=%s want %s msg=%q", c.Severity, sev, c.Message)
		}
		if msgSub != "" && !strings.Contains(strings.ToLower(c.Message), strings.ToLower(msgSub)) {
			return fmt.Errorf("openspec message %q missing %q", c.Message, msgSub)
		}
		if sev == doctor.SeverityFail && wantRepair != doctor.RepairUnspecified && c.Repairability != wantRepair {
			return fmt.Errorf("repairability=%s want %s", c.Repairability, wantRepair)
		}
		return nil
	}
	return fmt.Errorf("openspec provider check not found (checks=%d)", len(rep.Checks))
}

func assertReadyOpenSpecTree(root string) error {
	st, reason := openspec.ClassifyProject(root)
	if st != openspec.ProjectReady {
		return fmt.Errorf("classify=%s reason=%q", st, reason)
	}
	for _, rel := range []string{
		"openspec/config.yaml",
		"openspec/specs",
		"openspec/changes",
		"openspec/changes/archive",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("missing %s: %w", rel, err)
		}
	}
	return nil
}

func readOpenSpecPin(root string) (string, error) {
	data, err := os.ReadFile(filepath.Join(root, ".atlas", "config.yaml"))
	if err != nil {
		return "", err
	}
	var doc struct {
		Governance struct {
			OpenSpecVersion string `yaml:"openspec_version"`
		} `yaml:"governance"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return "", err
	}
	return strings.TrimSpace(doc.Governance.OpenSpecVersion), nil
}

func gitInit(dir string) error {
	cmds := [][]string{
		{"git", "init", "-b", "main"},
		{"git", "config", "user.email", "d5@atlas.test"},
		{"git", "config", "user.name", "Atlas D5"},
	}
	for _, c := range cmds {
		cmd := exec.Command(c[0], c[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %w (%s)", c, err, out)
		}
	}
	return nil
}

func commitCanonical(dir string) error {
	// local_only gitignore excludes Atlas/runtime — force-add what a team must share
	// for OpenSpec pin + provider tree + Atlas config.
	add := []string{
		".atlas/config.yaml",
		"openspec",
		".cursor/commands",
		".cursor/skills/openspec-propose",
		"AGENTS.md",
		".cursor/rules/atlas.mdc",
		".gitignore",
		"README.md",
	}
	for _, rel := range add {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err != nil {
			continue
		}
		cmd := exec.Command("git", "add", "-f", rel)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git add %s: %w (%s)", rel, err, out)
		}
	}
	cmd := exec.Command("git", "commit", "-m", "d5 canonical openspec project", "--allow-empty")
	cmd.Dir = dir
	// Signing may be required in agent env — force no-gpg for dogfood fixture only.
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_NAME=Atlas D5", "GIT_AUTHOR_NAME=Atlas D5")
	if out, err := cmd.CombinedOutput(); err != nil {
		// Retry without signing
		cmd2 := exec.Command("git", "-c", "commit.gpgsign=false", "commit", "-m", "d5 canonical openspec project")
		cmd2.Dir = dir
		if out2, err2 := cmd2.CombinedOutput(); err2 != nil {
			return fmt.Errorf("git commit: %w (%s / %s)", err2, out, out2)
		}
	}
	return nil
}

func gitClone(src, dst string) error {
	cmd := exec.Command("git", "clone", src, dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w (%s)", err, out)
	}
	return nil
}

func withIsolatedPATH(root string, shims map[string]string, fn func(pathEnv string) error) error {
	bin := filepath.Join(root, "pathbins", fmt.Sprintf("bin-%d", time.Now().UnixNano()))
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	// Provide git from real PATH so Doctor Git checks still work.
	realGit, _ := exec.LookPath("git")
	if realGit != "" {
		_ = os.Symlink(realGit, filepath.Join(bin, "git"))
	}
	for name, body := range shims {
		path := filepath.Join(bin, name)
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			return err
		}
	}
	return fn(bin)
}

func withVersionShim(root, version string, fn func(pathEnv string) error) error {
	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
  version)
    if [ "$2" = "--json" ]; then
      printf '{"schemaVersion":1,"version":"%s"}\n'
      exit 0
    fi
    echo "%s"
    exit 0
    ;;
  list)
    printf '{"root":null,"changes":[],"status":[]}\n'
    exit 0
    ;;
  init|update|status|instructions|validate|archive)
    if [ "$2" = "--help" ] || [ "$1" = "init" ]; then
      echo "Usage: openspec $1 [--json] [--tools]"
      exit 0
    fi
    printf '{"ok":true}\n'
    exit 0
    ;;
  *)
    echo "shim openspec: unhandled $*" >&2
    exit 1
    ;;
esac
`, version, version)
	return withIsolatedPATH(root, map[string]string{"openspec": script}, fn)
}

func fingerprintTree(root string) map[string]string {
	out := map[string]string{}
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(path, string(filepath.Separator)+".git"+string(filepath.Separator)) || strings.HasSuffix(path, string(filepath.Separator)+".git") {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	return out
}

func fingerprintSubset(root string, prefixes []string) map[string]string {
	all := fingerprintTree(root)
	out := map[string]string{}
	for k, v := range all {
		for _, p := range prefixes {
			if k == p || strings.HasPrefix(k, strings.TrimSuffix(p, "/")+"/") {
				out[k] = v
				break
			}
		}
	}
	return out
}

func fingerprintsEqual(a, b map[string]string) bool {
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

func fingerprintDiff(before, after map[string]string) string {
	var lines []string
	for k, v := range before {
		if after[k] == "" {
			lines = append(lines, "- removed "+k)
		} else if after[k] != v {
			lines = append(lines, "- changed "+k)
		}
	}
	for k := range after {
		if before[k] == "" {
			lines = append(lines, "- added "+k)
		}
	}
	sort.Strings(lines)
	if len(lines) > 40 {
		lines = append(lines[:40], fmt.Sprintf("… %d more", len(lines)-40))
	}
	return strings.Join(lines, "\n")
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == ".git" || strings.HasPrefix(rel, ".git"+string(os.PathSeparator)) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, in)
		closeErr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
}

func mustAbs(p string) string {
	a, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	return a
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
