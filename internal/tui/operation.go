package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/runtime"
)

// Blocking TUI operations that show a loader and reject duplicate Apply.
type opKind string

const (
	opNone           opKind = ""
	opConfigureApply opKind = "configure"
	opInitApply      opKind = "init"
	opRuntimeRepair  opKind = "repair"
)

type operationDoneMsg struct {
	Kind     opKind
	Err      error
	Duration time.Duration

	ConfigureNotice string
	InitNotice      string
	RepairResult    runtime.RuntimeRepairResult
	RepairMessage   string
	RepairStale     bool
	RepairBlocked   bool
}

func newOpSpinner() spinner.Model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	return s
}

func (m Model) operationActive() bool {
	return m.opActive
}

func (m Model) beginOperation(kind opKind, title, detail string) (Model, tea.Cmd) {
	if m.opActive {
		return m, nil
	}
	m.opActive = true
	m.opKind = kind
	m.opTitle = title
	m.opDetail = detail
	m.opStarted = time.Now()
	m.spinner = newOpSpinner()
	return m, tea.Batch(m.spinner.Tick, m.runOperationCmd(kind))
}

func (m Model) clearOperation() Model {
	m.opActive = false
	m.opKind = opNone
	m.opTitle = ""
	m.opDetail = ""
	m.opStarted = time.Time{}
	return m
}

func (m Model) runOperationCmd(kind opKind) tea.Cmd {
	root := m.discovery.RootPath
	draft := m.configDraft
	mcpDraft := m.mcpDraft
	acceptHome := m.initAcceptHomeReset
	repairSig := m.repairSignature
	started := m.opStarted
	if started.IsZero() {
		started = time.Now()
	}
	return func() tea.Msg {
		start := started
		switch kind {
		case opConfigureApply:
			result, err := config.PersistConfigure(config.ApplyInput{
				Root:  root,
				Draft: draft,
				MCP:   mcpDraft,
			})
			dur := time.Since(start)
			if err != nil {
				return operationDoneMsg{Kind: kind, Err: err, Duration: dur}
			}
			return operationDoneMsg{
				Kind:            kind,
				Duration:        dur,
				ConfigureNotice: formatConfigureOpSuccess(draft, mcpDraft, result, dur),
			}
		case opInitApply:
			_, err := config.ApplyConfig(config.ApplyInput{
				Root:            root,
				Draft:           draft,
				MCP:             mcpDraft,
				AcceptHomeReset: acceptHome,
			})
			dur := time.Since(start)
			if err != nil {
				return operationDoneMsg{Kind: kind, Err: err, Duration: dur}
			}
			return operationDoneMsg{
				Kind:       kind,
				Duration:   dur,
				InitNotice: formatInitOpSuccess(draft, mcpDraft, dur),
			}
		case opRuntimeRepair:
			result, err := runtime.ApplyRuntimeRepair(root, repairSig, nil)
			dur := time.Since(start)
			msg := operationDoneMsg{Kind: kind, Duration: dur, RepairResult: result}
			if err != nil {
				msg.Err = err
				msg.RepairMessage = err.Error()
				msg.RepairBlocked = result.Blocked
				return msg
			}
			if result.Stale {
				msg.RepairStale = true
				msg.RepairMessage = runtime.RepairStaleMessage
				return msg
			}
			msg.RepairMessage = formatRepairOpSuccess(result, dur)
			return msg
		default:
			return operationDoneMsg{Kind: kind, Err: fmt.Errorf("unknown operation"), Duration: time.Since(start)}
		}
	}
}

