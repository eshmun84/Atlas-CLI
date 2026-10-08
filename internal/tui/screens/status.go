package screens

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	atlascontext "github.com/eshmun84/Atlas-CLI/internal/context"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"github.com/eshmun84/Atlas-CLI/internal/project"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

var (
	statusHead = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("51"))
	statusYes  = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	statusNo   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	statusWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	statusFail = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusInfo = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	statusAct  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
)

// Status renders the read-only executive project overview (default landing).
// It consumes the composed discovery snapshot only — no re-discovery of Git,
// tools, runtime, or Code Intelligence. Health counts come from doctor.Evaluate
// on that same snapshot so they match Doctor.
func Status(result inspect.Inspection) string {
	return StatusWithReport(result, doctor.Evaluate(result))
}

// StatusWithReport renders Status using an existing diagnostics report.
// Callers (TUI shell) should pass the same report snapshot shown on Doctor.
// If report has no checks, Status falls back to doctor.Evaluate(result).
func StatusWithReport(result inspect.Inspection, report doctor.Report) string {
	if len(report.Checks) == 0 {
		report = doctor.Evaluate(result)
	}
	var b strings.Builder

	fmt.Fprintln(&b, statusHead.Render("Atlas Status"))
	fmt.Fprintln(&b, statusNo.Render("Executive overview · read-only"))
	fmt.Fprintln(&b)

	writeStatusWorkspace(&b, result)
	writeStatusRuntime(&b, result)
	writeStatusSourceControl(&b, result)
	writeStatusTechnology(&b, result)
	writeStatusAdapters(&b, result)
	writeStatusGovernance(&b, result)
	writeStatusMCP(&b, result)
	writeStatusHealth(&b, result, report)

	return strings.TrimRight(b.String(), "\n")
}

func writeStatusWorkspace(b *strings.Builder, result inspect.Inspection) {
	fmt.Fprintln(b, statusHead.Render("Workspace"))
	name := projectDisplayName(result)
	fmt.Fprintf(b, "  Project: %s\n", name)
	fmt.Fprintf(b, "  Root: %s\n", displayOrDash(result.RootPath))
	fmt.Fprintf(b, "  Atlas state: %s\n", result.Atlas.State)
	fmt.Fprintf(b, "  Project mode: %s\n\n", detectedModeLabel(result))
}

func writeStatusRuntime(b *strings.Builder, result inspect.Inspection) {
	rt := result.Runtime
	fmt.Fprintln(b, statusHead.Render("Atlas Runtime"))
	fmt.Fprintf(b, "  Initialized: %s\n", yesNo(rt.Initialized))
	fmt.Fprintf(b, "  Config: %s\n", configStatus(rt))
	fmt.Fprintf(b, "  State: %s\n", stateStatus(rt))
	fmt.Fprintf(b, "  Runtime: %s\n", runtimeMaterializedLabel(rt))
	fmt.Fprintf(b, "  AGENTS.md contract: %s\n", agentsContractSummary(rt))
	fmt.Fprintf(b, "  SDD/OpenSpec contract: %s\n", sddContractStatus(rt))
	fmt.Fprintf(b, "  Context Economy: %s\n", contextEconomyStatus(rt))
	fmt.Fprintf(b, "  Code Intelligence: %s\n\n", codeIntelligenceStatus(rt))
}

func writeStatusSourceControl(b *strings.Builder, result inspect.Inspection) {
	fmt.Fprintln(b, statusHead.Render("Source Control / Delivery Tools"))
	fmt.Fprintf(b, "  Repository: %s\n", yesNo(result.Git.IsRepo))
	fmt.Fprintf(b, "  Current branch: %s\n", displayOrNone(result.Git.CurrentBranch))
	fmt.Fprintf(b, "  Default remote: %s\n", displayOrNone(result.Git.DefaultRemote))
	fmt.Fprintf(b, "  Remote URL: %s\n", displayOrNone(result.Git.DefaultRemoteURL))
	fmt.Fprintf(b, "  Remote default branch: %s\n", remoteDefaultBranchLabel(result.Git))
	fmt.Fprintf(b, "  Remotes: %s\n", remotesSummary(result.Git))
	fmt.Fprintf(b, "  gh: %s\n", toolAvailability(result.Tools, "gh"))
	if result.Runtime.ConfigLoads {
		if result.Runtime.Document.SourceControl.DeliveryAssist {
			fmt.Fprintf(b, "  Delivery assist: %s\n", statusInfo.Render("configured (NOT IMPLEMENTED)"))
		} else {
			fmt.Fprintf(b, "  Delivery assist: %s\n", statusNo.Render("not selected"))
		}
		mode := strings.TrimSpace(result.Runtime.Document.SourceControl.Mode)
		if mode == "" {
			mode = "none"
		}
		fmt.Fprintf(b, "  Source-control mode: %s\n", mode)
	} else {
		fmt.Fprintf(b, "  Delivery assist: %s\n", statusNo.Render("n/a"))
	}
	fmt.Fprintln(b)
}

