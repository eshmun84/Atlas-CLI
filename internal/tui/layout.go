package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/tui/screens"
)

func tooSmall(width, height int) bool {
	return width > 0 && height > 0 && (width < MinWidth || height < MinHeight)
}

func sidebarWidth(totalWidth int) int {
	if totalWidth >= 100 {
		return 24
	}
	return 20
}

func (m Model) footerHeight() int {
	h := lipgloss.Height(strings.TrimRight(m.renderFooter(), "\n"))
	if h < 1 {
		return 1
	}
	return h
}

func (m Model) actionHeight() int {
	action, ok := m.renderActionRow()
	if !ok {
		return 0
	}
	return max(lipgloss.Height(strings.TrimRight(action, "\n")), 1)
}

func (m Model) actionChromeHeight() int {
	if m.actionHeight() == 0 {
		return 0
	}
	// blank line above the action row + the row + blank line below
	return 1 + m.actionHeight() + 1
}

func (m Model) chromeHeight() int {
	// header + top divider + bottom divider + footer
	return 1 + 1 + 1 + m.footerHeight()
}

func (m Model) maxMiddleHeight() int {
	return max(m.height-2-m.chromeHeight()-m.topGap(), 1)
}

func (m Model) wrappedContent() string {
	raw := strings.TrimRight(m.rawContent(), "\n")
	if raw == "" {
		return ""
	}
	return bodyStyle.Width(m.contentWidth()).Render(raw)
}

func (m Model) naturalMiddleHeight() int {
	contentH := max(lipgloss.Height(m.wrappedContent()), 1)
	rightH := contentH + m.actionChromeHeight()
	sidebarH := max(lipgloss.Height(m.renderSidebarRaw()), 1)
	return max(rightH, sidebarH)
}

func (m Model) middleHeight() int {
	natural := m.naturalMiddleHeight()
	limit := m.maxMiddleHeight()
	if natural > limit {
		return limit
	}
	return natural
}

func (m Model) contentFitsTerminal() bool {
	return m.naturalMiddleHeight() <= m.maxMiddleHeight()
}

func (m Model) contentViewportHeight() int {
	if m.route == RouteError {
		return max(m.height-5-m.footerHeight(), 1)
	}
	return max(m.middleHeight()-m.actionChromeHeight(), 1)
}

func (m Model) contentWidth() int {
	inner := max(m.width-2, 1)
	side := sidebarWidth(m.width)
	return max(inner-side-1, 1)
}

func (m Model) contentLines() []string {
	wrapped := m.wrappedContent()
	if wrapped == "" {
		return nil
	}
	return strings.Split(wrapped, "\n")
}

func (m Model) maxContentOffset() int {
	lines := len(m.contentLines())
	vis := m.contentViewportHeight()
	if lines <= vis {
		return 0
	}
	return lines - vis
}

func clampOffset(offset, maxOff int) int {
	if offset < 0 {
		return 0
	}
	if offset > maxOff {
		return maxOff
	}
	return offset
}

