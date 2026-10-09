package home

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

func TestInspectProjectDataPresence(t *testing.T) {
	homeDir := t.TempDir()
	pid := "presence-1"

	present, err := InspectProjectDataPresence(homeDir, pid)
	if err != nil || present {
		t.Fatalf("missing: present=%v err=%v", present, err)
	}

	root := filepath.Join(homeDir, "projects", pid)
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	present, err = InspectProjectDataPresence(homeDir, pid)
	if err != nil || present {
		t.Fatalf("empty: present=%v err=%v", present, err)
	}

	if err := os.WriteFile(filepath.Join(root, "marker"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	present, err = InspectProjectDataPresence(homeDir, pid)
	if err != nil || !present {
		t.Fatalf("populated: present=%v err=%v", present, err)
	}

	prev := lstatFn
	lstatFn = func(name string) (os.FileInfo, error) {
		if filepath.Clean(name) == filepath.Clean(root) {
			return nil, errors.New("injected lstat failure")
		}
		return os.Lstat(name)
	}
	t.Cleanup(func() { lstatFn = prev })
	present, err = InspectProjectDataPresence(homeDir, pid)
	if err == nil || present {
		t.Fatalf("inspection failure: present=%v err=%v", present, err)
	}
}

func TestCaptureProjectLayoutSnapshot_FailClosedIdentity(t *testing.T) {
	homeDir := t.TempDir()
	pid := "snap-id"
	if err := EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	if err := WriteProjectIdentity(homeDir, pid, "demo", t.TempDir(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	prev := captureFileSnapshotFn
	captureFileSnapshotFn = func(root, rel string) (fsafety.FileSnapshot, error) {
		if strings.Contains(rel, "identity.yaml") {
			return fsafety.FileSnapshot{}, errors.New("injected identity read error")
		}
		return fsafety.CaptureFileSnapshot(root, rel)
	}
	t.Cleanup(func() { captureFileSnapshotFn = prev })

	_, err := CaptureProjectLayoutSnapshot(homeDir, pid)
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected identity snapshot error, got %v", err)
	}
}

func TestCaptureProjectLayoutSnapshot_FailClosedDirProbe(t *testing.T) {
	homeDir := t.TempDir()
	pid := "snap-dir"
	if err := os.MkdirAll(filepath.Join(homeDir, "projects", pid), 0o700); err != nil {
		t.Fatal(err)
	}
	prev := lstatFn
	lstatFn = func(name string) (os.FileInfo, error) {
		if strings.HasSuffix(filepath.ToSlash(name), "/"+pid+"/state") {
			return nil, errors.New("injected dir probe error")
		}
		return os.Lstat(name)
	}
	t.Cleanup(func() { lstatFn = prev })
	_, err := CaptureProjectLayoutSnapshot(homeDir, pid)
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("expected dir probe error, got %v", err)
	}
}

func TestCaptureMirrorSnapshot_FailClosedState(t *testing.T) {
	homeDir := t.TempDir()
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prev := captureFileSnapshotFn
	captureFileSnapshotFn = func(root, rel string) (fsafety.FileSnapshot, error) {
		if strings.Contains(rel, "home.yaml") {
			return fsafety.FileSnapshot{}, errors.New("injected state snapshot error")
		}
		return fsafety.CaptureFileSnapshot(root, rel)
	}
	t.Cleanup(func() { captureFileSnapshotFn = prev })
	_, err := CaptureMirrorSnapshot(homeDir)
	if err == nil || !strings.Contains(err.Error(), "state snapshot") {
		t.Fatalf("expected state snapshot error, got %v", err)
	}
}

func TestRestoreProjectLayoutSnapshot_ChmodFailure(t *testing.T) {
	homeDir := t.TempDir()
	pid := "chmod-rb"
	if err := EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	snap, err := CaptureProjectLayoutSnapshot(homeDir, pid)
	if err != nil {
		t.Fatal(err)
	}
	// Change mode so restore must chmod (same-mode short-circuit would skip).
	root := ProjectRoot(homeDir, pid)
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatal(err)
	}
	prev := chmodFn
	chmodFn = func(name string, mode os.FileMode) error {
		return errors.New("injected chmod failure")
	}
	t.Cleanup(func() { chmodFn = prev })
	err = RestoreProjectLayoutSnapshot(snap)
	if err == nil || !strings.Contains(err.Error(), "chmod") {
		t.Fatalf("expected chmod restore error, got %v", err)
	}
}

func TestEnsureProjectLayout_MissingExtrasOK(t *testing.T) {
	homeDir := t.TempDir()
	pid := "extras-missing"
	if err := EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"codegraph", "mcp"} {
		if _, err := os.Lstat(filepath.Join(homeDir, "projects", pid, extra)); !os.IsNotExist(err) {
			t.Fatalf("%s should remain absent: %v", extra, err)
		}
	}
}

