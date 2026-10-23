package memoryhealth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathWritable_RejectsEmptyAndRelative(t *testing.T) {
	for _, path := range []string{"", " ", "\t", "."} {
		if pathWritable(path) {
			t.Fatalf("path %q reported writable", path)
		}
	}
}

func TestPathWritable_MissingDoesNotCreate(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "absent")
	before := entries(t, dir)
	if pathWritable(missing) {
		t.Fatal("missing path reported writable")
	}
	if _, err := os.Lstat(missing); !os.IsNotExist(err) {
		t.Fatalf("missing path created: %v", err)
	}
	if after := entries(t, dir); len(after) != len(before) {
		t.Fatalf("entries before=%v after=%v", before, after)
	}
}

func TestPathWritable_FileAndDirectory(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "memory.db")
	if err := os.WriteFile(file, []byte("sqlite-placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}

	if !pathWritable(dir) {
		t.Fatal("writable directory rejected")
	}
	if !pathWritable(file) {
		t.Fatal("writable file rejected")
	}
	after, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("writability check modified file contents")
	}
	got := entries(t, dir)
	if len(got) != 1 || got[0] != "memory.db" {
		t.Fatalf("entries=%v; check created storage sidecars", got)
	}
}

func TestPathWritable_RejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, []byte("target"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if pathWritable(link) {
		t.Fatal("symlink reported writable")
	}
	if !pathWritable(target) {
		t.Fatal("symlink target became unwritable")
	}
}

func entries(t *testing.T, dir string) []string {
	t.Helper()
	list, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(list))
	for _, item := range list {
		names = append(names, item.Name())
	}
	return names
}
