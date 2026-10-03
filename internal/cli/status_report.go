package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/workspace"
)

func writeStatusReport(w io.Writer, result workspace.DiscoveryResult) {
	fmt.Fprintln(w, "Atlas Status")
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Workspace:")
	fmt.Fprintf(w, "  Root: %s\n", result.RootPath)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Git:")
	fmt.Fprintf(w, "  Repository: %s\n", yesNo(result.Git.IsRepo))
	fmt.Fprintf(w, "  Branch: %s\n", displayBranch(result.Git.CurrentBranch))
	fmt.Fprintln(w, "  Remotes:")
	if len(result.Git.Remotes) == 0 {
		fmt.Fprintln(w, "    none detected")
	} else {
		for _, remote := range result.Git.Remotes {
			fmt.Fprintf(w, "    - %s %s\n", remote.Name, remote.URL)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Files:")
	fmt.Fprintf(w, "  README.md: %s\n", yesNo(result.Files.HasReadme))
	fmt.Fprintf(w, "  .gitignore: %s\n", yesNo(result.Files.HasGitignore))
	fmt.Fprintf(w, "  Makefile: %s\n", yesNo(result.Files.HasMakefile))
	fmt.Fprintf(w, "  go.mod: %s\n", yesNo(result.Files.HasGoMod))
	fmt.Fprintf(w, "  AGENTS.md: %s\n", yesNo(result.Files.HasAgentsFile))
	fmt.Fprintf(w, "  .atlas/: %s\n", yesNo(result.Files.HasAtlasDir))
	fmt.Fprintf(w, "  .atlas/config.yaml: %s\n", yesNo(result.Files.HasAtlasConfig))
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Technologies:")
	if len(result.Technologies) == 0 {
		fmt.Fprintln(w, "  none detected")
	} else {
		for _, tech := range result.Technologies {
			fmt.Fprintf(w, "  - %s (%s, %s)\n", tech.Name, tech.Source, tech.Confidence)
		}
	}
	fmt.Fprintln(w)

	fmt.Fprintln(w, "Tools:")
	for _, tool := range result.Tools {
		fmt.Fprintf(w, "  %s: %s\n", tool.Name, availability(tool.Available))
	}

	if len(result.Warnings) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Warnings:")
		for _, warning := range result.Warnings {
			fmt.Fprintf(w, "  - %s\n", warning)
		}
	}
}

func displayBranch(branch string) string {
	if strings.TrimSpace(branch) == "" {
		return "none"
	}
	return branch
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func availability(available bool) string {
	if available {
		return "available"
	}
	return "unavailable"
}