func writeStatusTechnology(b *strings.Builder, result inspect.Inspection) {
	fmt.Fprintln(b, statusHead.Render("Project Technology"))
	if len(result.Technologies) == 0 {
		fmt.Fprintln(b, "  "+statusNo.Render("none detected"))
	} else {
		for _, tech := range result.Technologies {
			fmt.Fprintf(b, "  - %s (%s)\n", tech.Name, tech.Confidence)
		}
	}
	if projectHasGo(result.Technologies) {
		fmt.Fprintf(b, "  go toolchain: %s\n", toolAvailability(result.Tools, "go"))
	}
	if len(result.Libraries) == 0 {
		fmt.Fprintf(b, "  Libraries: %s\n", statusNo.Render("none detected"))
	} else {
		fmt.Fprintln(b, "  Libraries:")
		for _, lib := range result.Libraries {
			fmt.Fprintf(b, "    - %s\n", lib.Name)
		}
	}
	fmt.Fprintln(b)
}

func writeStatusAdapters(b *strings.Builder, result inspect.Inspection) {
	rt := result.Runtime
	fmt.Fprintln(b, statusHead.Render("Adapters"))
	if !rt.ConfigLoads {
		fmt.Fprintln(b, "  "+statusNo.Render("n/a (Atlas not configured)"))
		fmt.Fprintln(b)
		return
	}
	selected := map[string]bool{}
	for _, a := range rt.SelectedAdapters {
		selected[strings.ToLower(strings.TrimSpace(a))] = true
	}
	for _, name := range []string{"cursor", "opencode"} {
		if selected[name] {
			fmt.Fprintf(b, "  %s: %s\n", name, adapterExecutiveStatus(rt, name))
			continue
		}
		fmt.Fprintf(b, "  %s: %s\n", name, statusNo.Render("NOT SELECTED"))
	}
	for _, name := range []string{"claude", "codex"} {
		fmt.Fprintf(b, "  %s: %s\n", name, statusNo.Render("NOT SELECTED"))
	}
	fmt.Fprintln(b)
}

func writeStatusGovernance(b *strings.Builder, result inspect.Inspection) {
	fmt.Fprintln(b, statusHead.Render("Governance Tools"))
	rt := result.Runtime
	if !rt.ConfigLoads {
		fmt.Fprintln(b, "  "+statusNo.Render("n/a (Atlas not configured)"))
		fmt.Fprintln(b)
		return
	}
	gov := rt.Document.Governance
	workflow := strings.TrimSpace(gov.Workflow)
	if workflow == "" {
		workflow = "none"
	}
	engine := strings.TrimSpace(gov.SpecEngine)
	if engine == "" {
		engine = "none"
	}
	fmt.Fprintf(b, "  Workflow: %s\n", workflow)
	fmt.Fprintf(b, "  Spec engine: %s\n", engine)
	fmt.Fprintf(b, "  Testing required: %s\n", yesNo(gov.TestingRequired))
	fmt.Fprintf(b, "  Review required: %s\n", yesNo(gov.ReviewRequired))
	fmt.Fprintf(b, "  Evidence required: %s\n", yesNo(gov.EvidenceRequired))
	fmt.Fprintf(b, "  OpenSpec CLI: %s\n", toolAvailability(result.Tools, "openspec"))
	fmt.Fprintf(b, "  OpenSpec execution: %s\n", statusInfo.Render("NOT IMPLEMENTED"))
	fmt.Fprintln(b)
}