func (m Model) rawContent() string {
	if m.loadErr != nil {
		return failBadge.Render("ERROR") + "  " + m.loadErr.Error()
	}
	if !m.ready {
		return mutedStyle.Render("Loading workspace…")
	}
	switch m.route {
	case RouteHelp:
		return screens.Help()
	case RouteInitPlan:
		if m.initWizardStep == screens.InitWizardStepReview {
			return screens.RenderReview(screens.ReviewView{
				Plan:           m.initReviewPlan,
				ApplyMessage:   m.initReviewMessage,
				Applied:        m.initApplied,
				ContentFocused: m.focus == FocusContent,
			})
		}
		if m.initWizardStep == screens.InitWizardStepConfig {
			return screens.RenderConfigForm(screens.ConfigFormView{
				Title:          "Initial Configuration",
				Subtitle:       "Step 2 — Initial Configuration",
				Draft:          m.configDraft,
				SectionIndex:   m.configSectionIdx,
				FieldIndex:     m.configFieldIdx,
				OptionIndex:    m.configOptionIdx,
				PanelFocus:     m.configPanel,
				FooterIndex:    m.configFooterIdx,
				ContentFocused: m.focus == FocusContent,
				ShowBack:       true,
				ShowNext:       true,
				BackLabel:      "Back",
				NextLabel:      "Next",
				FooterNote:     "No files are written until Review → Apply.",
				Width:          m.contentWidth(),
				MCP:            m.mcpView(),
			})
		}
		if m.initWizardStep == screens.InitWizardStepConflict {
			return screens.InitConflict(screens.InitConflictView{
				RootPath:       m.discovery.RootPath,
				Artifacts:      m.discovery.RuntimeArtifacts,
				ActiveField:    m.initField,
				ContentFocused: m.focus == FocusContent,
			})
		}
		return screens.InitPlan(screens.InitView{
			RootPath:       m.discovery.RootPath,
			DetectedName:   m.detectedName,
			DetectedMode:   m.detectedMode,
			AtlasState:     m.discovery.Atlas.State,
			DraftName:      m.nameInput.Value(),
			NameInputView:  m.nameInput.View(),
			ModeConfirmed:  string(m.initModeConfirmed),
			ActiveField:    m.initField,
			ContentFocused: m.focus == FocusContent,
		})
	case RouteConfigure:
		if len(m.configDraft.Sections) == 0 {
			return screens.ConfigureFallback(m.discovery.Atlas.State, m.discovery.Atlas.ConfigPath)
		}
		note := config.ConfigureApplyFooterNote
		if m.configureNotice != "" {
			note = m.configureNotice
		}
		return screens.ConfigureView(screens.ConfigFormView{
			Draft:          m.configDraft,
			SectionIndex:   m.configSectionIdx,
			FieldIndex:     m.configFieldIdx,
			OptionIndex:    m.configOptionIdx,
			PanelFocus:     m.configPanel,
			FooterIndex:    m.configFooterIdx,
			ContentFocused: m.focus == FocusContent,
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Close",
			NextLabel:      "Apply changes",
			FooterNote:     note,
			Width:          m.contentWidth(),
			MCP:            m.mcpView(),
		})
	case RouteStatus:
		// Use the same doctor.Report snapshot as Doctor so Health counts match.
		return screens.StatusWithReport(m.discovery, m.report)
	case RouteDoctor:
		return screens.Doctor(m.report, m.discovery)
	case RouteRuntimeRepair:
		return screens.RenderRuntimeRepair(screens.RepairView{
			Plan:           m.repairPlan,
			Applied:        m.repairApplied,
			ApplyMessage:   m.repairMessage,
			ContentFocused: m.focus == FocusContent,
		})
	case RouteContextEconomy:
		return screens.RenderContextEconomy(screens.ContextEconomyView{
			Plan:           m.contextPlan,
			Applied:        m.contextApplied,
			ApplyMessage:   m.contextMessage,
			ContentFocused: m.focus == FocusContent,
		})
	default:
		return screens.Status(m.discovery)
	}
}

