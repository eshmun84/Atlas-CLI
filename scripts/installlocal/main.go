// Command installlocal stages the Atlas binary under PREFIX, materializes
// Atlas Home, then renames the staged binary into place so a Home failure
// cannot leave a new definitive binary published.
//
// Usage:
//
//	go run ./scripts/installlocal -binary=bin/atlas -prefix="$PREFIX"
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/home"
)

func main() {
	binary := flag.String("binary", "bin/atlas", "path to built atlas binary")
	prefix := flag.String("prefix", "", "install prefix (default: $PREFIX or ~/.local)")
	flag.Parse()

	p := *prefix
	if p == "" {
		p = os.Getenv("PREFIX")
	}
	if p == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			fatal(err)
		}
		p = filepath.Join(homeDir, ".local")
	}

	if err := Install(*binary, p, materializeHome); err != nil {
		fatal(err)
	}
}

func materializeHome() error {
	_, err := home.EnsureAndMirror(time.Now().UTC())
	return err
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "installlocal: %v\n", err)
	os.Exit(1)
}

// Install copies binary into prefix/bin as a temporary name, runs materialize,
// then renames to prefix/bin/atlas. On any failure after staging, the temp
// file is removed and an existing final binary is left untouched.
func Install(binaryPath, prefix string, materialize func() error) error {
	if materialize == nil {
		return fmt.Errorf("materialize callback is required")
	}
	binaryPath = filepath.Clean(binaryPath)
	prefix = filepath.Clean(prefix)
	if binaryPath == "" || binaryPath == "." {
		return fmt.Errorf("binary path is required")
	}
	if prefix == "" || prefix == "." {
		return fmt.Errorf("prefix is required")
	}

	info, err := os.Stat(binaryPath)
	if err != nil {
		return fmt.Errorf("stat binary: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("binary path is a directory: %s", binaryPath)
	}

	destDir := filepath.Join(prefix, "bin")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", destDir, err)
	}

	finalPath := filepath.Join(destDir, "atlas")
	stage, err := os.CreateTemp(destDir, ".atlas.new.*")
	if err != nil {
		return fmt.Errorf("create staged binary: %w", err)
	}
	stagePath := stage.Name()
	removeStage := true
	defer func() {
		if removeStage {
			_ = os.Remove(stagePath)
		}
	}()

	src, err := os.Open(binaryPath)
	if err != nil {
		_ = stage.Close()
		return fmt.Errorf("open binary: %w", err)
	}
	if _, err := io.Copy(stage, src); err != nil {
		_ = src.Close()
		_ = stage.Close()
		return fmt.Errorf("copy binary: %w", err)
	}
	_ = src.Close()
	if err := stage.Chmod(0o755); err != nil {
		_ = stage.Close()
		return fmt.Errorf("chmod staged binary: %w", err)
	}
	if err := stage.Close(); err != nil {
		return fmt.Errorf("close staged binary: %w", err)
	}

	if err := materialize(); err != nil {
		return fmt.Errorf("materialize Atlas Home: %w", err)
	}

	if err := os.Rename(stagePath, finalPath); err != nil {
		return fmt.Errorf("publish binary: %w", err)
	}
	removeStage = false
	fmt.Printf("installed %s\n", finalPath)
	return nil
}
