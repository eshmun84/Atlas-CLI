package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project/mutatelock"
	"github.com/eshmun84/Atlas-CLI/internal/version"
	"gopkg.in/yaml.v3"
)

func TestInstallAt_CleanInstall(t *testing.T) {
	homeDir, prefix := isolatedInstallEnv(t)
	src := writeBin(t, "v1")
	if err := installAt(src, prefix); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(got) != "v1" {
		t.Fatalf("binary=%q err=%v", got, err)
	}
	st := home.Inspect()
	if !home.Healthy(st) {
		t.Fatalf("home unhealthy: %#v", st)
	}
	if st.SchemaVersion != home.CurrentSchemaVersion {
		t.Fatalf("schema=%d", st.SchemaVersion)
	}
	assertPerm(t, homeDir, 0o700)
	assertPerm(t, filepath.Join(homeDir, "state", "home.yaml"), 0o600)
	assertPerm(t, filepath.Join(prefix, "bin", "atlas"), 0o755)
	assertNoStageLeft(t, filepath.Join(prefix, "bin"))
	doc := readState(t, homeDir)
	if doc.AtlasVersion != version.Version {
		t.Fatalf("home atlas_version=%q process=%q", doc.AtlasVersion, version.Version)
	}
}

func TestInstallAt_ReinstallAndUpdatePreserveProjects(t *testing.T) {
	homeDir, prefix := isolatedInstallEnv(t)
	if err := installAt(writeBin(t, "v1"), prefix); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(homeDir, "projects", "kept", "state", "local.yaml")
	if err := os.MkdirAll(filepath.Dir(project), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("user-project\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(homeDir, "notes.txt")
	if err := os.WriteFile(foreign, []byte("user-note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	created := readState(t, homeDir).CreatedAt
	if created == "" {
		t.Fatal("missing created_at")
	}
	if err := installAt(writeBin(t, "v2"), prefix); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(got) != "v2" {
		t.Fatalf("binary=%q err=%v", got, err)
	}
	if readState(t, homeDir).CreatedAt != created {
		t.Fatalf("created_at changed: %s -> %s", created, readState(t, homeDir).CreatedAt)
	}
	if data, err := os.ReadFile(project); err != nil || string(data) != "user-project\n" {
		t.Fatalf("project=%q err=%v", data, err)
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "user-note\n" {
		t.Fatalf("foreign=%q err=%v", data, err)
	}
	if readState(t, homeDir).AtlasVersion != version.Version {
		t.Fatal("version drifted on update")
	}
}

func TestInstallAt_MaterializeErrorRollsBackHome(t *testing.T) {
	saveSeams(t)
	homeDir, prefix := isolatedInstallEnv(t)
	old := writeBin(t, "old")
	if err := installAt(old, prefix); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(homeDir, "assets", "agents", "base.md")
	sentinel := []byte("USER-SENTINEL\n")
	if err := os.WriteFile(base, sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(homeDir, "projects", "kept", "note")
	if err := os.MkdirAll(filepath.Dir(project), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ensureHome = func(now time.Time) (home.EnsureResult, error) {
		result, err := home.EnsureAndMirror(now)
		if err != nil {
			return result, err
		}
		got, err := os.ReadFile(base)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Equal(got, sentinel) {
			t.Fatal("mirror did not rewrite the asset before the injected failure")
		}
		return result, errors.New("materialize boom")
	}
	err := installAt(writeBin(t, "new"), prefix)
	if err == nil || !strings.Contains(err.Error(), "materialize boom") {
		t.Fatalf("got %v", err)
	}
	got, err := os.ReadFile(base)
	if err != nil || !bytes.Equal(got, sentinel) {
		t.Fatalf("asset after rollback=%q err=%v", got, err)
	}
	bin, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(bin) != "old" {
		t.Fatalf("binary=%q err=%v", bin, err)
	}
	data, err := os.ReadFile(project)
	if err != nil || string(data) != "keep\n" {
		t.Fatalf("project=%q err=%v", data, err)
	}
	assertNoStageLeft(t, filepath.Join(prefix, "bin"))
}

func TestInstallAt_PublishErrorRollsBackHome(t *testing.T) {
	saveSeams(t)
	homeDir, prefix := isolatedInstallEnv(t)
	if err := installAt(writeBin(t, "old"), prefix); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(homeDir, "assets", "agents", "base.md")
	sentinel := []byte("USER-SENTINEL\n")
	if err := os.WriteFile(base, sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	stateBefore := mustRead(t, filepath.Join(homeDir, "state", "home.yaml"))
	renameFn = func(string, string) error { return errors.New("rename boom") }
	err := installAt(writeBin(t, "new"), prefix)
	if err == nil || !strings.Contains(err.Error(), "rename boom") {
		t.Fatalf("got %v", err)
	}
	got, err := os.ReadFile(base)
	if err != nil || !bytes.Equal(got, sentinel) {
		t.Fatalf("asset=%q err=%v", got, err)
	}
	stateAfter := mustRead(t, filepath.Join(homeDir, "state", "home.yaml"))
	if !bytes.Equal(stateBefore, stateAfter) {
		t.Fatal("home.yaml changed after failed publish")
	}
	bin, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(bin) != "old" {
		t.Fatalf("binary=%q err=%v", bin, err)
	}
	assertNoStageLeft(t, filepath.Join(prefix, "bin"))
}

func TestInstallAt_IncompatibleHomeUntouched(t *testing.T) {
	homeDir, prefix := isolatedInstallEnv(t)
	if err := installAt(writeBin(t, "old"), prefix); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(homeDir, "state", "home.yaml")
	raw := mustRead(t, statePath)
	updated := bytes.Replace(raw, []byte("schema_version: 1\n"), []byte("schema_version: 2\n"), 1)
	if bytes.Equal(raw, updated) {
		t.Fatal("schema stamp not found")
	}
	if err := os.WriteFile(statePath, updated, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	err = installAt(writeBin(t, "new"), prefix)
	if err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("got %v", err)
	}
	after, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) || before.Size() != after.Size() {
		t.Fatal("incompatible home.yaml was modified")
	}
	if !bytes.Equal(mustRead(t, statePath), updated) {
		t.Fatal("schema bytes changed")
	}
	bin, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(bin) != "old" {
		t.Fatalf("binary=%q err=%v", bin, err)
	}
}

func TestInstallAt_CorruptHomeUntouched(t *testing.T) {
	homeDir, prefix := isolatedInstallEnv(t)
	if err := os.MkdirAll(filepath.Join(homeDir, "state"), 0o700); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(homeDir, "state", "home.yaml")
	corrupt := []byte("schema_version: {\nbad\n")
	if err := os.WriteFile(statePath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	err = installAt(writeBin(t, "new"), prefix)
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("got %v", err)
	}
	after, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("corrupt home.yaml was modified")
	}
	if !bytes.Equal(mustRead(t, statePath), corrupt) {
		t.Fatal("corrupt bytes changed")
	}
	if _, err := os.Stat(filepath.Join(prefix, "bin", "atlas")); !os.IsNotExist(err) {
		t.Fatal("binary published despite corrupt home")
	}
}

func TestInstallAt_UserOwnedAssetDirNotRemoved(t *testing.T) {
	homeDir, prefix := isolatedInstallEnv(t)
	if err := installAt(writeBin(t, "old"), prefix); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(homeDir, "assets", "agents", "base.md")
	sentinel := []byte("USER-SENTINEL\n")
	if err := os.WriteFile(base, sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	blocked := filepath.Join(homeDir, "assets", "agents", "adapters", "cursor.md")
	if err := os.Remove(blocked); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(blocked, 0o700); err != nil {
		t.Fatal(err)
	}
	userFile := filepath.Join(blocked, "user.txt")
	if err := os.WriteFile(userFile, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := installAt(writeBin(t, "new"), prefix)
	if err == nil {
		t.Fatal("expected materialize failure")
	}
	if _, err := os.Stat(userFile); err != nil {
		t.Fatal("user file removed")
	}
	got, err := os.ReadFile(base)
	if err != nil || !bytes.Equal(got, sentinel) {
		t.Fatalf("asset=%q err=%v (install err=%v)", got, err, err)
	}
	bin, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(bin) != "old" {
		t.Fatalf("binary=%q err=%v", bin, err)
	}
}

func TestInstallAt_HomeLockBusy(t *testing.T) {
	homeDir, prefix := isolatedInstallEnv(t)
	if err := os.MkdirAll(homeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(homeDir, "marker")
	if err := os.WriteFile(marker, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	held, err := mutatelock.Acquire(mutatelock.Options{
		HomePath: homeDir,
		CacheDir: installLockCacheDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	err = installAt(writeBin(t, "v"), prefix)
	if err == nil || !errors.Is(err, mutatelock.ErrBusy) {
		t.Fatalf("expected busy, got %v", err)
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "keep\n" {
		t.Fatalf("marker=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(prefix, "bin", "atlas")); !os.IsNotExist(err) {
		t.Fatal("binary published while home lock was busy")
	}
}

func TestInstallAt_SyncBeforePublishRollsBack(t *testing.T) {
	saveSeams(t)
	homeDir, prefix := isolatedInstallEnv(t)
	if err := installAt(writeBin(t, "old"), prefix); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(homeDir, "assets", "agents", "base.md")
	sentinel := []byte("USER-SENTINEL\n")
	if err := os.WriteFile(base, sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	syncBeforePublish = func(string) error { return errors.New("sync before boom") }
	err := installAt(writeBin(t, "new"), prefix)
	if err == nil || !strings.Contains(err.Error(), "sync before boom") {
		t.Fatalf("got %v", err)
	}
	if errors.Is(err, errPublishDurability) {
		t.Fatal("pre-rename sync failure reported as post-commit")
	}
	got, err := os.ReadFile(base)
	if err != nil || !bytes.Equal(got, sentinel) {
		t.Fatalf("asset=%q err=%v", got, err)
	}
	bin, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(bin) != "old" {
		t.Fatalf("binary=%q err=%v", bin, err)
	}
}

func TestInstallAt_SyncAfterPublishKeepsStateAndReconciles(t *testing.T) {
	saveSeams(t)
	homeDir, prefix := isolatedInstallEnv(t)
	if err := installAt(writeBin(t, "old"), prefix); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(homeDir, "assets", "agents", "base.md")
	sentinel := []byte("USER-SENTINEL\n")
	if err := os.WriteFile(base, sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(homeDir, "projects", "kept", "note")
	if err := os.MkdirAll(filepath.Dir(project), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, []byte("keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	syncAfterPublish = func(string) error { return errors.New("sync after boom") }
	err := installAt(writeBin(t, "new"), prefix)
	if !errors.Is(err, errPublishDurability) || !strings.Contains(err.Error(), "sync after boom") {
		t.Fatalf("got %v", err)
	}
	got, err := os.ReadFile(base)
	if err != nil || bytes.Equal(got, sentinel) {
		t.Fatalf("home was rolled back or unreadable: %q err=%v", got, err)
	}
	bin, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(bin) != "new" {
		t.Fatalf("binary=%q err=%v", bin, err)
	}
	if data, err := os.ReadFile(project); err != nil || string(data) != "keep\n" {
		t.Fatalf("project=%q err=%v", data, err)
	}
	syncAfterPublish = syncDir
	if err := installAt(writeBin(t, "newer"), prefix); err != nil {
		t.Fatal(err)
	}
	bin, err = os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(bin) != "newer" {
		t.Fatalf("reconciled binary=%q err=%v", bin, err)
	}
	if !home.Healthy(home.Inspect()) {
		t.Fatal("home not healthy after reconcile")
	}
	if data, err := os.ReadFile(project); err != nil || string(data) != "keep\n" {
		t.Fatalf("project after reconcile=%q err=%v", data, err)
	}
}

func isolatedInstallEnv(t *testing.T) (homeDir, prefix string) {
	t.Helper()
	root := t.TempDir()
	homeDir = filepath.Join(root, "home")
	prefix = filepath.Join(root, "prefix")
	t.Setenv(home.EnvAtlasHome, homeDir)
	prev := installLockCacheDir
	installLockCacheDir = filepath.Join(root, "locks")
	t.Cleanup(func() { installLockCacheDir = prev })
	return homeDir, prefix
}

func writeBin(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func readState(t *testing.T, homeDir string) home.StateDocument {
	t.Helper()
	var doc home.StateDocument
	if err := yaml.Unmarshal(mustRead(t, filepath.Join(homeDir, "state", "home.yaml")), &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("%s perm=%o want=%o", path, info.Mode().Perm(), want)
	}
}