func (m Model) renderHeader() string {
	inner := max(m.width-2, 1)
	left := titleStyle.Render("Atlas")
	right := mutedStyle.Render(fmt.Sprintf("Project: %s   Route: %s", m.ProjectName(), m.route.String()))
	gap := inner - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m Model) renderActionRow() (string, bool) {
	width := m.contentWidth()
	switch {
	case m.route == RouteInitPlan && m.initWizardStep == screens.InitWizardStepConflict:
		panel := ""
		footerIdx := 0
		if m.focus == FocusContent {
			panel = screens.ConfigPanelFooter
			if m.initField == screens.InitFieldConflictExit {
				footerIdx = 0
			} else {
				footerIdx = 1
			}
		}
		nextLabel := "Refresh / Re-check"
		if !m.hasRuntimeArtifacts() {
			nextLabel = "Continue to Setup"
		}
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Exit / Back",
			NextLabel:      nextLabel,
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     panel,
			FooterIndex:    footerIdx,
			Width:          width,
		}), true
	case m.route == RouteInitPlan && m.initWizardStep == screens.InitWizardStepProject:
		panel := ""
		if m.focus == FocusContent && m.initField == screens.InitFieldNext {
			panel = screens.ConfigPanelFooter
		}
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       false,
			ShowNext:       true,
			NextLabel:      "Next",
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     panel,
			FooterIndex:    0,
			Width:          width,
		}), true
	case m.route == RouteInitPlan && m.initWizardStep == screens.InitWizardStepConfig && m.mcpMode == screens.MCPModeAdd:
		panel := ""
		if m.focus == FocusContent && (m.mcpAddFocus == screens.MCPFocusCancel || m.mcpAddFocus == screens.MCPFocusSubmit) {
			panel = screens.ConfigPanelFooter
		}
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Cancel",
			NextLabel:      "Add",
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     panel,
			FooterIndex:    m.mcpFooterIdx,
			Width:          width,
		}), true
	case m.route == RouteInitPlan && m.initWizardStep == screens.InitWizardStepConfig:
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Back",
			NextLabel:      "Next",
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     m.configPanel,
			FooterIndex:    m.configFooterIdx,
			Width:          width,
		}), true
	case m.route == RouteInitPlan && m.initWizardStep == screens.InitWizardStepReview:
		panel := ""
		if m.focus == FocusContent {
			panel = screens.ConfigPanelFooter
		}
		if m.initApplied {
			return screens.RenderActionFooter(screens.ActionFooterView{
				ShowBack:       true,
				ShowNext:       false,
				BackLabel:      "Close",
				ContentFocused: m.focus == FocusContent,
				PanelFocus:     panel,
				FooterIndex:    0,
				Width:          width,
			}), true
		}
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Back",
			NextLabel:      "Apply config",
			NextDisabled:   false,
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     panel,
			FooterIndex:    m.initReviewFooterIdx,
			Width:          width,
		}), true
	case m.route == RouteConfigure && m.mcpMode == screens.MCPModeAdd:
		panel := ""
		if m.focus == FocusContent && (m.mcpAddFocus == screens.MCPFocusCancel || m.mcpAddFocus == screens.MCPFocusSubmit) {
			panel = screens.ConfigPanelFooter
		}
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Cancel",
			NextLabel:      "Add",
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     panel,
			FooterIndex:    m.mcpFooterIdx,
			Width:          width,
		}), true
	case m.route == RouteConfigure && len(m.configDraft.Sections) > 0:
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Close",
			NextLabel:      "Apply changes",
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     m.configPanel,
			FooterIndex:    m.configFooterIdx,
			Width:          width,
		}), true
	case m.route == RouteRuntimeRepair:
		panel := ""
		if m.focus == FocusContent {
			panel = screens.ConfigPanelFooter
		}
		if m.repairApplied || m.repairPlan.Blocked || !m.repairPlan.NeedsApply() {
			return screens.RenderActionFooter(screens.ActionFooterView{
				ShowBack:       true,
				ShowNext:       false,
				BackLabel:      "Close",
				ContentFocused: m.focus == FocusContent,
				PanelFocus:     panel,
				FooterIndex:    0,
				Width:          width,
			}), true
		}
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Close",
			NextLabel:      "Apply repair",
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     panel,
			FooterIndex:    m.repairFooterIdx,
			Width:          width,
		}), true
	case m.route == RouteContextEconomy:
		panel := ""
		if m.focus == FocusContent {
			panel = screens.ConfigPanelFooter
		}
		if m.contextApplied || m.contextPlan.Blocked || !m.contextPlan.NeedsApply() {
			return screens.RenderActionFooter(screens.ActionFooterView{
				ShowBack:       true,
				ShowNext:       false,
				BackLabel:      "Close",
				ContentFocused: m.focus == FocusContent,
				PanelFocus:     panel,
				FooterIndex:    0,
				Width:          width,
			}), true
		}
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Close",
			NextLabel:      "Update context",
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     panel,
			FooterIndex:    m.contextFooterIdx,
			Width:          width,
		}), true
	default:
		return "", false
	}
}

