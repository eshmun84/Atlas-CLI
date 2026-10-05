package assets

import "embed"

// Content holds embedded Atlas runtime contract and adapter entrypoint assets.
//
//go:embed agents/base.md agents/adapters/*.md adapter-files/cursor/atlas.mdc adapter-files/opencode/atlas.md
var Content embed.FS
