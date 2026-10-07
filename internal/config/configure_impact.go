package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Configure impact / honesty copy shared by Configure Apply and Review.
const (
	ConfigureApplySuccessTitle = "Configuration changes are saved to .atlas/config.yaml."
	ConfigureApplySuccessBody  = "Runtime files are not repaired automatically."

	ConfigureRepairHint  = "Run Runtime Repair when adapter/runtime assets need to be updated."
	ConfigureContextHint = "Run Update Context when project context should be refreshed."
	ConfigureMCPHint     = "MCP selections record preferences/config only until materialization/auth/verification exists."

	configureFooterDocsScaffold = "Project docs scaffold is selected: Apply may also create docs/atlas/README.md once if missing."
)

// ConfigureApplyFooterNote is the default (docs scaffold off) Configure footer.
// Prefer FormatConfigureFooterNote when a draft is available.
var ConfigureApplyFooterNote = formatConfigureFooterNote(false)

// FormatConfigureFooterNote returns Configure Apply footer copy for the current draft.
// When docs scaffold is selected it must not claim config.yaml is the only possible write.
func FormatConfigureFooterNote(draft ConfigDraft) string {
	docsSelected := false
	if field, ok := draft.FieldByKey("project.docs_scaffold"); ok {
		docsSelected = strings.EqualFold(strings.TrimSpace(field.Value), "true")
	}
	return formatConfigureFooterNote(docsSelected)
}

func formatConfigureFooterNote(docsScaffoldSelected bool) string {
	lines := []string{
		"Close discards unsaved changes. Apply saves .atlas/config.yaml.",
	}
	if docsScaffoldSelected {
		lines = append(lines, configureFooterDocsScaffold)
	}
	lines = append(lines,
		"Runtime files are not repaired or rematerialized automatically.",
		"Context Economy payloads are not updated automatically.",
		"MCP selections are preference/config only — not materialized, connected, authenticated, or verified.",
		"Runtime Repair and Update Context are separate flows.",
	)
	return strings.Join(lines, "\n")
}

// FileProjectDocsREADME is the optional developer-owned project docs scaffold.
const FileProjectDocsREADME = "docs/atlas/README.md"

// ProjectDocsScaffoldContent is the template written once when scaffold is selected.
const ProjectDocsScaffoldContent = `# Project documentation (Atlas scaffold)

This folder is **project-owned**, versionable documentation.

Atlas will **not** overwrite it during Runtime Repair. Configure Apply creates
docs/atlas/README.md only once when selected and missing.

Add architecture notes, decisions, delivery evidence, or onboarding notes here.
Atlas framework docs stay in the Atlas CLI repository — they are not dumped into projects.
`

// ConfigureImpact summarizes what Configure Apply did and what still needs explicit flows.
type ConfigureImpact struct {
	ConfigOnly           bool
	RuntimeRepairNeeded  bool
	ContextUpdateHint    bool
	MCPPreferenceOnly    bool
	DocsScaffoldSelected bool
	DocsCreated          []string
	DocsSkipped          []string
	AdapterChange        bool
	Lines                []string
}

// AnalyzeConfigureImpact compares previous persisted config with the applied draft.
// previous may be zero-value when no prior config existed.
func AnalyzeConfigureImpact(previous, next ProjectDocument, mcpChanged bool) ConfigureImpact {
	impact := ConfigureImpact{
		ConfigOnly:        true,
		MCPPreferenceOnly: mcpChanged || mcpPreferencePresent(next),
	}

	prevAdapters := JoinChips(previous.Adapters.Selected)
	nextAdapters := JoinChips(next.Adapters.Selected)
	if prevAdapters != nextAdapters {
		impact.AdapterChange = true
		impact.RuntimeRepairNeeded = true
	}

	// Context Economy payload is not written by Configure; remind when graph preference flips
	// or when project remains initialized (operators may want a fresh capsule after config churn).
	prevGraph := previous.ContextGraphEnabled()
	nextGraph := next.ContextGraphEnabled()
	if prevGraph != nextGraph {
		impact.ContextUpdateHint = true
	}

	impact.DocsScaffoldSelected = next.Project.DocsScaffold

	impact.Lines = impact.NoticeLines()
	return impact
}

// NoticeLines returns compact honest Configure Apply notices.
func (i ConfigureImpact) NoticeLines() []string {
	lines := []string{
		ConfigureApplySuccessTitle,
		ConfigureApplySuccessBody,
	}
	if i.RuntimeRepairNeeded || i.AdapterChange {
		lines = append(lines, ConfigureRepairHint)
	}
	if i.ContextUpdateHint {
		lines = append(lines, ConfigureContextHint)
	}
	if i.MCPPreferenceOnly {
		lines = append(lines, ConfigureMCPHint)
	}
	for _, path := range i.DocsCreated {
		lines = append(lines, "Created project docs scaffold: "+path)
	}
	for _, path := range i.DocsSkipped {
		lines = append(lines, "Project docs scaffold left untouched (already present): "+path)
	}
	if i.DocsScaffoldSelected && len(i.DocsCreated) == 0 && len(i.DocsSkipped) == 0 {
		lines = append(lines, "Project docs scaffold selected; create runs only when "+FileProjectDocsREADME+" is missing.")
	}
	return lines
}

// FormatConfigureNotice joins impact lines for the Configure footer.
func FormatConfigureNotice(impact ConfigureImpact) string {
	lines := impact.Lines
	if len(lines) == 0 {
		lines = impact.NoticeLines()
	}
	return strings.Join(lines, "\n")
}

func mcpPreferencePresent(doc ProjectDocument) bool {
	if doc.MCP.Builtins.Jira.Enabled || doc.MCP.Builtins.Context7.Enabled || doc.MCP.Builtins.ChromeDevTools.Enabled {
		return true
	}
	for _, custom := range doc.MCP.Custom {
		if custom.Enabled || strings.TrimSpace(custom.Name) != "" {
			return true
		}
	}
	return false
}

// EnsureProjectDocsScaffold creates docs/atlas/README.md once when selected.
// Never overwrites an existing file. Never touches unrelated docs/ content.
// Returns created and skipped relative paths.
func EnsureProjectDocsScaffold(root string, selected bool) (created, skipped []string, err error) {
	if !selected {
		return nil, nil, nil
	}
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil, nil, fmt.Errorf("project docs: workspace root is required")
	}
	rel := FileProjectDocsREADME
	full := filepath.Join(root, filepath.FromSlash(rel))
	if info, statErr := os.Stat(full); statErr == nil {
		if info.IsDir() {
			return nil, nil, fmt.Errorf("project docs: %s exists as a directory", rel)
		}
		return nil, []string{rel}, nil
	} else if !os.IsNotExist(statErr) {
		return nil, nil, fmt.Errorf("project docs: stat %s: %w", rel, statErr)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return nil, nil, fmt.Errorf("project docs: create parent: %w", err)
	}
	if err := os.WriteFile(full, []byte(ProjectDocsScaffoldContent), 0o644); err != nil {
		return nil, nil, fmt.Errorf("project docs: write %s: %w", rel, err)
	}
	return []string{rel}, nil, nil
}
