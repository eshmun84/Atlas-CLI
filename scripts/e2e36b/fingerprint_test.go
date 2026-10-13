package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFingerprintTree_DeterministicAndSymlinkSafe(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("a.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Fatal(err)
	}
	fp1, err := fingerprintTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	fp2, err := fingerprintTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 != fp2 {
		t.Fatalf("fingerprint not deterministic")
	}
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello!"), 0o644); err != nil {
		t.Fatal(err)
	}
	fp3, err := fingerprintTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if fp1 == fp3 {
		t.Fatal("fingerprint ignored content change")
	}
}
