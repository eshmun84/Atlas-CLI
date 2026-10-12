// Command prepareinteractive creates a durable, isolated workspace for
// interactive Cursor/OpenCode Context Composition E2E under Atlas-E2E.
//
// Required env:
//
//	ATLAS_E2E_ROOT — existing directory (e.g. .../Atlas-E2E/context-compose-36b6e)
//	ATLAS_BIN      — absolute path to atlas binary built from validated HEAD
//
// Safety: never deletes the E2E root or preexisting children. Always creates a
// new uniquely named interactive-* directory.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
	"github.com/eshmun84/Atlas-CLI/internal/contextcompose/composeapp"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/memory"
)

const (
	canaryMemA = "CANARY-MEM-36B6E-ALPHA"
	canaryMemB = "CANARY-MEM-36B6E-BETA"
	canaryCEA  = "CANARY-CE-36B6E-ALPHA.md"
	canaryCEB  = "CANARY-CE-36B6E-BETA.md"
	taskPrompt = "Identify project architectural decisions"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "prepareinteractive: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	e2eRoot := strings.TrimSpace(os.Getenv("ATLAS_E2E_ROOT"))
	atlasBin := strings.TrimSpace(os.Getenv("ATLAS_BIN"))
	if e2eRoot == "" || atlasBin == "" {
		return fmt.Errorf("ATLAS_E2E_ROOT and ATLAS_BIN are required")
	}
	absRoot, err := filepath.Abs(e2eRoot)
	if err != nil {
		return err
	}
	info, err := os.Lstat(absRoot)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("ATLAS_E2E_ROOT must be an existing real directory: %s", absRoot)
	}
	absBin, err := filepath.Abs(atlasBin)
	if err != nil {
		return err
	}
	if st, err := os.Stat(absBin); err != nil || st.IsDir() {
		return fmt.Errorf("ATLAS_BIN not usable: %s", absBin)
	}

	stamp := time.Now().UTC().Format("20060102-150405")
	runDir := filepath.Join(absRoot, "interactive-"+stamp)
	if err := os.Mkdir(runDir, 0o755); err != nil {
		return fmt.Errorf("create run dir (refusing reuse/collision): %w", err)
	}

	atlasHome := filepath.Join(runDir, "atlas-home")
	projA := filepath.Join(runDir, "project-a")
	projB := filepath.Join(runDir, "project-b")
	evidence := filepath.Join(runDir, "evidence")
	binDir := filepath.Join(runDir, "bin")
	for _, d := range []string{atlasHome, projA, projB, evidence, binDir} {
		if err := os.Mkdir(d, 0o755); err != nil {
			return err
		}
	}

	// Project-local reversible binary: copy (not symlink) so GUI agents can
	// invoke a known path without global PATH changes.
	localAtlas := filepath.Join(binDir, "atlas")
	if err := copyFile(absBin, localAtlas, 0o755); err != nil {
		return fmt.Errorf("copy atlas binary: %w", err)
	}
	for _, proj := range []string{projA, projB} {
		pb := filepath.Join(proj, "bin")
		if err := os.Mkdir(pb, 0o755); err != nil {
			return err
		}
		wrapper := filepath.Join(pb, "atlas")
		body := fmt.Sprintf("#!/bin/sh\nexec %q \"$@\"\n", localAtlas)
		if err := os.WriteFile(wrapper, []byte(body), 0o755); err != nil {
			return err
		}
	}

	verOut, err := exec.Command(localAtlas, "--version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("atlas --version: %v", err)
	}
	version := strings.TrimSpace(string(verOut))
	sum, err := fileSHA256(localAtlas)
	if err != nil {
		return err
	}

	if err := os.Setenv("ATLAS_HOME", atlasHome); err != nil {
		return err
	}
	fixed := time.Date(2026, 10, 12, 19, 0, 0, 0, time.UTC)

	idA, err := initProject(projA, atlasHome, "interactive-a-36b6e", fixed)
	if err != nil {
		return fmt.Errorf("init project-a: %w", err)
	}
	idB, err := initProject(projB, atlasHome, "interactive-b-36b6e", fixed)
	if err != nil {
		return fmt.Errorf("init project-b: %w", err)
	}
	if idA == idB {
		return fmt.Errorf("project id collision: %s", idA)
	}

	if err := seedMemory(atlasHome, idA, "Alpha architectural decision",
		"Decision: prefer SQLite Memory under Atlas Home. Token "+canaryMemA); err != nil {
		return fmt.Errorf("memory A: %w", err)
	}
	if err := seedMemory(atlasHome, idB, "Beta architectural decision",
		"Decision: isolate peer project Memory. Token "+canaryMemB); err != nil {
		return fmt.Errorf("memory B: %w", err)
	}

	if err := os.WriteFile(filepath.Join(projA, canaryCEA), []byte("# Alpha CE canary\nToken path for interactive E2E.\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(projB, canaryCEB), []byte("# Beta CE canary\nToken path for interactive E2E.\n"), 0o644); err != nil {
		return err
	}
	if err := rebuildCE(projA, atlasHome, idA, "interactive-a-36b6e", fixed, canaryCEA, taskPrompt); err != nil {
		return fmt.Errorf("CE A: %w", err)
	}
	if err := rebuildCE(projB, atlasHome, idB, "interactive-b-36b6e", fixed, canaryCEB, taskPrompt); err != nil {
		return fmt.Errorf("CE B: %w", err)
	}

	beforeHome, err := fingerprint(atlasHome)
	if err != nil {
		return err
	}
	beforeA, err := fingerprint(projA)
	if err != nil {
		return err
	}
	beforeB, err := fingerprint(projB)
	if err != nil {
		return err
	}

	resA, rawA, err := compose(localAtlas, atlasHome, projA, taskPrompt)
	if err != nil && resA.SchemaVersion == 0 {
		return fmt.Errorf("compose A: %w", err)
	}
	resB, rawB, err := compose(localAtlas, atlasHome, projB, taskPrompt)
	if err != nil && resB.SchemaVersion == 0 {
		return fmt.Errorf("compose B: %w", err)
	}
	if err := writeEvidenceFile(filepath.Join(evidence, "compose-project-a.json"), rawA); err != nil {
		return fmt.Errorf("evidence compose-project-a.json: %w", err)
	}
	if err := writeEvidenceFile(filepath.Join(evidence, "compose-project-b.json"), rawB); err != nil {
		return fmt.Errorf("evidence compose-project-b.json: %w", err)
	}

	if err := assertCompose(resA, idA, canaryMemA, canaryCEA, canaryMemB, canaryCEB); err != nil {
		return fmt.Errorf("verify project-a: %w", err)
	}
	if err := assertCompose(resB, idB, canaryMemB, canaryCEB, canaryMemA, canaryCEA); err != nil {
		return fmt.Errorf("verify project-b: %w", err)
	}

	afterHome, err := fingerprint(atlasHome)
	if err != nil {
		return fmt.Errorf("fingerprint ATLAS_HOME after compose: %w", err)
	}
	afterA, err := fingerprint(projA)
	if err != nil {
		return fmt.Errorf("fingerprint project-a after compose: %w", err)
	}
	afterB, err := fingerprint(projB)
	if err != nil {
		return fmt.Errorf("fingerprint project-b after compose: %w", err)
	}
	if beforeHome != afterHome || beforeA != afterA || beforeB != afterB {
		return fmt.Errorf("filesystem mutated during composition verification")
	}

	for _, proj := range []string{projA, projB} {
		for _, rel := range []string{
			"AGENTS.md",
			".atlas/config.yaml",
			".cursor/skills/memory/SKILL.md",
			".opencode/skills/memory/SKILL.md",
			".cursor/rules/atlas.mdc",
			".opencode/atlas.md",
			"bin/atlas",
		} {
			if _, err := os.Lstat(filepath.Join(proj, filepath.FromSlash(rel))); err != nil {
				return fmt.Errorf("missing %s in %s: %w", rel, filepath.Base(proj), err)
			}
		}
		agents, _ := os.ReadFile(filepath.Join(proj, "AGENTS.md"))
		skill, _ := os.ReadFile(filepath.Join(proj, ".cursor/skills/memory/SKILL.md"))
		if !bytes.Contains(agents, []byte("atlas context compose")) || !bytes.Contains(skill, []byte("atlas context compose")) {
			return fmt.Errorf("%s missing compose instructions", filepath.Base(proj))
		}
	}

	head := strings.TrimSpace(os.Getenv("ATLAS_EXPECTED_HEAD"))
	if err := writeInteractiveDoc(evidence, absBin, localAtlas, version, sum, head, runDir, atlasHome, projA, projB, idA, idB, resA, resB); err != nil {
		return err
	}
	meta := map[string]any{
		"slice":            "36B.6E-interactive",
		"run_dir":          runDir,
		"atlas_bin_source": absBin,
		"atlas_bin_local":  localAtlas,
		"atlas_version":    version,
		"atlas_sha256":     sum,
		"expected_head":    head,
		"ATLAS_HOME":       atlasHome,
		"project_a":        projA,
		"project_b":        projB,
		"project_a_id":     idA,
		"project_b_id":     idB,
		"compose_a": map[string]any{
			"status": resA.Status, "count": resA.Count, "used": resA.UsedChars, "budget": resA.BudgetChars,
		},
		"compose_b": map[string]any{
			"status": resB.Status, "count": resB.Count, "used": resB.UsedChars, "budget": resB.BudgetChars,
		},
		"readonly": true,
	}
	mb, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ENVIRONMENT.json: %w", err)
	}
	if err := writeEvidenceFile(filepath.Join(evidence, "ENVIRONMENT.json"), mb); err != nil {
		return fmt.Errorf("evidence ENVIRONMENT.json: %w", err)
	}

	fmt.Printf("run_dir=%s\n", runDir)
	fmt.Printf("project_a=%s\n", projA)
	fmt.Printf("project_b=%s\n", projB)
	fmt.Printf("ATLAS_HOME=%s\n", atlasHome)
	fmt.Printf("atlas_bin=%s\n", localAtlas)
	fmt.Printf("atlas_version=%s sha256=%s\n", version, sum)
	fmt.Printf("project_a_id=%s\n", idA)
	fmt.Printf("project_b_id=%s\n", idB)
	fmt.Printf("evidence=%s\n", evidence)
	fmt.Printf("instructions=%s\n", filepath.Join(evidence, "INTERACTIVE-E2E.md"))
	fmt.Println("prepareinteractive: ok")
	return nil
}

