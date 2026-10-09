package mcp_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
)

func TestNewBackupStamp_UniqueWithinSameSecond(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 8, 12, 0, 0, 123, time.UTC)
	a := mcp.NewBackupStamp(now)
	b := mcp.NewBackupStamp(now)
	if a == b {
		t.Fatalf("stamps collided: %q", a)
	}
	home := t.TempDir()
	projectID := "proj-backup"
	pathA, err := mcp.WriteHomeBackup(home, projectID, a, "cursor", []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	pathB, err := mcp.WriteHomeBackup(home, projectID, b, "cursor", []byte(`{"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if pathA == pathB {
		t.Fatal("backup paths collided")
	}
	if filepath.Dir(pathA) == filepath.Dir(pathB) {
		t.Fatal("backup dirs should differ per stamp")
	}
}
