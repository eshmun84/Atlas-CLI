package mutatelock_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
)

func TestCanonicalTarget_AliasesSamePath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "ws")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		t.Fatal(err)
	}
	viaDot := filepath.Join(root, ".", "ws")
	a, err := mutatelock.CanonicalTarget(abs)
	if err != nil {
		t.Fatal(err)
	}
	b, err := mutatelock.CanonicalTarget(viaDot)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("aliases differ: %q vs %q", a, b)
	}
}

func TestCanonicalTarget_MissingHomeSameAncestor(t *testing.T) {
	root := t.TempDir()
	a := filepath.Join(root, "atlas-home")
	b := filepath.Join(root, "other", "..", "atlas-home")
	ca, err := mutatelock.CanonicalTarget(a)
	if err != nil {
		t.Fatal(err)
	}
	cb, err := mutatelock.CanonicalTarget(b)
	if err != nil {
		t.Fatal(err)
	}
	if ca != cb {
		t.Fatalf("missing home aliases differ: %q vs %q", ca, cb)
	}
}
