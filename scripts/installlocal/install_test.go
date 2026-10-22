package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInstall_SuccessPublishesAfterHome(t *testing.T) {
	prefix := t.TempDir()
	binSrc := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(binSrc, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var materialized bool
	err := Install(binSrc, prefix, Hooks{Materialize: func() error {
		materialized = true
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !materialized {
		t.Fatal("expected materialize before publish")
	}
	final := filepath.Join(prefix, "bin", "atlas")
	data, err := os.ReadFile(final)
	if err != nil || !strings.Contains(string(data), "echo ok") {
		t.Fatalf("final binary = %q err=%v", data, err)
	}
	info, err := os.Stat(final)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%o", info.Mode().Perm())
	}
	assertNoStageLeft(t, filepath.Join(prefix, "bin"))
}

func TestInstall_HomeFailurePreservesExistingBinary(t *testing.T) {
	prefix := t.TempDir()
	destDir := filepath.Join(prefix, "bin")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(destDir, "atlas")
	old := []byte("#!/bin/sh\necho old\n")
	if err := os.WriteFile(final, old, 0o755); err != nil {
		t.Fatal(err)
	}

	binSrc := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(binSrc, []byte("#!/bin/sh\necho new\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := Install(binSrc, prefix, Hooks{Materialize: func() error {
		return errors.New("home boom")
	}})
	if err == nil || !strings.Contains(err.Error(), "home boom") {
		t.Fatalf("expected home failure, got %v", err)
	}
	got, err := os.ReadFile(final)
	if err != nil || string(got) != string(old) {
		t.Fatalf("final binary mutated: %q err=%v", got, err)
	}
	assertNoStageLeft(t, destDir)
}

func TestInstall_SecondInstallReplacesAfterHealthyHome(t *testing.T) {
	prefix := t.TempDir()
	writeBin := func(name, body string) string {
		t.Helper()
		p := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if err := Install(writeBin("a1", "v1"), prefix, Hooks{Materialize: func() error { return nil }}); err != nil {
		t.Fatal(err)
	}
	if err := Install(writeBin("a2", "v2"), prefix, Hooks{Materialize: func() error { return nil }}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(got) != "v2" {
		t.Fatalf("got %q err=%v", got, err)
	}
	assertNoStageLeft(t, filepath.Join(prefix, "bin"))
}

func TestInstall_PublishErrorPreservesBinary(t *testing.T) {
	saveSeams(t)
	prefix := t.TempDir()
	final := filepath.Join(prefix, "bin", "atlas")
	old := []byte("old-bin")
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(final, old, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "new")
	if err := os.WriteFile(src, []byte("new-bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	renameFn = func(string, string) error { return errors.New("rename boom") }
	var rolled bool
	err := Install(src, prefix, Hooks{
		Materialize: func() error { return nil },
		Rollback:    func() error { rolled = true; return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "rename boom") {
		t.Fatalf("got %v", err)
	}
	if errors.Is(err, errPublishDurability) {
		t.Fatal("rename failure was reported as a post-commit durability error")
	}
	if !rolled {
		t.Fatal("expected rollback after publish failure")
	}
	got, err := os.ReadFile(final)
	if err != nil || string(got) != string(old) {
		t.Fatalf("binary=%q err=%v", got, err)
	}
	assertNoStageLeft(t, filepath.Dir(final))
}

func TestInstall_SyncsBeforePublish(t *testing.T) {
	saveSeams(t)
	prefix := t.TempDir()
	src := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(src, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	synced := false
	syncFile = func(f *os.File) error {
		synced = true
		return f.Sync()
	}
	renameFn = func(a, b string) error {
		if !synced {
			t.Fatal("rename ran before sync")
		}
		return os.Rename(a, b)
	}
	if err := Install(src, prefix, Hooks{Materialize: func() error { return nil }}); err != nil {
		t.Fatal(err)
	}
}

func TestInstall_PublishSyncSucceedsInOrder(t *testing.T) {
	saveSeams(t)
	var order []string
	syncFile = func(f *os.File) error {
		order = append(order, "file")
		return f.Sync()
	}
	syncBeforePublish = func(dir string) error {
		order = append(order, "before")
		return syncDir(dir)
	}
	renameFn = func(a, b string) error {
		order = append(order, "rename")
		return os.Rename(a, b)
	}
	syncAfterPublish = func(dir string) error {
		order = append(order, "after")
		return syncDir(dir)
	}
	prefix := t.TempDir()
	src := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(src, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Install(src, prefix, Hooks{Materialize: func() error {
		order = append(order, "materialize")
		return nil
	}}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "file,materialize,before,rename,after" {
		t.Fatalf("order = %v", order)
	}
	got, err := os.ReadFile(filepath.Join(prefix, "bin", "atlas"))
	if err != nil || string(got) != "bin" {
		t.Fatalf("binary=%q err=%v", got, err)
	}
}

func TestInstall_RefusesDirectoryCollision(t *testing.T) {
	prefix := t.TempDir()
	final := filepath.Join(prefix, "bin", "atlas")
	if err := os.MkdirAll(final, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(src, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	called := false
	err := Install(src, prefix, Hooks{Materialize: func() error {
		called = true
		return nil
	}})
	if err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("got %v", err)
	}
	if called {
		t.Fatal("materialize ran despite collision")
	}
	info, err := os.Lstat(final)
	if err != nil || !info.IsDir() {
		t.Fatalf("directory target changed: %v", err)
	}
}

func TestInstall_RefusesSymlinkCollision(t *testing.T) {
	prefix := t.TempDir()
	dest := filepath.Join(prefix, "bin")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte("user"), 0o644); err != nil {
		t.Fatal(err)
	}
	final := filepath.Join(dest, "atlas")
	if err := os.Symlink(target, final); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(src, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := Install(src, prefix, Hooks{Materialize: func() error { return nil }})
	if err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("got %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "user" {
		t.Fatalf("symlink target changed: %q %v", got, err)
	}
	if info, err := os.Lstat(final); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("symlink was replaced")
	}
}

func TestInstall_ConcurrentPrefixRejected(t *testing.T) {
	prefix := t.TempDir()
	src := filepath.Join(t.TempDir(), "atlas")
	if err := os.WriteFile(src, []byte("bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Install(src, prefix, Hooks{Materialize: func() error {
			close(started)
			<-release
			return nil
		}})
	}()
	select {
	case <-started:
	case err := <-done:
		t.Fatalf("first install ended early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for first install")
	}
	dest := filepath.Join(prefix, "bin")
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	var active string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), stagePrefix) {
			active = e.Name()
		}
	}
	if active == "" {
		t.Fatal("expected a live staged binary")
	}
	err = Install(src, prefix, Hooks{Materialize: func() error {
		t.Fatal("second install materialized")
		return nil
	}})
	if err == nil || !errors.Is(err, errInstallBusy) {
		t.Fatalf("expected busy, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, active)); err != nil {
		t.Fatalf("active stage removed: %v", err)
	}
	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("first install did not finish")
	}
	assertNoStageLeft(t, dest)
}

func TestMakefile_VersionLdflagsShared(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repoRoot(t), "Makefile")))
	if !strings.Contains(text, "VERSION_LDFLAGS :=") {
		t.Fatal("VERSION_LDFLAGS missing")
	}
	if !strings.Contains(text, `github.com/eshmun84/Atlas-CLI/internal/version.Version=$(VERSION)`) {
		t.Fatal("version ldflag missing")
	}
	if !strings.Contains(text, "go build $(VERSION_LDFLAGS) -o $(BINARY) ./cmd/atlas") {
		t.Fatal("build does not use VERSION_LDFLAGS")
	}
	if !strings.Contains(text, "go run $(VERSION_LDFLAGS) ./scripts/installlocal") {
		t.Fatal("installlocal does not use VERSION_LDFLAGS")
	}
}

func TestSourceVersionUnchanged(t *testing.T) {
	text := string(mustRead(t, filepath.Join(repoRoot(t), "internal", "version", "version.go")))
	if !strings.Contains(text, `var Version = "0.1.0"`) {
		t.Fatal("version.Version source default changed")
	}
}

func assertNoStageLeft(t *testing.T, destDir string) {
	t.Helper()
	entries, err := os.ReadDir(destDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), stagePrefix) {
			t.Fatalf("leftover staged binary: %s", e.Name())
		}
	}
}

func saveSeams(t *testing.T) {
	t.Helper()
	prevRename := renameFn
	prevSync := syncFile
	prevBefore := syncBeforePublish
	prevAfter := syncAfterPublish
	prevEnsure := ensureHome
	prevInspect := stageAfterInspect
	prevUnlink := stageBeforeUnlink
	t.Cleanup(func() {
		renameFn = prevRename
		syncFile = prevSync
		syncBeforePublish = prevBefore
		syncAfterPublish = prevAfter
		ensureHome = prevEnsure
		stageAfterInspect = prevInspect
		stageBeforeUnlink = prevUnlink
	})
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
