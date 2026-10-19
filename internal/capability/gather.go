package capability

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/skills"
)

// GatherInput configures read-only Tier-0/1 evidence collection for planning.
type GatherInput struct {
	Root           string
	Home           string
	Task           string
	Phase          string
	Classification string
	ActiveChange   string
	Files          []string
	Getwd          func() (string, error)
}

// GatherResult is bounded evidence plus project identity (when available).
type GatherResult struct {
	Evidence  TaskEvidence
	ProjectID string
	HomePath  string
	Root      string
	Warnings  []string
}

// GatherEvidence collects cheap Tier-0/1 inputs without Memory/CodeGraph/MCP calls.
// Missing Atlas init still yields a plan from task text + technology markers.
func GatherEvidence(in GatherInput) (GatherResult, error) {
	root := strings.TrimSpace(in.Root)
	if root == "" {
		getwd := in.Getwd
		if getwd == nil {
			getwd = os.Getwd
		}
		wd, err := getwd()
		if err != nil {
			return GatherResult{}, fmt.Errorf("capability: working directory: %w", err)
		}
		root = wd
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return GatherResult{}, fmt.Errorf("capability: resolve root: %w", err)
	}

	out := GatherResult{Root: absRoot}
	ev := TaskEvidence{
		TaskText:       strings.TrimSpace(in.Task),
		Phase:          strings.TrimSpace(in.Phase),
		Classification: strings.TrimSpace(in.Classification),
		ActiveChange:   strings.TrimSpace(in.ActiveChange),
		ExplicitFiles:  append([]string{}, in.Files...),
	}

	// Tier 1: bounded technology markers (not full Inspect).
	files, err := project.DiscoverFiles(absRoot)
	if err != nil {
		out.Warnings = append(out.Warnings, "technology discovery unavailable: "+err.Error())
	} else {
		techs := project.DiscoverTechnologies(files)
		for _, t := range techs {
			ev.Technologies = append(ev.Technologies, t.Name)
		}
		if files.HasOpenSpecDir && ev.ActiveChange == "" {
			// Presence only — do not scan change bodies.
			if project.Exists(absRoot, filepath.Join("openspec", "changes")) {
				ev.ActiveChange = "openspec/changes"
			}
		}
	}

	homePath := strings.TrimSpace(in.Home)
	if homePath == "" {
		homePath, err = home.Resolve()
		if err != nil {
			out.Warnings = append(out.Warnings, "atlas home unresolved: "+err.Error())
		}
	}
	out.HomePath = homePath

	// Config / pins / governance when project is initialized.
	if doc, loadErr := config.LoadProjectDocumentAt(absRoot); loadErr == nil {
		ev.Workflow = strings.TrimSpace(doc.Governance.Workflow)
		ev.TestingRequired = doc.Governance.TestingRequired
		ev.ReviewRequired = doc.Governance.ReviewRequired
		// Evidence is inherent governance behavior — never opt-out via config.
		ev.EvidenceRequired = true
		ev.EnabledPins = doc.SkillPins()
		if name := strings.TrimSpace(doc.Project.Name); name != "" && homePath != "" {
			if pid, pidErr := home.ProjectID(absRoot, name); pidErr == nil {
				out.ProjectID = pid
			}
		}
		if len(doc.Adapters.Selected) > 0 {
			ev.AdapterIDs = append([]string{}, doc.Adapters.Selected...)
		}
	} else {
		out.Warnings = append(out.Warnings, "project config not loaded; using default skill pins")
		ev.EnabledPins = skills.DefaultPins()
	}

	if homePath != "" {
		catalog, catErr := skills.DiscoverCatalog(homePath)
		if catErr != nil {
			out.Warnings = append(out.Warnings, "skill catalog: "+catErr.Error())
			if bundled, bErr := skills.BundledCatalog(); bErr == nil {
				ev.Catalog = bundled
			}
		} else {
			ev.Catalog = catalog
		}
	} else if bundled, bErr := skills.BundledCatalog(); bErr == nil {
		ev.Catalog = bundled
	}

	out.Evidence = ev
	return out, nil
}
