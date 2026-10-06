package context

import (
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

// RenderCapsule builds a deterministic compact capsule markdown from an index.
// No LLM is used.
func RenderCapsule(idx IndexDocument) string {
	var b strings.Builder
	name := idx.ProjectName
	if name == "" {
		name = "unnamed project"
	}
	fmt.Fprintf(&b, "# Atlas Context Capsule\n\n")
	fmt.Fprintf(&b, "Deterministic project capsule for Context Economy v0. Not an LLM summary.\n\n")
	fmt.Fprintf(&b, "## Identity\n\n")
	fmt.Fprintf(&b, "- Name: **%s**\n", name)
	fmt.Fprintf(&b, "- Project ID: `%s`\n", idx.ProjectID)
	fmt.Fprintf(&b, "- Root: `%s`\n", idx.ProjectRoot)
	fmt.Fprintf(&b, "- Indexed at: `%s`\n", idx.IndexedAt)
	if idx.AtlasVersion != "" {
		fmt.Fprintf(&b, "- Atlas version: `%s`\n", idx.AtlasVersion)
	}
	fmt.Fprintf(&b, "- Fingerprint: `%s`\n\n", idx.Fingerprint)

	fmt.Fprintf(&b, "## Technologies\n\n")
	if len(idx.Languages) == 0 && len(idx.Frameworks) == 0 {
		fmt.Fprintf(&b, "- none detected from indexed manifests/sources\n\n")
	} else {
		for _, lang := range idx.Languages {
			fmt.Fprintf(&b, "- Language: %s\n", lang)
		}
		for _, fw := range idx.Frameworks {
			fmt.Fprintf(&b, "- Framework/tooling: %s\n", fw)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintf(&b, "## Important structure\n\n")
	writeLimited(&b, "Top directories", idx.Directories, 24)
	writeLimited(&b, "Entrypoints", idx.Entrypoints, 16)
	writeLimited(&b, "Manifests", idx.Manifests, 16)

	fmt.Fprintf(&b, "## Atlas runtime\n\n")
	if len(idx.RuntimeAtlas) == 0 {
		fmt.Fprintf(&b, "- No Atlas runtime surfaces indexed (project may be uninitialized).\n\n")
	} else {
		writeLimited(&b, "Runtime files", idx.RuntimeAtlas, 32)
	}

	fmt.Fprintf(&b, "## Contracts\n\n")
	if len(idx.Contracts) == 0 {
		fmt.Fprintf(&b, "- none indexed\n")
		fmt.Fprintf(&b, "- Expected SDD contract when governed: `%s`\n\n", config.FileSDDOpenSpecContract)
	} else {
		writeLimited(&b, "Active/contract paths", idx.Contracts, 16)
	}

	fmt.Fprintf(&b, "## Tests and checks\n\n")
	writeLimited(&b, "Test files", idx.TestFiles, 24)
	hasMakefile := false
	for _, m := range idx.Manifests {
		if strings.EqualFold(m, "Makefile") || strings.EqualFold(m, "makefile") {
			hasMakefile = true
			break
		}
	}
	if hasMakefile {
		fmt.Fprintf(&b, "- Makefile present (common local checks may exist).\n")
	}
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "## Docs\n\n")
	writeLimited(&b, "Documents", idx.Docs, 16)

	fmt.Fprintf(&b, "## Risks / unknowns\n\n")
	if idx.Truncated {
		fmt.Fprintf(&b, "- Index truncated; some paths were not recorded.\n")
	}
	if idx.IgnoredDirs+idx.IgnoredFiles > 0 {
		fmt.Fprintf(&b, "- Ignored during indexing: %d dirs, %d files (noise, binaries, large files, backups).\n", idx.IgnoredDirs, idx.IgnoredFiles)
	}
	if len(idx.Languages) == 0 {
		fmt.Fprintf(&b, "- Language detection found no strong signals; treat technology claims cautiously.\n")
	}
	fmt.Fprintf(&b, "- Capsule is rule-based; it does not prove architectural completeness.\n")
	fmt.Fprintf(&b, "- Do not invent repo facts beyond indexed paths.\n")
	return b.String()
}

func writeLimited(b *strings.Builder, title string, paths []string, limit int) {
	fmt.Fprintf(b, "### %s\n\n", title)
	if len(paths) == 0 {
		fmt.Fprintf(b, "- none\n\n")
		return
	}
	n := len(paths)
	if n > limit {
		n = limit
	}
	for i := 0; i < n; i++ {
		fmt.Fprintf(b, "- `%s`\n", paths[i])
	}
	if len(paths) > limit {
		fmt.Fprintf(b, "- … %d more\n", len(paths)-limit)
	}
	fmt.Fprintln(b)
}
