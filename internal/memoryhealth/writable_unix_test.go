//go:build unix

package memoryhealth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathWritable_ReadOnlyFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses Unix mode checks")
	}
	file := filepath.Join(t.TempDir(), "memory.db")
	if err := os.WriteFile(file, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(file, 0o644) })
	if pathWritable(file) {
		t.Fatal("read-only file reported writable")
	}
}

func TestPathWritable_ReadOnlyDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses Unix mode checks")
	}
	dir := filepath.Join(t.TempDir(), "storage")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if pathWritable(dir) {
		t.Fatal("read-only directory reported writable")
	}
}
