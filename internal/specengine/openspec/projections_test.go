package openspec_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func TestNeedsToolUpdate_MissingCursorMarkers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeReadyOpenSpecTree(t, root)
	if !openspec.NeedsToolUpdate(root, []string{"cursor"}) {
		t.Fatal("expected needs update when markers missing")
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "commands"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".cursor", "skills", "openspec-propose"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "commands", "opsx-propose.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".cursor", "skills", "openspec-propose", "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if openspec.NeedsToolUpdate(root, []string{"cursor"}) {
		t.Fatal("expected no update when markers present")
	}
}

func TestToolsForAdapters_Mapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"cursor"}, "cursor"},
		{[]string{"opencode"}, "opencode"},
		{[]string{"cursor", "opencode"}, "cursor,opencode"},
		{[]string{}, "none"},
	}
	for _, tc := range cases {
		if got := openspec.ToolsForAdapters(tc.in); got != tc.want {
			t.Fatalf("ToolsForAdapters(%v)=%q want %q", tc.in, got, tc.want)
		}
	}
}
