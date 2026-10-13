package codegraphsource

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/eshmun84/Atlas-CLI/internal/contextcompose"
)

func TestAcceptProjectPath_RejectsOversizedWithHiddenTraversal(t *testing.T) {
	t.Parallel()
	oversized := strings.Repeat("x", contextcompose.MaxPathRunes) + "/../secret.go"
	if utf8.RuneCountInString(oversized) <= contextcompose.MaxPathRunes {
		t.Fatal("fixture must exceed MaxPathRunes")
	}
	// Truncation would drop "/../secret.go" and leave only safe 'x' runes.
	truncated := string([]rune(oversized)[:contextcompose.MaxPathRunes])
	if !safeProjectPath(truncated) {
		t.Fatalf("truncated fixture unexpectedly unsafe: %q", truncated)
	}
	if _, ok := acceptProjectPath(oversized); ok {
		t.Fatal("oversized path with hidden traversal must be rejected entirely")
	}
}

func TestAcceptProjectPath_RejectsAbsoluteAndTraversal(t *testing.T) {
	t.Parallel()
	cases := []string{
		"/etc/passwd",
		"../secret.go",
		"src/../../etc/passwd",
		`C:\windows\system32`,
		"https://evil.example/x.go",
		strings.Repeat("a", contextcompose.MaxPathRunes+1),
	}
	for _, c := range cases {
		if got, ok := acceptProjectPath(c); ok {
			t.Fatalf("accepted unsafe path %q as %q", c, got)
		}
	}
}

func TestAcceptProjectPath_AcceptsRelative(t *testing.T) {
	t.Parallel()
	got, ok := acceptProjectPath("  src/payments.go  ")
	if !ok || got != "src/payments.go" {
		t.Fatalf("got=(%q,%v)", got, ok)
	}
}
