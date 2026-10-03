package workspace

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Library is a known dependency detected from the project manifests.
type Library struct {
	Name   string
	Module string
	Source string
}

var knownGoLibraries = []Library{
	{Name: "Bubble Tea", Module: "github.com/charmbracelet/bubbletea"},
	{Name: "Lip Gloss", Module: "github.com/charmbracelet/lipgloss"},
	{Name: "Bubbles", Module: "github.com/charmbracelet/bubbles"},
}

// DiscoverLibraries inspects known manifests for recognized libraries. Read-only.
func DiscoverLibraries(root string, files FileInfo) []Library {
	if !files.HasGoMod {
		return nil
	}
	mods, err := parseGoModRequires(filepath.Join(root, "go.mod"))
	if err != nil {
		return nil
	}
	var libs []Library
	for _, known := range knownGoLibraries {
		if mods[known.Module] {
			libs = append(libs, Library{
				Name:   known.Name,
				Module: known.Module,
				Source: "go.mod",
			})
		}
	}
	return libs
}

func parseGoModRequires(path string) (map[string]bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mods := make(map[string]bool)
	inBlock := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.HasPrefix(line, "require (") {
			inBlock = true
			continue
		}
		if inBlock {
			if line == ")" {
				inBlock = false
				continue
			}
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				mods[fields[0]] = true
			}
			continue
		}
		if strings.HasPrefix(line, "require ") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				mods[fields[1]] = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return mods, nil
}
