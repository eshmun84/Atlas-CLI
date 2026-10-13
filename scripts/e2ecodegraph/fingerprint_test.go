package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprintTree_Deterministic(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("alpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sub", "b.txt"), []byte("beta"), 0o600); err != nil {
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
	if a == "" || a != b {
		t.Fatalf("non-deterministic fingerprint: %q vs %q", a, b)
	}
}

func TestFingerprintTree_SymlinkNonFollow(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("SECRET-PAYLOAD"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "local.txt"), []byte("local"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	fp, err := fingerprintTree(root)
	if err != nil {
		t.Fatal(err)
	}
	// Changing the symlink target content must not change the fingerprint
	// because we hash the link target path, not followed content.
	if err := os.WriteFile(secret, []byte("MUTATED-SECRET"), 0o600); err != nil {
		t.Fatal(err)
	}
	fp2, err := fingerprintTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if fp != fp2 {
		t.Fatal("fingerprint followed symlink content")
	}
	// Changing the link target string itself must change the fingerprint.
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "other.txt"), link); err != nil {
		t.Fatal(err)
	}
	fp3, err := fingerprintTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if fp3 == fp {
		t.Fatal("fingerprint ignored symlink target change")
	}
}
