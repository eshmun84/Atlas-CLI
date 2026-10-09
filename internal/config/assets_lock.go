package config

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/assets"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/version"
)

// Testable seams; production uses RenderRuntimeManifestYAML / home.ReadCanonical / embed FS.
var (
	renderRuntimeManifestYAMLFn = RenderRuntimeManifestYAML
	readCanonicalAssetFn        = home.ReadCanonical
	readEmbeddedAssetFn         = func(path string) ([]byte, error) {
		return assets.Content.ReadFile(path)
	}
)

// BuildAssetsLockDocumentFor builds the project assets lock with Home references.
// Render failures (e.g. runtime manifest) abort; no empty/invalid checksum is recorded.
// Declared bundled assets must be readable; omission from the lock is never silent.
func BuildAssetsLockDocumentFor(homePath string, doc ProjectDocument, atlasVersion string) (AssetsLockDocument, error) {
	ver := strings.TrimSpace(atlasVersion)
	if ver == "" {
		ver = version.Version
	}
	if ver == "" {
		ver = "0.1.0"
	}
	selected := doc.Adapters.Selected
	entries := make([]AssetsLockEntry, 0, 32)

	// Bundled/Home-backed assets that project materialization depends on.
	for _, asset := range home.BundledAssets() {
		data, err := readCanonicalAssetFn(asset.EmbedPath)
		if err != nil {
			data, err = readEmbeddedAssetFn(asset.EmbedPath)
			if err != nil {
				return AssetsLockDocument{}, fmt.Errorf(
					"assets lock: bundled asset %s (%s): %w",
					asset.ID, asset.EmbedPath, err,
				)
			}
		}
		entry := AssetsLockEntry{
			ID:       asset.ID,
			Family:   asset.Family,
			Source:   "bundled",
			Version:  ver,
			Checksum: checksumHex(data),
		}
		if homePath != "" {
			// Home-relative only — never absolute machine paths in portable lock.
			entry.HomePath = home.RelHomePath(homePath, home.AssetHomePath(homePath, asset))
		}
		entry.ProjectPaths = projectPathsForHomeAsset(asset, selected, DependsOnSDDOpenSpecContract(doc))
		entries = append(entries, entry)
	}

	// Generated project surfaces.
	agentsMD, err := RenderAgentsMD(doc.Project.Name, doc.ContextGraphEnabled(), selected, nil)
	if err != nil {
		return AssetsLockDocument{}, err
	}
	entries = append(entries, AssetsLockEntry{
		ID:           "project/AGENTS.md",
		Family:       "contract",
		Source:       "generated",
		Version:      ver,
		Checksum:     checksumHex([]byte(agentsMD)),
		ProjectPaths: []string{FileAgentsMD},
	})
	registry := RenderAgentRegistry(doc.Project.Name, selected, homePath)
	entries = append(entries, AssetsLockEntry{
		ID:           "project/agent-registry.md",
		Family:       "registry",
		Source:       "generated",
		Version:      ver,
		Checksum:     checksumHex([]byte(registry)),
		ProjectPaths: []string{FileAgentRegistry},
	})
	manifest, err := renderRuntimeManifestYAMLFn(doc.Project.Name, selected)
	if err != nil {
		return AssetsLockDocument{}, err
	}
	entries = append(entries, AssetsLockEntry{
		ID:           "project/runtime-manifest.yaml",
		Family:       "manifest",
		Source:       "generated",
		Version:      ver,
		Checksum:     checksumHex([]byte(manifest)),
		ProjectPaths: []string{FileRuntimeManifest},
	})

	return AssetsLockDocument{
		SchemaVersion: PersistSchemaVersion,
		Assets:        entries,
	}, nil
}

// RenderAssetsLockYAMLFor marshals a fully contextual assets lock.
func RenderAssetsLockYAMLFor(homePath string, doc ProjectDocument) (string, error) {
	lockDoc, err := BuildAssetsLockDocumentFor(homePath, doc, version.Version)
	if err != nil {
		return "", err
	}
	data, err := marshalYAML(lockDoc)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func projectPathsForHomeAsset(asset home.Asset, selected []string, dependsOnSDDContract bool) []string {
	adapters := normalizeSelectedAdapters(selected)
	switch asset.EmbedPath {
	case "adapter-files/cursor/atlas.mdc":
		for _, adapter := range adapters {
			if adapter == "cursor" {
				return []string{FileCursorAtlasMDC}
			}
		}
		return nil
	case "adapter-files/opencode/atlas.md":
		for _, adapter := range adapters {
			if adapter == "opencode" {
				return []string{FileOpenCodeAtlas}
			}
		}
		return nil
	}
	if strings.HasPrefix(asset.EmbedPath, "agents/runtime/") {
		name := filepath.Base(asset.EmbedPath)
		var paths []string
		for _, adapter := range adapters {
			if path := AtlasAgentRuntimePath(adapter, name); path != "" {
				paths = append(paths, path)
			}
		}
		return paths
	}
	// Contract fragments feed AGENTS.md composition; project path is the contract file.
	if asset.EmbedPath == "agents/base.md" ||
		asset.EmbedPath == "agents/adapters/cursor.md" ||
		asset.EmbedPath == "agents/adapters/opencode.md" {
		return []string{FileAgentsMD}
	}
	if asset.EmbedPath == EmbedPathSDDOpenSpecContract && dependsOnSDDContract {
		return []string{FileSDDOpenSpecContract}
	}
	return nil
}

func checksumHex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
