package openspec_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func writeReadyOpenSpecTree(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{
		"openspec/specs",
		"openspec/changes/archive",
	} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "openspec", "config.yaml"), []byte("schema: spec-driven\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestClassifyProject_RealInitShapeReady(t *testing.T) {
	root := t.TempDir()
	writeReadyOpenSpecTree(t, root)
	st, reason := openspec.ClassifyProject(root)
	if st != openspec.ProjectReady || reason != "" {
		t.Fatalf("ready: %s (%s)", st, reason)
	}
	assess, err := openspec.AssessLayout(root)
	if err != nil || !assess.Valid {
		t.Fatalf("ready assess: %#v %v", assess, err)
	}
}

func TestClassifyProject_ConfigAndChangesOnlyMalformed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "openspec", "config.yaml"), []byte("schema: spec-driven\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, reason := openspec.ClassifyProject(root)
	if st != openspec.ProjectMalformed {
		t.Fatalf("got %s (%s), want malformed", st, reason)
	}
}

func TestClassifyProject_MissingSpecsMalformed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "openspec", "config.yaml"), []byte("schema: spec-driven\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, reason := openspec.ClassifyProject(root)
	if st != openspec.ProjectMalformed || reason == "" {
		t.Fatalf("got %s (%s), want malformed missing specs", st, reason)
	}
}

func TestClassifyProject_MissingArchiveMalformed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "openspec", "config.yaml"), []byte("schema: spec-driven\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, reason := openspec.ClassifyProject(root)
	if st != openspec.ProjectMalformed || reason == "" {
		t.Fatalf("got %s (%s), want malformed missing archive", st, reason)
	}
}

func TestClassifyProject_LegacyMkdir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes"), 0o755); err != nil {
		t.Fatal(err)
	}
	st, reason := openspec.ClassifyProject(root)
	if st != openspec.ProjectLegacyMkdir {
		t.Fatalf("legacy: %s (%s)", st, reason)
	}
	assess, err := openspec.AssessLayout(root)
	if err != nil || !assess.Incompatible || assess.Valid {
		t.Fatalf("legacy must not be Valid: %#v", assess)
	}
}

func TestClassifyProject_Absent(t *testing.T) {
	st, _ := openspec.ClassifyProject(t.TempDir())
	if st != openspec.ProjectAbsent {
		t.Fatalf("absent: %s", st)
	}
}

func TestClassifyProject_SymlinkSurfacesMalformed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink cases use Unix semantics")
	}
	cases := []struct {
		name string
		prep func(t *testing.T, root string)
	}{
		{
			name: "config symlink",
			prep: func(t *testing.T, root string) {
				writeReadyOpenSpecTree(t, root)
				cfg := filepath.Join(root, "openspec", "config.yaml")
				if err := os.Remove(cfg); err != nil {
					t.Fatal(err)
				}
				target := filepath.Join(root, "openspec", "config-real.yaml")
				if err := os.WriteFile(target, []byte("schema: spec-driven\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, cfg); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "specs symlink",
			prep: func(t *testing.T, root string) {
				writeReadyOpenSpecTree(t, root)
				specs := filepath.Join(root, "openspec", "specs")
				if err := os.RemoveAll(specs); err != nil {
					t.Fatal(err)
				}
				real := filepath.Join(root, "openspec", "specs-real")
				if err := os.MkdirAll(real, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, specs); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "archive symlink",
			prep: func(t *testing.T, root string) {
				writeReadyOpenSpecTree(t, root)
				arch := filepath.Join(root, "openspec", "changes", "archive")
				if err := os.RemoveAll(arch); err != nil {
					t.Fatal(err)
				}
				real := filepath.Join(root, "openspec", "archive-real")
				if err := os.MkdirAll(real, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, arch); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "openspec root symlink",
			prep: func(t *testing.T, root string) {
				real := filepath.Join(root, "openspec-real")
				if err := os.MkdirAll(filepath.Join(real, "changes", "archive"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(filepath.Join(real, "specs"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(real, "config.yaml"), []byte("schema: spec-driven\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(real, filepath.Join(root, "openspec")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			tc.prep(t, root)
			st, reason := openspec.ClassifyProject(root)
			if st != openspec.ProjectMalformed {
				t.Fatalf("got %s (%s), want malformed", st, reason)
			}
		})
	}
}

func TestEnsureBootstrap_Removed(t *testing.T) {
	_, err := openspec.EnsureBootstrap(t.TempDir())
	if err == nil {
		t.Fatal("expected removal error")
	}
}
