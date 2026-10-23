//go:build windows

package memoryhealth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathWritable_ReadOnlyFileAttribute(t *testing.T) {
	file := filepath.Join(t.TempDir(), "memory.db")
	if err := os.WriteFile(file, []byte("db"), 0o644); err != nil {
		t.Fatal(err)
	}
	// On Windows this sets FILE_ATTRIBUTE_READONLY and clears the write bits.
	if err := os.Chmod(file, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(file, 0o644) })
	if pathWritable(file) {
		t.Fatal("read-only file attribute reported writable")
	}
}

func TestPathWritable_DirectoryReadOnlyAttributeIsNotDenial(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "storage")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if !pathWritable(dir) {
		t.Fatal("directory read-only attribute was treated as a write denial")
	}
}