func (m Model) operationStatusView() string {
	if !m.opActive {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n", m.spinner.View(), m.opTitle)
	if m.opDetail != "" {
		fmt.Fprintf(&b, "\nCurrent operation:\n%s\n", m.opDetail)
	}
	return strings.TrimRight(b.String(), "\n")
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func formatConfigureOpSuccess(draft config.ConfigDraft, mcp config.MCPDraft, result config.PersistConfigureResult, dur time.Duration) string {
	adapters := draftAdaptersSummary(draft)
	gov := formatGovernanceSummary(draft)
	mcpLine := formatMCPSummary(mcp)
	lines := []string{
		"Configuration applied successfully",
		"",
		"Adapter: " + adapters,
		"Governance: " + gov,
		"MCP: " + mcpLine,
		"",
		"Completed in " + formatDuration(dur),
	}
	if notice := strings.TrimSpace(result.Notice); notice != "" && notice != config.ConfigureApplySuccessTitle {
		// Keep technical impact lines after the executive summary.
		lines = append(lines, "", notice)
	}
	return strings.Join(lines, "\n")
}

func formatInitOpSuccess(draft config.ConfigDraft, mcp config.MCPDraft, dur time.Duration) string {
	// Title/body are rendered by the Review screen when Applied; keep extras here.
	return strings.Join([]string{
		"Adapter: " + draftAdaptersSummary(draft),
		"Governance: " + formatGovernanceSummary(draft),
		"MCP: " + formatMCPSummary(mcp),
		"",
		"Completed in " + formatDuration(dur),
	}, "\n")
}

func draftAdaptersSummary(draft config.ConfigDraft) string {
	if f, ok := draft.FieldByKey("adapters.selected"); ok {
		if v := strings.TrimSpace(f.Value); v != "" {
			return v
		}
	}
	return "(none)"
}

func formatRepairOpSuccess(result runtime.RuntimeRepairResult, dur time.Duration) string {
	title := result.MessageTitle
	if title == "" {
		title = runtime.RepairSuccessTitle
	}
	return strings.Join([]string{
		title,
		"",
		"Completed in " + formatDuration(dur),
	}, "\n")
}

func formatOpFailure(kind opKind, err error, dur time.Duration) string {
	title := "Operation failed"
	opLabel := "Apply"
	switch kind {
	case opConfigureApply:
		title = "Configuration failed"
		opLabel = "Configure Apply"
	case opInitApply:
		title = "Initialization failed"
		opLabel = "Init Apply"
	case opRuntimeRepair:
		title = "Runtime Repair failed"
		opLabel = "Runtime Repair"
	}
	reason := strings.TrimSpace(err.Error())
	operation := classifyOpFailure(kind, reason)
	lines := []string{
		title,
		"",
		"Operation:",
		operation,
		"",
		"Reason:",
		reason,
		"",
		"Completed in " + formatDuration(dur),
	}
	if strings.Contains(reason, "rolled back to baseline") || strings.Contains(reason, "(rolled back)") {
		lines = append(lines, "", "Rollback:", "Completed successfully")
	} else if strings.Contains(reason, "rollback failed") {
		lines = append(lines, "", "Rollback:", "Failed — manual inspection required")
	}
	_ = opLabel
	return strings.Join(lines, "\n")
}

func classifyOpFailure(kind opKind, reason string) string {
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "assets lock") || strings.Contains(lower, "assets.lock"):
		return "Assets lock update"
	case strings.Contains(lower, "adapter cleanup") || strings.Contains(lower, "adapter runtime"):
		return "Adapter reconciliation"
	case strings.Contains(lower, "mcp"):
		return "MCP projection"
	case strings.Contains(lower, "openspec") || strings.Contains(lower, "provider"):
		return "OpenSpec provider sync"
	case strings.Contains(lower, "skills"):
		return "Skills projection"
	case strings.Contains(lower, "hooks"):
		return "Hooks projection"
	case kind == opRuntimeRepair:
		return "Runtime Repair"
	case kind == opInitApply:
		return "Init Apply"
	default:
		return "Configure Apply"
	}
}

func formatGovernanceSummary(draft config.ConfigDraft) string {
	workflow := draftFieldValue(draft, "governance.workflow")
	engine := draftFieldValue(draft, "governance.spec_engine")
	switch {
	case workflow != "" && engine != "":
		return workflow + " / " + engine
	case workflow != "":
		return workflow
	case engine != "":
		return engine
	default:
		return "(unchanged)"
	}
}

func formatMCPSummary(mcp config.MCPDraft) string {
	var parts []string
	for _, b := range mcp.Builtins {
		if !b.Enabled {
			continue
		}
		parts = append(parts, b.Name)
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func draftFieldValue(draft config.ConfigDraft, key string) string {
	if f, ok := draft.FieldByKey(key); ok {
		return strings.TrimSpace(f.Value)
	}
	return ""
}
