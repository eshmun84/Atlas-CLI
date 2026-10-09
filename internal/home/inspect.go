package home

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/project/fsafety"
)

// Testable seam for embedded asset reads; production uses assets.Content.ReadFile.
var readEmbeddedAssetFn = func(embedPath string) ([]byte, error) {
	return assets.Content.ReadFile(embedPath)
}

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
	// AssetErrors reports declared bundled assets whose embedded canonical
	// content could not be read (integrity failure, not Home drift/missing),
	// and Home path integrity failures (symlink root/layout/asset leaves).
	AssetErrors []string
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
// Symlink-safe: never follows Home root, layout dirs, or asset leaves.
func InspectPath(homePath string) Status {
	status := Status{
		Path:          homePath,
		MissingAssets: []string{},
		DriftedAssets: []string{},
		AssetErrors:   []string{},
	}

	rootInfo, rootErr := os.Lstat(homePath)
	if os.IsNotExist(rootErr) {
		status.Exists = false
		status.Writable = Writable(homePath)
		status.MissingAssets = assetIDs(BundledAssets())
		return status
	}
	if rootErr != nil {
		status.Exists = false
		status.StateError = rootErr.Error()
		status.MissingAssets = assetIDs(BundledAssets())
		return status
	}
	status.Exists = true
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		status.Writable = false
		status.LayoutComplete = false
		status.AssetErrors = append(status.AssetErrors, "home root is a symlink")
		status.StateError = "atlas home root is a symlink"
		status.MissingAssets = assetIDs(BundledAssets())
		return status
	}
	if !rootInfo.IsDir() {
		status.Writable = false
		status.LayoutComplete = false
		status.AssetErrors = append(status.AssetErrors, "home root is not a directory")
		status.StateError = "atlas home root is not a directory"
		return status
	}
	status.Writable = Writable(homePath)

	status.LayoutComplete = layoutCompleteSafe(homePath, &status)
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
		rel := filepath.ToSlash(filepath.Join("assets", filepath.FromSlash(asset.EmbedPath)))
		data, err := fsafety.ReadFileContained(homePath, rel)
		if err != nil {
			if os.IsNotExist(err) {
				status.MissingAssets = append(status.MissingAssets, asset.ID)
				continue
			}
			status.AssetErrors = append(status.AssetErrors, asset.ID+": "+err.Error())
			continue
		}
		want, err := readEmbeddedAssetFn(asset.EmbedPath)
		if err != nil {
			status.AssetErrors = append(status.AssetErrors, asset.ID+": "+err.Error())
			continue
		}
		if sha256Hex(data) != sha256Hex(want) {
			status.DriftedAssets = append(status.DriftedAssets, asset.ID)
		}
	}
	return status
}

func layoutCompleteSafe(homePath string, status *Status) bool {
	complete := true
	for _, dir := range LayoutDirectories {
		info, err := fsafety.LstatContained(homePath, dir)
		if err != nil {
			complete = false
			if !os.IsNotExist(err) {
				status.AssetErrors = append(status.AssetErrors, fmt.Sprintf("layout %s: %v", dir, err))
			}
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			complete = false
			status.AssetErrors = append(status.AssetErrors, "layout "+dir+" is a symlink")
			continue
		}
		if !info.IsDir() {
			complete = false
			status.AssetErrors = append(status.AssetErrors, "layout "+dir+" is not a directory")
		}
	}
	return complete
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
