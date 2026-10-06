package home

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
)

// Status is a read-only snapshot of Atlas Home. Never mutates the filesystem.
type Status struct {
	Path           string
	Exists         bool
	Writable       bool
	LayoutComplete bool
	StatePresent   bool
	StateLoads     bool
	StateError     string
	CreatedAt      string
	UpdatedAt      string
	AssetCount     int
	MissingAssets  []string
	DriftedAssets  []string
}

// Inspect resolves Atlas Home and reports layout/asset health without creating anything.
func Inspect() Status {
	homePath, err := Resolve()
	if err != nil {
		return Status{
			Path:       "",
			Exists:     false,
			Writable:   false,
			StateError: err.Error(),
		}
	}
	return InspectPath(homePath)
}

// InspectPath inspects a concrete Atlas Home path without creating anything.
func InspectPath(homePath string) Status {
	status := Status{
		Path:          homePath,
		Exists:        Exists(homePath),
		Writable:      Writable(homePath),
		MissingAssets: []string{},
		DriftedAssets: []string{},
	}
	if !status.Exists {
		status.MissingAssets = assetIDs(BundledAssets())
		return status
	}

	status.LayoutComplete = layoutComplete(homePath)
	doc, present, err := LoadState(homePath)
	status.StatePresent = present
	if err != nil {
		status.StateLoads = false
		status.StateError = err.Error()
	} else if present {
		status.StateLoads = true
		status.CreatedAt = doc.CreatedAt
		status.UpdatedAt = doc.UpdatedAt
		status.AssetCount = len(doc.Assets)
	}

	for _, asset := range BundledAssets() {
		dest := AssetHomePath(homePath, asset)
		data, err := os.ReadFile(dest)
		if err != nil {
			status.MissingAssets = append(status.MissingAssets, asset.ID)
			continue
		}
		want, err := assets.Content.ReadFile(asset.EmbedPath)
		if err != nil {
			continue
		}
		if sha256Hex(data) != sha256Hex(want) {
			status.DriftedAssets = append(status.DriftedAssets, asset.ID)
		}
	}
	return status
}

func layoutComplete(homePath string) bool {
	for _, dir := range LayoutDirectories {
		info, err := os.Stat(filepath.Join(homePath, dir))
		if err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

func assetIDs(assets []Asset) []string {
	out := make([]string, 0, len(assets))
	for _, asset := range assets {
		out = append(out, asset.ID)
	}
	return out
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
