package home

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvAtlasHome is the environment variable that overrides the default Atlas Home.
const EnvAtlasHome = "ATLAS_HOME"

// DefaultDirName is the directory under the user home when ATLAS_HOME is unset.
const DefaultDirName = ".atlas"

// Layout directories are created under Atlas Home by mutating flows.
var LayoutDirectories = []string{
	"assets",
	"agents",
	"skills",
	"rules",
	"templates",
	"adapters",
	"state",
}

// Resolve returns the absolute Atlas Home path.
// ATLAS_HOME wins when set; otherwise ~/.atlas.
// Resolve never creates directories.
func Resolve() (string, error) {
	if override := strings.TrimSpace(os.Getenv(EnvAtlasHome)); override != "" {
		abs, err := filepath.Abs(override)
		if err != nil {
			return "", fmt.Errorf("atlas home: resolve %s: %w", EnvAtlasHome, err)
		}
		return abs, nil
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("atlas home: user home: %w", err)
	}
	return filepath.Join(userHome, DefaultDirName), nil
}

// PathJoin joins rel under the resolved Atlas Home using slash-safe segments.
func PathJoin(homePath string, elem ...string) string {
	parts := make([]string, 0, len(elem)+1)
	parts = append(parts, homePath)
	parts = append(parts, elem...)
	return filepath.Join(parts...)
}

// AssetsRoot returns $ATLAS_HOME/assets.
func AssetsRoot(homePath string) string {
	return filepath.Join(homePath, "assets")
}

// StateFile returns $ATLAS_HOME/state/home.yaml.
func StateFile(homePath string) string {
	return filepath.Join(homePath, "state", "home.yaml")
}

// Exists reports whether path exists (file or directory).
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Writable reports whether the directory (or nearest existing ancestor when
// missing) appears writable using permission bits. Read-only: never creates
// files or directories.
func Writable(path string) bool {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return false
		}
		return (info.Mode().Perm() & 0o200) != 0
	}
	if !os.IsNotExist(err) {
		return false
	}
	parent := filepath.Dir(path)
	for parent != "" && parent != string(filepath.Separator) && parent != "." {
		pinfo, err := os.Stat(parent)
		if err == nil {
			return pinfo.IsDir() && (pinfo.Mode().Perm()&0o200) != 0
		}
		if !os.IsNotExist(err) {
			return false
		}
		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		parent = next
	}
	return false
}
