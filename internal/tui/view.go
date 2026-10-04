package tui

// View renders the sidebar shell, or the Error layout with global header/footer.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if tooSmall(m.width, m.height) {
		return renderSmallTerminal(m.width, m.height)
	}
	if m.route == RouteError {
		return m.withFrame(m.renderErrorLayout())
	}
	return m.withFrame(m.renderShell())
}
