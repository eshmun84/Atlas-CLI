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

func (m Model) contentViewportHeight() int {
	// outer borders(2) + header(1) + top divider(1) + bottom divider(1) + footer(1)
	return max(m.height-6, 1)
}

func (m Model) contentWidth() int {
	inner := max(m.width-2, 1)
	side := sidebarWidth(m.width)
	return max(inner-side-1, 1)
}

func (m Model) contentLines() []string {
	raw := m.rawContent()
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
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
		return screens.InitPlan(screens.InitView{
			Plan:            m.plan,
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
			StepConfirmed:   m.initStepConfirmed,
		})
	case RouteConfigure:
		return screens.Configure(m.discovery)
	case RouteStatus:
		return screens.Status(m.discovery)
	case RouteDoctor:
		return screens.Doctor(m.report)
	default:
		return screens.Dashboard(m.discovery)
	}
}

func (m Model) visibleContent() string {
	lines := m.contentLines()
	vis := m.contentViewportHeight()
	if len(lines) == 0 {
		return ""
	}
	start := clampOffset(m.contentOffset, m.maxContentOffset())
	end := start + vis
	if end > len(lines) {
		end = len(lines)
	}
	chunk := strings.Join(lines[start:end], "\n")
	return bodyStyle.Width(m.contentWidth()).Height(vis).MaxHeight(vis).Render(chunk)
}

func (m Model) renderSidebar() string {
	w := sidebarWidth(m.width)
	h := m.contentViewportHeight()
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
	return lipgloss.NewStyle().Width(w).Height(h).MaxHeight(h).Render(strings.TrimRight(b.String(), "\n"))
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

func (m Model) renderFooter() string {
	text := "↑/↓ menu  Enter select  PgUp/PgDn scroll  h help  b dash  q quit"
	switch m.route {
	case RouteError:
		text = "Enter/q/Esc salir"
	case RouteInitPlan:
		text = "Tab focus  ↑/↓ fields  ←/→ edit name  Enter select/Next  r reset  b dash  q quit"
	}
	return footerStyle.Width(max(m.width-2, 1)).Render(text)
}

func (m Model) renderShell() string {
	innerW := max(m.width-2, 1)
	innerH := max(m.height-2, 1)

	header := m.renderHeader()
	footer := m.renderFooter()
	divider := mutedStyle.Render(strings.Repeat("─", innerW))
	middle := lipgloss.JoinHorizontal(lipgloss.Top, m.renderSidebar(), m.visibleContent())

	content := lipgloss.JoinVertical(lipgloss.Left,
		header,
		divider,
		middle,
		divider,
		footer,
	)

	return panelBorder.Width(innerW).Height(innerH).Render(content)
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
	return panelBorder.Width(w).Height(h).Render(lipgloss.NewStyle().Width(w).Height(h).Render(msg))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