func writeStatusMCP(b *strings.Builder, result inspect.Inspection) {
	fmt.Fprintln(b, statusHead.Render("MCP / External Context"))
	rt := result.Runtime
	if !rt.ConfigLoads {
		fmt.Fprintln(b, "  "+statusNo.Render("n/a (Atlas not configured)"))
		fmt.Fprintln(b)
		return
	}
	mcp := rt.Document.MCP
	writeMCPBuiltin(b, "jira", mcp.Builtins.Jira.Enabled)
	writeMCPBuiltin(b, "context7", mcp.Builtins.Context7.Enabled)
	writeMCPBuiltin(b, "chrome_devtools", mcp.Builtins.ChromeDevTools.Enabled)
	if len(mcp.Custom) == 0 {
		fmt.Fprintf(b, "  Custom MCP: %s\n", statusNo.Render("none"))
	} else {
		for _, custom := range mcp.Custom {
			state := "preference recorded"
			if !custom.Enabled {
				state = "not selected"
			}
			fmt.Fprintf(b, "  Custom %s: %s · %s\n", custom.Name, statusYes.Render(state), statusInfo.Render("connected/authenticated/verified NOT IMPLEMENTED"))
		}
	}
	fmt.Fprintf(b, "  Credentials / auth: %s\n", statusInfo.Render("NOT IMPLEMENTED"))
	fmt.Fprintln(b)
}

func writeStatusHealth(b *strings.Builder, result inspect.Inspection, report doctor.Report) {
	rt := result.Runtime
	pass, warn, errn := report.Counts()
	fmt.Fprintln(b, statusHead.Render("Health"))
	fmt.Fprintf(b, "  PASS: %s   WARNING: %s   ERROR: %s\n",
		statusYes.Render(fmt.Sprintf("%d", pass)),
		statusWarn.Render(fmt.Sprintf("%d", warn)),
		statusFail.Render(fmt.Sprintf("%d", errn)),
	)
	fmt.Fprintf(b, "  Result: %s\n", statusResultStyle(report).Render(report.ResultLabel()))
	if attention := statusNeedsAttention(report, 5); len(attention) > 0 {
		fmt.Fprintln(b, "  Needs attention:")
		for _, line := range attention {
			fmt.Fprintf(b, "    - %s\n", line)
		}
	}
	fmt.Fprintf(b, "  Atlas Home: %s\n", homePresenceLabel(rt))
	fmt.Fprintf(b, "  Context Economy: %s\n", contextEconomyStatus(rt))
	fmt.Fprintf(b, "  Code Intelligence: %s\n", codeIntelligenceStatus(rt))
	fmt.Fprintf(b, "  Suggested next action: %s\n", statusAct.Render(suggestedAction(result)))
}

