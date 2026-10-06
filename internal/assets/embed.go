package assets

import "embed"

// Content holds embedded Atlas runtime contract, adapter entrypoints, agent pack, and operational contracts.
//
//go:embed agents/base.md agents/adapters/*.md agents/runtime/*.md adapter-files/cursor/atlas.mdc adapter-files/opencode/atlas.md contracts/sdd-openspec.md
var Content embed.FS
