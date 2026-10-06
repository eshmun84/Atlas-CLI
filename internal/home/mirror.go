package home

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

// EnsureResult summarizes a mutating Atlas Home ensure/mirror operation.
type EnsureResult struct {
	HomePath  string
	Created   bool
	Mirrored  []string
	UpdatedAt string
}

// EnsureLayout creates the Atlas Home directory tree. Mutating.
func EnsureLayout(homePath string) error {
	if err := os.MkdirAll(homePath, 0o755); err != nil {
		return fmt.Errorf("atlas home: create %s: %w", homePath, err)
	}
	for _, dir := range LayoutDirectories {
		path := filepath.Join(homePath, dir)
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("atlas home: create %s: %w", dir, err)
		}
	}
	return nil
}

// EnsureAndMirror creates layout (if needed) and mirrors bundled assets into Home.
// This is the only supported mutating Home bootstrap used by init/apply and repair.
func EnsureAndMirror(now time.Time) (EnsureResult, error) {
	homePath, err := Resolve()
	if err != nil {
		return EnsureResult{}, err
	}
	created := !Exists(homePath)
	if err := EnsureLayout(homePath); err != nil {
		return EnsureResult{}, err
	}

	existing, _, _ := LoadState(homePath)
	entries := make([]StateAssetEntry, 0, len(BundledAssets()))
	mirrored := make([]string, 0, len(BundledAssets()))
	ver := version.Version
	if ver == "" {
		ver = "0.1.0"
	}

	for _, asset := range BundledAssets() {
		data, err := assets.Content.ReadFile(asset.EmbedPath)
		if err != nil {
			return EnsureResult{}, fmt.Errorf("atlas home: read bundled %s: %w", asset.EmbedPath, err)
		}
		sum := sha256.Sum256(data)
		checksum := hex.EncodeToString(sum[:])
		dest := AssetHomePath(homePath, asset)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return EnsureResult{}, fmt.Errorf("atlas home: create parent for %s: %w", asset.ID, err)
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return EnsureResult{}, fmt.Errorf("atlas home: write %s: %w", asset.ID, err)
		}
		mirrored = append(mirrored, asset.ID)

		if convenience := ConvenienceHomePath(homePath, asset); convenience != "" {
			if err := os.MkdirAll(filepath.Dir(convenience), 0o755); err != nil {
				return EnsureResult{}, fmt.Errorf("atlas home: create convenience parent for %s: %w", asset.ID, err)
			}
			if err := os.WriteFile(convenience, data, 0o644); err != nil {
				return EnsureResult{}, fmt.Errorf("atlas home: write convenience %s: %w", asset.ID, err)
			}
		}

		entries = append(entries, StateAssetEntry{
			Family:       asset.Family,
			ID:           asset.ID,
			Version:      ver,
			Checksum:     checksum,
			Source:       "bundled",
			ResolvedPath: dest,
		})
	}

	createdAt := existing.CreatedAt
	if createdAt == "" {
		createdAt = stamp(now)
	}
	doc := StateDocument{
		SchemaVersion: StateSchemaVersion,
		HomePath:      homePath,
		CreatedAt:     createdAt,
		UpdatedAt:     stamp(now),
		Source:        "bundled",
		AtlasVersion:  ver,
		Assets:        entries,
	}
	if err := WriteState(homePath, doc); err != nil {
		return EnsureResult{}, err
	}

	return EnsureResult{
		HomePath:  homePath,
		Created:   created,
		Mirrored:  mirrored,
		UpdatedAt: doc.UpdatedAt,
	}, nil
}

// ReadCanonical returns asset bytes from Atlas Home when present and matching,
// otherwise falls back to the embedded bundled asset.
func ReadCanonical(embedPath string) ([]byte, error) {
	homePath, err := Resolve()
	if err == nil {
		candidate := filepath.Join(AssetsRoot(homePath), filepath.FromSlash(embedPath))
		if data, readErr := os.ReadFile(candidate); readErr == nil {
			return data, nil
		}
	}
	data, err := assets.Content.ReadFile(embedPath)
	if err != nil {
		return nil, fmt.Errorf("atlas home: canonical %s: %w", embedPath, err)
	}
	return data, nil
}