func (m Model) renderFooter() string {
	text := "↑/↓ menu  Enter select  PgUp/PgDn scroll  h help  b status  q quit"
	switch m.route {
	case RouteError:
		text = "Enter/q/Esc salir"
	case RouteInitPlan:
		switch m.initWizardStep {
		case screens.InitWizardStepConflict:
			if m.hasRuntimeArtifacts() {
				text = "Tab focus  Refresh / Re-check  Exit / Back  b status  q quit"
			} else {
				text = "Tab focus  Continue to Setup  Exit / Back  b status  q quit"
			}
		case screens.InitWizardStepConfig:
			if m.mcpMode == screens.MCPModeAdd {
				text = "Tab focus  ↑/↓  Space/Enter  Add MCP  b status  q quit"
			} else if m.mcpSectionActive() {
				text = "Tab focus  ↑/↓ rows  ←/→ sections  Space/Enter  d remove custom  b status  q quit"
			} else {
				text = "Tab focus  ↑/↓ rows  ←/→ sections  Space/Enter select  b status  q quit"
			}
		case screens.InitWizardStepReview:
			if m.initApplied {
				text = "Tab focus  PgUp/PgDn scroll  Close  b status  q quit"
			} else if m.initReviewPlan.HomeDataDetected {
				text = "Tab focus  PgUp/PgDn scroll  x accept Home reset  Back  Apply config  b status  q quit"
			} else {
				text = "Tab focus  PgUp/PgDn scroll  Back  Apply config  b status  q quit"
			}
		default:
			text = "Tab focus  ↑/↓ fields  ←/→ mode  Space/Enter select  r reset  b status  q quit"
		}
	case RouteConfigure:
		if m.mcpMode == screens.MCPModeAdd {
			text = "Tab focus  ↑/↓  Space/Enter  Add MCP  b status  q quit"
		} else if m.mcpSectionActive() {
			text = "Tab focus  ↑/↓ rows  ←/→ sections  Space/Enter  d remove custom  Apply changes  b status  q quit"
		} else {
			text = "Tab focus  ↑/↓ rows  Space/Enter select  Close  Apply changes  b status  q quit"
		}
	case RouteHelp:
		text = "↑/↓ menu  Enter select  PgUp/PgDn scroll  b status  q quit"
	case RouteRuntimeRepair:
		if m.repairApplied || m.repairPlan.Blocked || !m.repairPlan.NeedsApply() {
			text = "Tab focus  PgUp/PgDn scroll  Close  b status  q quit"
		} else {
			text = "Tab focus  PgUp/PgDn scroll  Close  Apply repair  b status  q quit"
		}
	case RouteContextEconomy:
		if m.contextApplied || m.contextPlan.Blocked || !m.contextPlan.NeedsApply() {
			text = "Tab focus  PgUp/PgDn scroll  Close  b status  q quit"
		} else {
			text = "Tab focus  PgUp/PgDn scroll  Close  Update context  b status  q quit"
		}
	}
	return text
}

func (m Model) renderShell() string {
	innerW := max(m.width-2, 1)

	header := m.renderHeader()
	footer := m.renderFooter()
	divider := mutedStyle.Render(strings.Repeat("─", innerW))
	middleH := m.middleHeight()
	sideW := sidebarWidth(m.width)
	contentW := m.contentWidth()

	sidebar := lipgloss.NewStyle().Width(sideW).Height(middleH).MaxHeight(middleH).
		Render(strings.TrimRight(m.renderSidebarRaw(), "\n"))

	right := m.renderRightPanel(contentW, m.contentViewportHeight(), middleH)
	content := lipgloss.NewStyle().Width(contentW).Height(middleH).AlignVertical(lipgloss.Top).Render(right)
	middle := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)

	body := lipgloss.JoinVertical(lipgloss.Top, header, divider, middle, divider, footer)

	if m.contentFitsTerminal() {
		return panelBorder.Width(innerW).Render(body)
	}
	innerH := max(m.height-2-m.topGap(), 1)
	return panelBorder.Width(innerW).Height(innerH).AlignVertical(lipgloss.Top).Render(body)
}

