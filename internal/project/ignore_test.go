package project_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project"
)

func TestShouldIgnoreDir_CommonTrees(t *testing.T) {
	t.Parallel()
	for _, name := range []string{".git", "vendor", "node_modules", "dist", "build", ".cache"} {
		if !project.ShouldIgnoreDir(name) {
			t.Fatalf("expected ignore dir %q", name)
		}
	}
	if project.ShouldIgnoreDir("cmd") || project.ShouldIgnoreDir("internal") {
		t.Fatal("product dirs must not be ignored")
	}
}

func TestShouldIgnoreRel_BackupsAndExts(t *testing.T) {
	t.Parallel()
	if !project.ShouldIgnoreRel(".atlas/backups/x") {
		t.Fatal("atlas backups must be ignored")
	}
	if !project.ShouldIgnoreRel("pkg/foo.log") {
		t.Fatal(".log must be ignored")
	}
	if project.ShouldIgnoreRel("main.go") {
		t.Fatal("main.go must be relevant")
	}
}

func TestShouldSkipSourceDir_RuntimeOwned(t *testing.T) {
	t.Parallel()
	for _, name := range []string{".atlas", ".codegraph", ".cursor", ".opencode", ".agents", ".claude", "vendor"} {
		if !project.ShouldSkipSourceDir(name, name) {
			t.Fatalf("source walk must skip %q", name)
		}
	}
}

func TestIsRelevantSourceFile(t *testing.T) {
	t.Parallel()
	if !project.IsRelevantSourceFile("main.go", 10) {
		t.Fatal("main.go relevant")
	}
	if project.IsRelevantSourceFile("vendor/x.go", 10) {
		t.Fatal("vendor irrelevant")
	}
	if project.IsRelevantSourceFile(".atlas/config.yaml", 10) {
		t.Fatal(".atlas irrelevant")
	}
	if project.IsRelevantSourceFile(".codegraph/changes.journal", 10) {
		t.Fatal(".codegraph irrelevant")
	}
	if project.IsRelevantSourceFile("AGENTS.md", 10) {
		t.Fatal("AGENTS.md irrelevant for source fingerprint")
	}
	if project.IsRelevantSourceFile("big.bin", project.MaxSourceFileBytes+1) {
		t.Fatal("oversized irrelevant")
	}
}