// statusNeedsAttention returns a compact list of WARNING/ERROR findings for Status.
// Full diagnostic detail stays on Doctor.
func statusNeedsAttention(report doctor.Report, limit int) []string {
	if limit <= 0 {
		return nil
	}
	var out []string
	for _, check := range report.Checks {
		switch check.Severity {
		case doctor.SeverityWarn, doctor.SeverityFail:
			label := "WARNING"
			style := statusWarn
			if check.Severity == doctor.SeverityFail {
				label = "ERROR"
				style = statusFail
			}
			out = append(out, style.Render(label)+" "+check.Name+": "+check.Message)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func statusResultStyle(report doctor.Report) lipgloss.Style {
	_, warnings, failed := report.Counts()
	switch {
	case failed > 0:
		return statusFail
	case warnings > 0:
		return statusWarn
	default:
		return statusYes
	}
}

func writeMCPBuiltin(b *strings.Builder, name string, enabled bool) {
	if enabled {
		fmt.Fprintf(b, "  %s: %s · %s\n", name, statusYes.Render("preference recorded"), statusInfo.Render("connected/authenticated/verified NOT IMPLEMENTED"))
		return
	}
	fmt.Fprintf(b, "  %s: %s\n", name, statusNo.Render("not selected"))
}

func projectDisplayName(result inspect.Inspection) string {
	if result.Runtime.ConfigLoads && strings.TrimSpace(result.Runtime.Document.Project.Name) != "" {
		return result.Runtime.Document.Project.Name
	}
	if result.Atlas.Initialized() && result.Atlas.Config.Project.Name != "" {
		return result.Atlas.Config.Project.Name
	}
	name := filepath.Base(result.RootPath)
	if name == "" || name == "." {
		return "—"
	}
	return name
}

func detectedModeLabel(result inspect.Inspection) string {
	if result.Runtime.ConfigLoads && result.Runtime.Document.Project.Mode != "" {
		return result.Runtime.Document.Project.Mode
	}
	if result.Atlas.Initialized() && result.Atlas.Config.Project.Mode != "" {
		return result.Atlas.Config.Project.Mode
	}
	if result.Files.HasAtlasConfig {
		return "existing"
	}
	if result.RootPath == "" {
		return "unknown"
	}
	entries, err := os.ReadDir(result.RootPath)
	if err != nil {
		return "unknown"
	}
	if len(entries) == 0 {
		return "new"
	}
	return "existing"
}

// SuggestedAction returns the executive next-step hint for Status Health.
func SuggestedAction(result inspect.Inspection) string {
	return suggestedAction(result)
}

func suggestedAction(result inspect.Inspection) string {
	switch result.Atlas.State {
	case project.AtlasStateNotInitialized, project.AtlasStatePartialSetup:
		return "Run Init / Setup"
	case project.AtlasStateInvalidConfig:
		return "Open Doctor"
	case project.AtlasStateInitialized:
		if runtime.BuildRuntimeRepairPlan(result.RootPath, result.Runtime).NeedsApply() {
			return "Review Runtime Repair"
		}
		if result.Runtime.ContextEconomy.Applicable &&
			(result.Runtime.ContextEconomy.State == atlascontext.StatusMissing ||
				result.Runtime.ContextEconomy.State == atlascontext.StatusStale) {
			return "Update Context Economy"
		}
		if len(result.Warnings) > 0 {
			return "Open Doctor"
		}
		return "Open Configure or Doctor"
	default:
		if len(result.Warnings) > 0 {
			return "Open Doctor"
		}
		return "Open Status"
	}
}

func runtimeMaterializedLabel(rt runtime.Health) string {
	if !rt.StateLoads && !rt.Initialized {
		return statusNo.Render("not materialized")
	}
	if rt.RuntimeMaterialized {
		return statusYes.Render("materialized")
	}
	if rt.Initialized {
		return statusWarn.Render("not materialized")
	}
	return statusNo.Render("not materialized")
}

func agentsContractSummary(rt runtime.Health) string {
	switch {
	case !rt.AgentsExists && rt.RuntimeMaterialized:
		return statusFail.Render("missing")
	case !rt.AgentsExists:
		return statusNo.Render("missing")
	case rt.AgentsMarkers.Complete() && rt.AgentsMarkers.ContractSatisfied(rt.SelectedAdapters):
		return statusYes.Render("verified")
	case rt.AgentsExists && rt.RuntimeMaterialized:
		return statusFail.Render("ERROR (markers/adapters)")
	default:
		return statusWarn.Render("WARNING (markers incomplete)")
	}
}

func adapterExecutiveStatus(rt runtime.Health, name string) string {
	var proj *runtime.ProjectionStatus
	for i := range rt.ExpectedProjections {
		if strings.EqualFold(rt.ExpectedProjections[i].Adapter, name) {
			proj = &rt.ExpectedProjections[i]
			break
		}
	}
	agentsOK := true
	agentsPresent := 0
	for _, agent := range rt.ExpectedAgents {
		if !strings.EqualFold(agent.Adapter, name) {
			continue
		}
		agentsPresent++
		if !agent.Present || !agent.Matches {
			agentsOK = false
		}
	}
	switch {
	case proj != nil && proj.Present && agentsOK && (agentsPresent > 0 || len(rt.ExpectedAgents) == 0):
		return statusYes.Render("selected · materialized")
	case proj != nil && !proj.Present:
		return statusFail.Render("selected · missing projection")
	case !agentsOK:
		return statusFail.Render("selected · agent drift")
	default:
		return statusWarn.Render("selected")
	}
}

func remotesSummary(git project.GitInfo) string {
	if !git.IsRepo || len(git.Remotes) == 0 {
		return statusNo.Render("none")
	}
	names := make([]string, 0, len(git.Remotes))
	for _, remote := range git.Remotes {
		names = append(names, remote.Name)
	}
	return strings.Join(names, ", ")
}

// remoteDefaultBranchLabel renders the local view of refs/remotes/<remote>/HEAD.
// "none" means no default remote; "unknown locally" means the remote exists but
// its HEAD symbolic-ref is not configured in this clone.
func remoteDefaultBranchLabel(git project.GitInfo) string {
	if branch := strings.TrimSpace(git.DefaultBranch); branch != "" {
		return branch
	}
	if strings.TrimSpace(git.DefaultRemote) != "" {
		return statusNo.Render("unknown locally")
	}
	return statusNo.Render("none")
}

func toolAvailability(tools []project.ToolInfo, name string) string {
	for _, tool := range tools {
		if tool.Name == name {
			if tool.Available {
				return statusYes.Render("available")
			}
			return statusNo.Render("missing")
		}
	}
	return statusNo.Render("missing")
}

func projectHasGo(techs []project.Technology) bool {
	for _, tech := range techs {
		name := strings.ToLower(tech.Name)
		if name == "go" || strings.Contains(name, "go module") {
			return true
		}
	}
	return false
}

func homePresenceLabel(rt runtime.Health) string {
	switch {
	case !rt.Home.Exists && (rt.Initialized || rt.RuntimeMaterialized):
		return statusFail.Render("missing")
	case !rt.Home.Exists:
		return statusNo.Render("not created")
	case !rt.Home.LayoutComplete || len(rt.Home.MissingAssets) > 0 || len(rt.Home.DriftedAssets) > 0:
		return statusWarn.Render("present (needs attention)")
	default:
		return statusYes.Render("present")
	}
}

func displayOrDash(v string) string {
	if strings.TrimSpace(v) == "" {
		return "—"
	}
	return v
}

func displayOrNone(v string) string {
	if strings.TrimSpace(v) == "" {
		return statusNo.Render("none")
	}
	return v
}

func displayOrUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return v
}

func yesNo(v bool) string {
	if v {
		return statusYes.Render("yes")
	}
	return statusNo.Render("no")
}

func configStatus(rt runtime.Health) string {
	switch {
	case !rt.ConfigExists:
		return statusNo.Render("missing")
	case rt.ConfigLoads:
		return statusYes.Render("configured")
	default:
		return statusFail.Render("ERROR")
	}
}

func stateStatus(rt runtime.Health) string {
	switch {
	case !rt.StateExists:
		if rt.Initialized {
			return statusFail.Render("missing")
		}
		return statusNo.Render("missing")
	case rt.StateLoads:
		return statusYes.Render("configured")
	default:
		return statusFail.Render("ERROR")
	}
}

func sddContractStatus(rt runtime.Health) string {
	if !rt.ConfigLoads || !rt.DependsOnSDDContract {
		return statusNo.Render("n/a")
	}
	switch {
	case !rt.SDDContractPresent:
		return statusFail.Render("missing")
	case !rt.SDDContractMatches:
		return statusFail.Render("ERROR (drifted)")
	default:
		return statusYes.Render("verified")
	}
}

func contextEconomyStatus(rt runtime.Health) string {
	ce := rt.ContextEconomy
	if !ce.Applicable {
		return statusNo.Render("not configured")
	}
	switch ce.State {
	case atlascontext.StatusMissing:
		return statusWarn.Render("missing")
	case atlascontext.StatusStale:
		return statusWarn.Render("WARNING (stale)")
	case atlascontext.StatusUnreadable:
		return statusFail.Render("ERROR (unreadable)")
	case atlascontext.StatusPresent:
		return statusYes.Render("present")
	default:
		return statusNo.Render("n/a")
	}
}

func codeIntelligenceStatus(rt runtime.Health) string {
	snap := rt.CodeIntelligence
	provider := string(snap.Provider)
	if provider == "" {
		provider = "codegraph"
	}
	label := provider + " · " + string(snap.State)
	if snap.Version != "" {
		label += " · " + snap.Version
	}
	if snap.State == "available" || snap.State == "ready" {
		if snap.GraphPresent {
			label += " · graph present"
		} else {
			label += " · graph absent"
		}
	}
	switch snap.State {
	case "available":
		return statusYes.Render(label)
	case "incompatible", "error":
		return statusWarn.Render(label)
	case "unavailable", "missing", "":
		return statusNo.Render(provider + " · unavailable")
	default:
		return statusInfo.Render(label)
	}
}
