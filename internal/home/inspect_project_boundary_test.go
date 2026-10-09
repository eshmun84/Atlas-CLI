package home_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func setupPresentProject(t *testing.T) (homeDir, pid string) {
	t.Helper()
	homeDir = t.TempDir()
	root := t.TempDir()
	var err error
	pid, err = home.ProjectID(root, "boundary")
	if err != nil {
		t.Fatal(err)
	}
	if err := home.EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	if err := home.WriteProjectIdentity(homeDir, pid, "boundary", root, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	local := home.ProjectLocalState{
		SchemaVersion:           1,
		ContextEconomyUpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := home.WriteProjectLocalState(homeDir, pid, local); err != nil {
		t.Fatal(err)
	}
	ctxFile := filepath.Join(home.ProjectContextDir(homeDir, pid), "marker.txt")
	if err := os.WriteFile(ctxFile, []byte("ctx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return homeDir, pid
}

func TestInspectProject_HealthyPathsUnchanged(t *testing.T) {
	homeDir, pid := setupPresentProject(t)
	st := home.InspectProject(homeDir, pid)
	if !st.Present || !st.IdentityPresent || !st.LocalStatePresent || !st.ContextPresent {
		t.Fatalf("want healthy present flags: %#v", st)
	}
	if len(st.IntegrityErrors) != 0 {
		t.Fatalf("unexpected integrity errors: %#v", st.IntegrityErrors)
	}
	if st.Identity.ProjectName != "boundary" {
		t.Fatalf("identity = %#v", st.Identity)
	}
}

func TestInspectProject_ContextDirSymlink_NotPresent(t *testing.T) {
	homeDir, pid := setupPresentProject(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "evil.txt"), []byte("EXTERNAL\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := home.ProjectContextDir(homeDir, pid)
	if err := os.RemoveAll(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, ctx); err != nil {
		t.Fatal(err)
	}
	st := home.InspectProject(homeDir, pid)
	if st.ContextPresent {
		t.Fatal("context symlink must not count as present")
	}
	if len(st.IntegrityErrors) == 0 {
		t.Fatal("expected integrity error for context symlink")
	}
	got, _ := os.ReadFile(filepath.Join(outside, "evil.txt"))
	if string(got) != "EXTERNAL\n" {
		t.Fatal("external context mutated")
	}
}

func TestInspectProject_BackupsDirSymlink_NotPresent(t *testing.T) {
	homeDir, pid := setupPresentProject(t)
	outside := t.TempDir()
	bak := home.ProjectBackupsDir(homeDir, pid)
	if err := os.RemoveAll(bak); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, bak); err != nil {
		t.Fatal(err)
	}
	st := home.InspectProject(homeDir, pid)
	if st.BackupsPresent {
		t.Fatal("backups symlink must not count as present")
	}
	if !hasIntegrity(st, "backups") {
		t.Fatalf("expected backups integrity error: %#v", st.IntegrityErrors)
	}
}

func TestInspectProject_IdentityLeafSymlink_NotPresent(t *testing.T) {
	homeDir, pid := setupPresentProject(t)
	outside := t.TempDir()
	ext := filepath.Join(outside, "identity.yaml")
	if err := os.WriteFile(ext, []byte("schema_version: 1\nproject_name: EXTERNAL\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	idPath := home.ProjectIdentityPath(homeDir, pid)
	if err := os.Remove(idPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, idPath); err != nil {
		t.Fatal(err)
	}
	st := home.InspectProject(homeDir, pid)
	if st.IdentityPresent {
		t.Fatal("identity symlink must not count as present")
	}
	if st.Identity.ProjectName == "EXTERNAL" {
		t.Fatal("external identity must never be parsed")
	}
	if !hasIntegrity(st, "identity") {
		t.Fatalf("expected identity integrity error: %#v", st.IntegrityErrors)
	}
}

func TestInspectProject_LocalStateLeafSymlink_NotPresent(t *testing.T) {
	homeDir, pid := setupPresentProject(t)
	outside := t.TempDir()
	ext := filepath.Join(outside, "local.yaml")
	if err := os.WriteFile(ext, []byte("schema_version: 1\nproject_id: forged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	localPath := home.ProjectLocalStatePath(homeDir, pid)
	if err := os.Remove(localPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(ext, localPath); err != nil {
		t.Fatal(err)
	}
	st := home.InspectProject(homeDir, pid)
	if st.LocalStatePresent {
		t.Fatal("local-state symlink must not count as present")
	}
	if !hasIntegrity(st, "local") {
		t.Fatalf("expected local-state integrity error: %#v", st.IntegrityErrors)
	}
}

func hasIntegrity(st home.ProjectStatus, needle string) bool {
	for _, e := range st.IntegrityErrors {
		if strings.Contains(strings.ToLower(e), needle) {
			return true
		}
	}
	return false
}
