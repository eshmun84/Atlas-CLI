package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

func (m Model) chromeHeight() int {
	// header + top divider + bottom divider + footer (+ action row when present)
	h := 1 + 1 + 1 + m.footerHeight()
	if ah := m.actionHeight(); ah > 0 {
		h += ah
	}
	return h
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
	sidebarH := max(lipgloss.Height(m.renderSidebarRaw()), 1)
	return max(contentH, sidebarH)
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
	return m.middleHeight()
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
	case RouteDashboard:
		return screens.Dashboard(m.discovery)
	case RouteInitPlan:
		if m.initWizardStep == screens.InitWizardStepConfig {
			var confirm []string
			if m.initConfigConfirmed {
				confirm = []string{
					"Configuration ready.",
					"Review / Materialization Plan is not implemented in this slice.",
					"No files were changed.",
				}
			}
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
				Confirmed:      m.initConfigConfirmed,
				ConfirmLines:   confirm,
				ShowBack:       true,
				ShowNext:       true,
				BackLabel:      "Back",
				NextLabel:      "Next",
				FooterNote:     "No files will be changed in this slice.",
				Width:          m.contentWidth(),
			})
		}
		return screens.InitPlan(screens.InitView{
			RootPath:        m.discovery.RootPath,
			DetectedName:    m.detectedName,
			DetectedMode:    m.detectedMode,
			RecommendedMode: string(m.recommendedMode),
			DraftName:       m.nameInput.Value(),
			NameInputView:   m.nameInput.View(),
			ModeConfirmed:   string(m.initModeConfirmed),
			Decision:        string(m.initDecision),
			Artifacts:       m.discovery.RuntimeArtifacts,
			ActiveField:     m.initField,
			ContentFocused:  m.focus == FocusContent,
		})
	case RouteConfigure:
		if len(m.configDraft.Sections) == 0 {
			return screens.ConfigureFallback(m.discovery.Atlas.State, m.discovery.Atlas.ConfigPath)
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
			ShowNext:       false,
			BackLabel:      "Close",
			Width:          m.contentWidth(),
		})
	case RouteMCP:
		return screens.RenderMCP(screens.MCPView{
			Mode:           m.mcpMode,
			Draft:          m.mcpDraft,
			ActiveIndex:    m.mcpIndex,
			KindFocus:      m.mcpKindFocus,
			KindSelected:   m.mcpKind,
			Initialized:    m.Initialized(),
			ContentFocused: m.focus == FocusContent,
			ListFocus:      m.mcpListFocus,
			AddFocus:       m.mcpAddFocus,
			NameView:       m.mcpNameInput.View(),
			ConnectionView: m.mcpConnInput.View(),
			Error:          m.mcpAddError,
		})
	case RouteStatus:
		return screens.Status(m.discovery)
	case RouteDoctor:
		return screens.Doctor(m.report)
	default:
		return screens.Dashboard(m.discovery)
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
	width := max(m.width-2, 1)
	switch {
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
	case m.route == RouteInitPlan && m.initWizardStep == screens.InitWizardStepConfig:
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Back",
			NextLabel:      "Next",
			Confirmed:      m.initConfigConfirmed,
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     m.configPanel,
			FooterIndex:    m.configFooterIdx,
			Width:          width,
		}), true
	case m.route == RouteConfigure && len(m.configDraft.Sections) > 0:
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       false,
			BackLabel:      "Close",
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     m.configPanel,
			FooterIndex:    m.configFooterIdx,
			Width:          width,
		}), true
	case m.route == RouteMCP && m.mcpMode == screens.MCPModeList:
		panel := ""
		if m.focus == FocusContent && (m.mcpListFocus == screens.MCPFocusAddBtn || m.mcpListFocus == screens.MCPFocusClose) {
			panel = screens.ConfigPanelFooter
		}
		return screens.RenderActionFooter(screens.ActionFooterView{
			ShowBack:       true,
			ShowNext:       true,
			BackLabel:      "Add MCP",
			NextLabel:      "Close",
			ContentFocused: m.focus == FocusContent,
			PanelFocus:     panel,
			FooterIndex:    m.mcpFooterIdx,
			Width:          width,
		}), true
	case m.route == RouteMCP && m.mcpMode == screens.MCPModeAdd:
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
	default:
		return "", false
	}
}

func (m Model) renderFooter() string {
	text := "↑/↓ menu  Enter select  PgUp/PgDn scroll  h help  b dash  q quit"
	switch m.route {
	case RouteError:
		text = "Enter/q/Esc salir"
	case RouteInitPlan:
		if m.initWizardStep == screens.InitWizardStepConfig {
			text = "Tab focus  ↑/↓ rows  ←/→ sections  Space/Enter select  b dash  q quit"
		} else {
			text = "Tab focus  ↑/↓ fields  ←/→ edit name  Enter Next  r reset  b dash  q quit"
		}
	case RouteConfigure:
		text = "Tab focus  ↑/↓ rows  Space/Enter select  Close  b dash  q quit"
	case RouteMCP:
		text = "Tab focus  ↑/↓  Space/Enter  Add MCP  b dash  q quit"
	case RouteHelp:
		text = "↑/↓ menu  Enter select  PgUp/PgDn scroll  b dash  q quit"
	}
	return text
}

func (m Model) renderShell() string {
	innerW := max(m.width-2, 1)

	header := m.renderHeader()
	footer := m.renderFooter()
	action, hasAction := m.renderActionRow()
	divider := mutedStyle.Render(strings.Repeat("─", innerW))
	middleH := m.middleHeight()

	sidebar := lipgloss.NewStyle().Width(sidebarWidth(m.width)).Height(middleH).MaxHeight(middleH).
		Render(strings.TrimRight(m.renderSidebarRaw(), "\n"))
	chunk := m.visibleContentChunk(middleH)
	contentStyle := bodyStyle.Width(m.contentWidth())
	if !m.contentFitsTerminal() {
		contentStyle = contentStyle.Height(middleH).MaxHeight(middleH)
	}
	content := contentStyle.Render(chunk)
	middle := lipgloss.JoinHorizontal(lipgloss.Top, sidebar, content)

	parts := []string{header, divider, middle}
	if hasAction {
		parts = append(parts, strings.TrimRight(action, "\n"))
	}
	parts = append(parts, divider, footer)
	body := lipgloss.JoinVertical(lipgloss.Top, parts...)

	if m.contentFitsTerminal() {
		return panelBorder.Width(innerW).Render(body)
	}
	innerH := max(m.height-2-m.topGap(), 1)
	return panelBorder.Width(innerW).Height(innerH).AlignVertical(lipgloss.Top).Render(body)
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