func TestEnsureProjectLayout_CodegraphChmodFailure(t *testing.T) {
	homeDir := t.TempDir()
	pid := "extras-chmod-cg"
	if err := EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	cg := filepath.Join(homeDir, "projects", pid, "codegraph")
	if err := os.MkdirAll(cg, 0o755); err != nil {
		t.Fatal(err)
	}
	prev := chmodFn
	chmodFn = func(name string, mode os.FileMode) error {
		if filepath.Clean(name) == filepath.Clean(cg) {
			return errors.New("injected codegraph chmod failure")
		}
		return os.Chmod(name, mode)
	}
	t.Cleanup(func() { chmodFn = prev })
	err := EnsureProjectLayout(homeDir, pid)
	if err == nil || !strings.Contains(err.Error(), "codegraph") {
		t.Fatalf("expected codegraph chmod error, got %v", err)
	}
}

func TestEnsureProjectLayout_MCPChmodFailure(t *testing.T) {
	homeDir := t.TempDir()
	pid := "extras-chmod-mcp"
	if err := EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	mcpDir := filepath.Join(homeDir, "projects", pid, "mcp")
	if err := os.MkdirAll(mcpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prev := chmodFn
	chmodFn = func(name string, mode os.FileMode) error {
		if filepath.Clean(name) == filepath.Clean(mcpDir) {
			return errors.New("injected mcp chmod failure")
		}
		return os.Chmod(name, mode)
	}
	t.Cleanup(func() { chmodFn = prev })
	err := EnsureProjectLayout(homeDir, pid)
	if err == nil || !strings.Contains(err.Error(), "mcp") {
		t.Fatalf("expected mcp chmod error, got %v", err)
	}
}

func TestEnsureProjectLayout_SymlinkExtrasBlock(t *testing.T) {
	homeDir := t.TempDir()
	pid := "extras-symlink"
	if err := EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	for _, extra := range []string{"codegraph", "mcp"} {
		link := filepath.Join(homeDir, "projects", pid, extra)
		if err := os.Symlink(target, link); err != nil {
			t.Fatal(err)
		}
		err := EnsureProjectLayout(homeDir, pid)
		if err == nil || !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("%s: expected symlink block, got %v", extra, err)
		}
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWriteProjectIdentity_CorruptIdentityUnchanged(t *testing.T) {
	homeDir := t.TempDir()
	root := t.TempDir()
	pid, err := ProjectID(root, "corrupt-id")
	if err != nil {
		t.Fatal(err)
	}
	if err := EnsureProjectLayout(homeDir, pid); err != nil {
		t.Fatal(err)
	}
	path := ProjectIdentityPath(homeDir, pid)
	corrupt := []byte("schema_version: [\nnot-yaml\n")
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	err = WriteProjectIdentity(homeDir, pid, "corrupt-id", root, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "identity") {
		t.Fatalf("expected identity load error, got %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("identity bytes mutated:\nbefore=%q\nafter=%q", corrupt, got)
	}
}

func TestEnsureAndMirror_CorruptStateUnchanged(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv(EnvAtlasHome, homeDir)
	if err := os.MkdirAll(filepath.Join(homeDir, "state"), 0o700); err != nil {
		t.Fatal(err)
	}
	statePath := StateFile(homeDir)
	corrupt := []byte("schema_version: {\nbad\n")
	if err := os.WriteFile(statePath, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := EnsureAndMirror(time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("expected state load error, got %v", err)
	}
	got, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("state bytes mutated:\nbefore=%q\nafter=%q", corrupt, got)
	}
}
