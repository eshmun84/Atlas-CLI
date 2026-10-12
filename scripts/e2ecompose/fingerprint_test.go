package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprintTree_DeterministicAndSymlinkSafe(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("world"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside-secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}

	a, err := fingerprintTree(root)
	if err != nil {
		t.Fatal(err)
	}
	b, err := fingerprintTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("non-deterministic fingerprint: %s vs %s", a, b)
	}

	// Changing symlink target content must not change fingerprint (not followed).
	if err := os.WriteFile(outside, []byte("outside-secret-CHANGED"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := fingerprintTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if c != a {
		t.Fatal("fingerprint followed symlink content")
	}

	// Changing link target string should change fingerprint.
	_ = os.Remove(filepath.Join(root, "link.txt"))
	if err := os.Symlink("/other/target", filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	d, err := fingerprintTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if d == a {
		t.Fatal("fingerprint ignored symlink retarget")
	}
}

func TestFingerprintTree_PropagatesReadErrors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nope.txt")
	if err := os.WriteFile(path, []byte("x"), 0o000); err != nil {
		t.Fatal(err)
	}
	// Make unreadable when possible (may still be readable as root / on some CI).
	if f, err := os.Open(path); err == nil {
		_ = f.Close()
		t.Skip("environment can still read 000 files")
	}
	if _, err := fingerprintTree(root); err == nil {
		t.Fatal("expected read error")
	}
}
