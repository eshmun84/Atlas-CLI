package assets

import "embed"

// Content holds embedded Atlas runtime contract, adapter entrypoints, and agent pack assets.
//
//go:embed agents/base.md agents/adapters/*.md agents/runtime/*.md adapter-files/cursor/atlas.mdc adapter-files/opencode/atlas.md
var Content embed.FS
