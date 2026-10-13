package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// withEnv returns a copy of env with overlay keys replacing any existing entries.
func withEnv(env []string, overlay map[string]string) []string {
	out := make([]string, 0, len(env)+len(overlay))
	skip := map[string]struct{}{}
	for k := range overlay {
		skip[k] = struct{}{}
	}
	for _, kv := range env {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, drop := skip[key]; drop {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range overlay {
		out = append(out, k+"="+v)
	}
	return out
}

// runWorkspace is a uniquely created E2E sandbox owned by one process run.
// Cleanup removes only RunDir, and only when this process created it.
type runWorkspace struct {
	Root      string
	RunDir    string
	Project   string
	AtlasHome string
	Evidence  string
	created   bool
}

// validateE2ERoot accepts an existing, real (non-symlink) directory that will
// hold uniquely named run workspaces. It never creates or deletes the root.
func validateE2ERoot(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("E2E root is empty")
	}
	if raw == "." || raw == ".." {
		return "", fmt.Errorf("E2E root %q is ambiguous", raw)
	}
	cleaned := filepath.Clean(raw)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return "", fmt.Errorf("E2E root abs: %w", err)
	}
	abs = filepath.Clean(abs)
	if abs == string(filepath.Separator) {
		return "", fmt.Errorf("E2E root must not be filesystem root")
	}
	if homeDir, err := os.UserHomeDir(); err == nil {
		homeDir = filepath.Clean(homeDir)
		if abs == homeDir {
			return "", fmt.Errorf("E2E root must not be the user home directory")
		}
	}
	info, err := os.Lstat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("E2E root does not exist: %s", abs)
		}
		return "", fmt.Errorf("E2E root lstat: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("E2E root must not be a symbolic link: %s", abs)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("E2E root is not a directory: %s", abs)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("E2E root resolve: %w", err)
	}
	return filepath.Clean(resolved), nil
}

var mkdir = os.Mkdir

// prepareRunWorkspace creates a unique child under a validated root plus
// project/, atlas-home/, and evidence/ directories. It never deletes existing paths.
func prepareRunWorkspace(validatedRoot string) (*runWorkspace, error) {
	root, err := resolveExistingPath(validatedRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve E2E root: %w", err)
	}
	runDir, err := os.MkdirTemp(root, "atlas-e2e-codegraph-*")
	if err != nil {
		return nil, fmt.Errorf("create unique E2E run dir: %w", err)
	}
	ws := &runWorkspace{
		Root:    root,
		RunDir:  runDir,
		created: true,
	}
	ws.Project = filepath.Join(runDir, "project")
	ws.AtlasHome = filepath.Join(runDir, "atlas-home")
	ws.Evidence = filepath.Join(runDir, "evidence")
	for _, d := range []string{ws.Project, ws.AtlasHome, ws.Evidence} {
		if err := mkdir(d, 0o755); err != nil {
			_ = ws.Cleanup()
			return nil, fmt.Errorf("create workspace dir: %w", err)
		}
	}
	runDirResolved, err := resolveExistingPath(runDir)
	if err != nil {
		_ = ws.Cleanup()
		return nil, fmt.Errorf("resolve run dir: %w", err)
	}
	ws.RunDir = runDirResolved
	ws.Project = filepath.Join(ws.RunDir, "project")
	ws.AtlasHome = filepath.Join(ws.RunDir, "atlas-home")
	ws.Evidence = filepath.Join(ws.RunDir, "evidence")
	return ws, nil
}

func resolveExistingPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return filepath.Clean(resolved), nil
}

// Cleanup removes only the uniquely created RunDir for this workspace.
func (w *runWorkspace) Cleanup() error {
	if w == nil || !w.created || w.RunDir == "" {
		return nil
	}
	info, err := os.Lstat(w.RunDir)
	if err != nil {
		if os.IsNotExist(err) {
			w.created = false
			return nil
		}
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to remove symlink run dir: %s", w.RunDir)
	}
	if !info.IsDir() {
		return fmt.Errorf("refusing to remove non-directory run dir: %s", w.RunDir)
	}
	root := w.Root
	if resolved, err := resolveExistingPath(w.Root); err == nil {
		root = resolved
	}
	runDir := w.RunDir
	if resolved, err := resolveExistingPath(w.RunDir); err == nil {
		runDir = resolved
	}
	rel, err := filepath.Rel(root, runDir)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("refusing cleanup outside E2E root: %s", w.RunDir)
	}
	w.RunDir = runDir
	if err := os.RemoveAll(w.RunDir); err != nil {
		return err
	}
	w.created = false
	return nil
}

// writeEvidenceFile creates a new regular evidence file exclusively (O_EXCL).
// It never follows a symlink leaf and never overwrites an existing path.
func writeEvidenceFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	n, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil {
		_ = os.Remove(path)
		return werr
	}
	if n != len(data) {
		_ = os.Remove(path)
		return fmt.Errorf("short write to %s: %d/%d", path, n, len(data))
	}
	if cerr != nil {
		_ = os.Remove(path)
		return cerr
	}
	return nil
}
