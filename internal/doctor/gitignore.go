package doctor

import (
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
)

func evaluateGitignore(result inspect.Inspection) []Check {
	if !result.Runtime.ConfigLoads {
		return nil
	}
	root := strings.TrimSpace(result.RootPath)
	if root == "" || root == "." {
		return nil
	}
	analysis, err := config.AnalyzeLocalOnlyGitignore(root, result.Runtime.Document)
	if err != nil {
		return []Check{fail("gitignore", err.Error(), RepairManual, "Resolve filesystem access for .gitignore")}
	}
	if !analysis.Applicable {
		return nil
	}
	switch analysis.State {
	case config.GitignoreHealthy:
		return []Check{pass("gitignore", analysis.Reason)}
	case config.GitignoreMissingFile, config.GitignoreMissingBlock, config.GitignoreStale, config.GitignoreMultiple:
		return []Check{fail("gitignore", analysis.Reason, RepairConfirm,
			"Run Runtime Repair to reconcile the Atlas local governance .gitignore block")}
	case config.GitignoreMalformed:
		return []Check{fail("gitignore", analysis.Reason, RepairManual,
			"Fix malformed Atlas markers manually; Atlas will not mutate ambiguous delimiters")}
	default:
		return []Check{info("gitignore", string(analysis.State))}
	}
}
