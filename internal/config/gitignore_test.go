package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func localOnlyDoc() config.ProjectDocument {
	return config.ProjectDocument{
		SourceControl: config.SourceControlPersist{GovernanceFiles: config.GovernanceFilesLocalOnly},
	}
}

func TestMergeAtlasGitignore_ZeroBlocks(t *testing.T) {
	got, err := config.MergeAtlasGitignoreBlockForTest("bin/\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "bin/\n") {
		t.Fatalf("lost project rules: %q", got)
	}
	if strings.Count(got, config.AtlasLocalGovernanceGitignoreBlock()) != 1 &&
		strings.Count(got, "# >>> Atlas local governance >>>") != 1 {
		t.Fatalf("want one atlas block: %q", got)
	}
}

func TestMergeAtlasGitignore_OneCorrectBlock_Noop(t *testing.T) {
	existing := "bin/\n\n" + config.AtlasLocalGovernanceGitignoreBlock()
	got, err := config.MergeAtlasGitignoreBlockForTest(existing)
	if err != nil {
		t.Fatal(err)
	}
	if got != existing {
		t.Fatalf("correct block must be byte-identical\ngot %q\nwant %q", got, existing)
	}
}

func TestMergeAtlasGitignore_OneStaleBlock(t *testing.T) {
	stale := "keep/\n# >>> Atlas local governance >>>\n.atlas/\n# <<< Atlas local governance <<<\n"
	got, err := config.MergeAtlasGitignoreBlockForTest(stale)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "keep/\n") {
		t.Fatalf("lost project rules: %q", got)
	}
	if strings.Count(got, "# >>> Atlas local governance >>>") != 1 {
		t.Fatalf("want one block: %q", got)
	}
	if !strings.Contains(got, config.AtlasLocalGovernanceGitignoreBlock()) {
		t.Fatalf("not canonical: %q", got)
	}
}

func TestMergeAtlasGitignore_TwoBlocksConsolidate(t *testing.T) {
	block := config.AtlasLocalGovernanceGitignoreBlock()
	existing := "top/\n" + block + "middle/\n" + block + "bottom/\n"
	got, err := config.MergeAtlasGitignoreBlockForTest(existing)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, "# >>> Atlas local governance >>>") != 1 {
		t.Fatalf("want exactly one atlas block: %q", got)
	}
	if !strings.Contains(got, "top/\n") || !strings.Contains(got, "middle/\n") || !strings.Contains(got, "bottom/\n") {
		t.Fatalf("project-owned content lost: %q", got)
	}
	if !strings.Contains(got, config.AtlasLocalGovernanceGitignoreBlock()) {
		t.Fatalf("missing canonical block: %q", got)
	}
}

func TestMergeAtlasGitignore_MalformedFailClosed(t *testing.T) {
	cases := []string{
		"x\n# >>> Atlas local governance >>>\n.atlas/\n",
		"x\n# <<< Atlas local governance <<<\n",
		"x\n# >>> Atlas local governance >>>\n# >>> Atlas local governance >>>\n# <<< Atlas local governance <<<\n",
	}
	for _, in := range cases {
		_, err := config.MergeAtlasGitignoreBlockForTest(in)
		if err == nil || !strings.Contains(err.Error(), "malformed") {
			t.Fatalf("expected malformed error for %q, got %v", in, err)
		}
	}
}

func TestReconcileLocalOnlyGitignore_IdempotentAndFailClosed(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	homePath := os.Getenv("ATLAS_HOME")
	pid, err := home.ProjectID(root, "gi")
	if err != nil {
		t.Fatal(err)
	}
	doc := localOnlyDoc()
	now := time.Date(2026, 10, 14, 20, 0, 0, 0, time.UTC)

	// Create with zero blocks.
	r1, _, _, err := config.ReconcileLocalOnlyGitignore(root, homePath, pid, doc, now)
	if err != nil || !r1.Created || !r1.Changed {
		t.Fatalf("create: %#v err=%v", r1, err)
	}
	path := filepath.Join(root, ".gitignore")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Second reconcile: no-op, no backup churn.
	r2, _, _, err := config.ReconcileLocalOnlyGitignore(root, homePath, pid, doc, now.Add(time.Minute))
	if err != nil || r2.Changed {
		t.Fatalf("noop: %#v err=%v", r2, err)
	}
	second, _ := os.ReadFile(path)
	if string(second) != string(first) {
		t.Fatal("bytes changed on idempotent reconcile")
	}

	// Malformed: fail closed, bytes intact.
	malformed := []byte("keep\n# >>> Atlas local governance >>>\nonly-open\n")
	if err := os.WriteFile(path, malformed, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, _, err = config.ReconcileLocalOnlyGitignore(root, homePath, pid, doc, now.Add(2*time.Minute))
	if err == nil || !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected malformed, got %v", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(malformed) {
		t.Fatalf("bytes mutated on fail-closed: %q", after)
	}
}

func TestReconcileLocalOnlyGitignore_TwoBlocksBackup(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	homePath := os.Getenv("ATLAS_HOME")
	pid, err := home.ProjectID(root, "gi2")
	if err != nil {
		t.Fatal(err)
	}
	block := config.AtlasLocalGovernanceGitignoreBlock()
	existing := "proj/\n" + block + "mid/\n" + block
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := localOnlyDoc()
	res, _, _, err := config.ReconcileLocalOnlyGitignore(root, homePath, pid, doc, time.Date(2026, 10, 14, 21, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if !res.Changed || res.BackupDir == "" {
		t.Fatalf("expected change+backup: %#v", res)
	}
	got, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if strings.Count(string(got), "# >>> Atlas local governance >>>") != 1 {
		t.Fatalf("want one block: %s", got)
	}
	if !strings.Contains(string(got), "proj/\n") || !strings.Contains(string(got), "mid/\n") {
		t.Fatalf("project content lost: %s", got)
	}
	bak, err := os.ReadFile(filepath.Join(res.BackupDir, ".gitignore"))
	if err != nil || string(bak) != existing {
		t.Fatalf("backup = %q err=%v", bak, err)
	}
}

func TestAnalyzeLocalOnlyGitignore_States(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	doc := localOnlyDoc()

	a, err := config.AnalyzeLocalOnlyGitignore(root, doc)
	if err != nil || a.State != config.GitignoreMissingFile || !a.Reconcilable {
		t.Fatalf("missing file: %#v %v", a, err)
	}

	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("bin/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err = config.AnalyzeLocalOnlyGitignore(root, doc)
	if err != nil || a.State != config.GitignoreMissingBlock || !a.Reconcilable {
		t.Fatalf("missing block: %#v %v", a, err)
	}

	healthy := "bin/\n\n" + config.AtlasLocalGovernanceGitignoreBlock()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(healthy), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err = config.AnalyzeLocalOnlyGitignore(root, doc)
	if err != nil || a.State != config.GitignoreHealthy {
		t.Fatalf("healthy: %#v %v", a, err)
	}

	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("# >>> Atlas local governance >>>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err = config.AnalyzeLocalOnlyGitignore(root, doc)
	if err != nil || a.State != config.GitignoreMalformed || a.Reconcilable {
		t.Fatalf("malformed: %#v %v", a, err)
	}

	versioned := config.ProjectDocument{
		SourceControl: config.SourceControlPersist{GovernanceFiles: config.GovernanceFilesVersioned},
	}
	a, err = config.AnalyzeLocalOnlyGitignore(root, versioned)
	if err != nil || a.State != config.GitignoreNotApplicable {
		t.Fatalf("not applicable: %#v %v", a, err)
	}
}