func initProject(root, atlasHome, name string, now time.Time) (string, error) {
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName:      name,
		ProjectMode:      "new",
		DefaultRemote:    "origin",
		CursorDetected:   true,
		OpenCodeDetected: true,
	})
	if !draft.SetValue("adapters.selected", "cursor,opencode") {
		return "", fmt.Errorf("set adapters.selected")
	}
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root:  root,
		Draft: draft,
		MCP:   config.EmptyMCPDraft(),
		Now:   func() time.Time { return now },
	}); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module "+name+"\n"), 0o644); err != nil {
		return "", err
	}
	return home.ProjectID(root, name)
}

func seedMemory(atlasHome, projectID, title, content string) error {
	store, err := memory.Open(atlasHome, projectID)
	if err != nil {
		return err
	}
	defer store.Close()
	_, err = store.SaveObservation(memory.Observation{
		Type:    memory.TypeDecision,
		Title:   title,
		Content: content,
	})
	return err
}

func rebuildCE(root, atlasHome, projectID, name string, now time.Time, ensurePath, task string) error {
	idx, err := atlascontext.BuildIndex(root, name, now)
	if err != nil {
		return err
	}
	idx.ProjectID = projectID
	found := false
	for _, f := range idx.Files {
		if f.Path == ensurePath {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("index missing %s", ensurePath)
	}
	indexYAML, err := atlascontext.RenderIndexYAML(idx)
	if err != nil {
		return err
	}
	capsule := atlascontext.RenderCapsule(idx)
	ensureCanary := func(pack *atlascontext.PackDocument) {
		for _, c := range pack.Candidates {
			if c.Path == ensurePath {
				return
			}
		}
		pack.Candidates = append([]atlascontext.PackCandidate{{
			Path: ensurePath, Kind: "doc", Reason: "interactive e2e canary",
		}}, pack.Candidates...)
		if len(pack.Candidates) > 24 {
			pack.Candidates = pack.Candidates[:24]
		}
	}
	def := atlascontext.BuildPack(idx, atlascontext.DefaultPackObjective, now)
	ensureCanary(&def)
	taskPack := atlascontext.BuildPack(idx, task, now)
	ensureCanary(&taskPack)
	defYAML, err := atlascontext.RenderPackYAML(def)
	if err != nil {
		return err
	}
	taskYAML, err := atlascontext.RenderPackYAML(taskPack)
	if err != nil {
		return err
	}
	for _, p := range []struct{ path, data string }{
		{atlascontext.IndexPath(atlasHome, projectID), indexYAML},
		{atlascontext.CapsulePath(atlasHome, projectID), capsule},
		{atlascontext.PackPath(atlasHome, projectID, def.PackID), defYAML},
		{atlascontext.PackPath(atlasHome, projectID, taskPack.PackID), taskYAML},
	} {
		if err := os.MkdirAll(filepath.Dir(p.path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(p.path, []byte(p.data), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func assertCompose(res composeapp.Result, wantID, memCanary, ceCanary, otherMem, otherCE string) error {
	if res.SchemaVersion != contextcompose.SchemaVersion {
		return fmt.Errorf("schema=%d", res.SchemaVersion)
	}
	if res.ProjectID != wantID {
		return fmt.Errorf("project_id=%q want %q", res.ProjectID, wantID)
	}
	if res.UsedChars > res.BudgetChars {
		return fmt.Errorf("used %d > budget %d", res.UsedChars, res.BudgetChars)
	}
	if !hasSource(res, contextcompose.SourceMemory, contextcompose.SourceStatusSupplied) {
		return fmt.Errorf("memory not supplied: %#v", res.Sources)
	}
	if !hasSource(res, contextcompose.SourceContextEconomy, contextcompose.SourceStatusSupplied) {
		return fmt.Errorf("ce not supplied: %#v", res.Sources)
	}
	if !contains(res, memCanary) {
		return fmt.Errorf("missing memory canary %s", memCanary)
	}
	if !hasPath(res, ceCanary) {
		return fmt.Errorf("missing CE path %s", ceCanary)
	}
	if contains(res, otherMem) || hasPath(res, otherCE) {
		return fmt.Errorf("cross-project leakage detected")
	}
	return nil
}

func hasSource(res composeapp.Result, source, status string) bool {
	for _, s := range res.Sources {
		if s.Source == source && s.Status == status {
			return true
		}
	}
	return false
}

func contains(res composeapp.Result, token string) bool {
	for _, it := range res.Items {
		if strings.Contains(it.Title, token) || strings.Contains(it.Excerpt, token) || strings.Contains(it.Ref, token) {
			return true
		}
	}
	return false
}

func hasPath(res composeapp.Result, path string) bool {
	for _, it := range res.Items {
		if it.Path == path {
			return true
		}
	}
	return false
}

func fingerprint(root string) (string, error) {
	h := sha256.New()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		_, _ = io.WriteString(h, rel+"\n")
		if info.Mode()&os.ModeSymlink != 0 {
			t, err := os.Readlink(path)
			if err != nil {
				return err
			}
			_, _ = io.WriteString(h, "symlink->"+t+"\n")
			return nil
		}
		if info.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(h, io.LimitReader(f, 8<<20))
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// writeEvidenceFile creates a new regular evidence file exclusively (O_EXCL).
// It never follows a symlink leaf and never overwrites an existing path.
// Callers must treat any error as fatal — success output must not be printed
// if this fails.
func writeEvidenceFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	n, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(path)
		return werr
	}
	if n != len(data) {
		_ = os.Remove(path)
		return fmt.Errorf("short write to %s: %d/%d", path, n, len(data))
	}
	if cerr != nil {
		_ = os.Remove(path)
		return cerr
	}
	return nil
}

func writeInteractiveDoc(evidence, sourceBin, localBin, version, sum, head, runDir, atlasHome, projA, projB, idA, idB string, resA, resB composeapp.Result) error {
	body := fmt.Sprintf(`# Interactive Cursor & OpenCode E2E (36B.6E)

**Status of this document:** environment preparation complete.  
**Agent-consumption PASS must NOT be claimed** until a real Cursor/OpenCode session is observed.

## Environment identity

| Item | Value |
|------|--------|
| Expected HEAD | %s |
| Source binary | %s |
| Local binary (use this) | %s |
| Version | %s |
| SHA-256 | %s |
| Run directory | %s |
| ATLAS_HOME | %s |
| project-a | %s |
| project-a id | %s |
| project-b | %s |
| project-b id | %s |

## Canaries

| Project | Memory token | CE path |
|---------|--------------|---------|
| A | %s | %s |
| B | %s | %s |

## Preflight CLI (already verified)

Command (from each project root):

`+"```"+`bash
export ATLAS_HOME=%q
%q context compose %q --json
`+"```"+`

Observed:

- project-a: status=%s count=%d used=%d budget=%d (Memory+CE supplied, Alpha canaries, no Beta leak)
- project-b: status=%s count=%d used=%d budget=%d (Memory+CE supplied, Beta canaries, no Alpha leak)
- Fingerprints unchanged after compose (read-only)

## Project-local Atlas binary (GUI PATH)

Do **not** change global PATH or agent settings.

Each project has:

`+"```"+`text
<project>/bin/atlas  → wrapper exec'ing %s
`+"```"+`

Options for agents whose GUI PATH differs from the shell:

1. **Preferred:** tell the agent to invoke the absolute path:
   `+"```"+`text
   %s
   `+"```"+`
2. **Session-only shell:** in the integrated terminal for that project:
   `+"```"+`bash
   export PATH="%s:%s:$PATH"
   export ATLAS_HOME=%q
   which atlas   # must resolve to project or run-dir bin
   atlas --version
   `+"```"+`
3. Revert by closing the terminal / unsetting PATH for that session only.

## Cursor IDE — project-a

1. Open folder: `+"```"+`%s`+"```"+`
2. Confirm files exist: AGENTS.md, .cursor/skills/memory/SKILL.md, bin/atlas
3. Set terminal env for the session (see above) **or** use the absolute atlas path in prompts.
4. Start a **new** Agent chat in this project.
5. Send exactly:

`+"```"+`text
Orient me on this project's architectural decisions. Use Atlas composed context via:
%s context compose "Identify project architectural decisions" --json
with ATLAS_HOME=%s
Cite any canary tokens or CE paths you retrieve. Do not claim automatic hook injection.
`+"```"+`

6. Confirm in the tool/terminal transcript that the agent invoked `+"`context compose`"+` (or the absolute command).
7. Confirm the answer cites Memory token `+"```"+`%s`+"```"+` and/or CE path `+"```"+`%s`+"```"+`.
8. Confirm it does **not** cite Beta canaries (%s / %s).
9. Save transcript excerpt to:
   `+"```"+`%s/cursor-project-a.txt`+"```"+`

### PASS criteria (Cursor / project-a)

- [ ] Tool invocation of atlas context compose observed
- [ ] Alpha Memory canary cited
- [ ] Alpha CE path cited (or clearly used from compose JSON)
- [ ] No Beta leakage
- [ ] No claim of native automatic injection

## OpenCode — project-a

1. Open the same folder in OpenCode: `+"```"+`%s`+"```"+`
2. Confirm .opencode/skills/memory/SKILL.md and .opencode/atlas.md
3. Use the same absolute atlas path + ATLAS_HOME
4. Send the same prompt as Cursor
5. Save transcript to:
   `+"```"+`%s/opencode-project-a.txt`+"```"+`

### PASS criteria (OpenCode / project-a)

- Same checklist as Cursor

## Isolation spot-check (optional)

Open project-b and repeat with the same prompt. Expect Beta canaries only. Save to cursor-project-b.txt / opencode-project-b.txt.

## Evidence locations

| File | Purpose |
|------|---------|
| %s/INTERACTIVE-E2E.md | This procedure |
| %s/ENVIRONMENT.json | Machine-readable env identity |
| %s/compose-project-a.json | CLI compose proof (A) |
| %s/compose-project-b.json | CLI compose proof (B) |
| %s/cursor-project-a.txt | **You** create after Cursor session |
| %s/opencode-project-a.txt | **You** create after OpenCode session |

## Security reminders

- No global Cursor/OpenCode config changes
- No production ~/.atlas or real project Memory
- No Git init/commit in these projects unless you explicitly choose to
- Do not delete sibling E2E run directories
- Composition is read-only; do not run Context Economy Update unless testing that flow separately
`,
		head, sourceBin, localBin, version, sum, runDir, atlasHome, projA, idA, projB, idB,
		canaryMemA, canaryCEA, canaryMemB, canaryCEB,
		atlasHome, localBin, taskPrompt,
		resA.Status, resA.Count, resA.UsedChars, resA.BudgetChars,
		resB.Status, resB.Count, resB.UsedChars, resB.BudgetChars,
		localBin, localBin, filepath.Join(projA, "bin"), filepath.Join(runDir, "bin"), atlasHome,
		projA, localBin, atlasHome, canaryMemA, canaryCEA, canaryMemB, canaryCEB, evidence,
		projA, evidence,
		evidence, evidence, evidence, evidence, evidence, evidence,
	)
	return os.WriteFile(filepath.Join(evidence, "INTERACTIVE-E2E.md"), []byte(body), 0o600)
}