func (m Model) composeRightPanel(chunk string) string {
	chunk = strings.TrimRight(chunk, "\n")
	action, ok := m.renderActionRow()
	if !ok {
		return chunk
	}
	action = strings.TrimRight(action, "\n")
	if chunk == "" {
		return action + "\n"
	}
	return chunk + "\n\n" + action + "\n"
}

// renderRightPanel keeps the action row outside content MaxHeight clipping so
// Configure [ Apply changes ] stays visible under tall MCP content.
func (m Model) renderRightPanel(contentW, viewportH, middleH int) string {
	chunk := strings.TrimRight(m.visibleContentChunk(viewportH), "\n")
	action, ok := m.renderActionRow()
	if !ok {
		return bodyStyle.Width(contentW).Height(middleH).MaxHeight(middleH).Render(chunk)
	}
	action = strings.TrimRight(action, "\n")
	styledChunk := bodyStyle.Width(contentW).Height(viewportH).MaxHeight(viewportH).AlignVertical(lipgloss.Top).Render(chunk)
	return strings.TrimRight(styledChunk, "\n") + "\n\n" + action + "\n"
}

func (m Model) renderSidebarRaw() string {
	w := sidebarWidth(m.width)
	items := m.Sidebar()
	var b strings.Builder
	for i, item := range items {
		label := "  " + item.Label
		cursor := " "
		style := sidebarItemStyle
		if i == m.sidebarIndex {
			cursor = "›"
			if m.focus == FocusSidebar {
				style = sidebarSelectedStyle
				label = " " + item.Label + " "
			} else {
				style = sidebarIdleSelectedStyle
				label = " " + item.Label
			}
		} else if !item.Exit && item.Route == m.route {
			style = sidebarActiveStyle
		}
		line := style.Render(cursor + label)
		b.WriteString(lipgloss.NewStyle().Width(w).Render(line))
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (m Model) visibleContentChunk(vis int) string {
	lines := m.contentLines()
	if len(lines) == 0 {
		return ""
	}
	maxOff := 0
	if len(lines) > vis {
		maxOff = len(lines) - vis
	}
	start := clampOffset(m.contentOffset, maxOff)
	end := start + vis
	if end > len(lines) {
		end = len(lines)
	}
	return strings.Join(lines[start:end], "\n")
}

func renderSmallTerminal(width, height int) string {
	msg := titleStyle.Render("Atlas") + "\n\n" +
		bodyStyle.Render("Atlas needs a larger terminal.") + "\n" +
		mutedStyle.Render("Minimum recommended size: 80x24.") + "\n\n" +
		footerStyle.Render("q / ctrl+c / esc quit")

	w := max(width-2, 1)
	h := max(height-2, 1)
	if width <= 2 || height <= 2 {
		return "Atlas needs a larger terminal.\nMinimum recommended size: 80x24."
	}
	return panelBorder.Width(w).Height(h).AlignVertical(lipgloss.Top).Render(lipgloss.NewStyle().Width(w).Render(msg))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m Model) topGap() int {
	if m.height <= MinHeight {
		return 0
	}
	return 1
}

func (m Model) withFrame(panel string) string {
	gap := m.topGap()
	panelH := lipgloss.Height(panel)
	if gap > 0 && panelH+gap > m.height {
		gap = 0
	}
	avail := max(m.height-gap, 1)
	framed := panel
	if panelH < avail {
		framed = lipgloss.Place(max(m.width, 1), avail, lipgloss.Left, lipgloss.Top, panel)
	}
	if gap > 0 {
		return "\n" + framed
	}
	return framed
}
