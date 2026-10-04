package screens

import "fmt"

// ConfigureView renders post-init configuration using ConfigDraft.
func ConfigureView(view ConfigFormView) string {
	if view.Title == "" {
		view.Title = "Configure"
	}
	if view.Subtitle == "" {
		view.Subtitle = "Post-init configuration · in memory only"
	}
	view.ShowBack = true
	view.ShowNext = false
	if view.BackLabel == "" {
		view.BackLabel = "Close"
	}
	if view.FooterNote == "" {
		view.FooterNote = "Apply Configuration Changes is not implemented in this slice. No files were changed."
	}
	return RenderConfigForm(view)
}

// ConfigureFallback renders a minimal message when no draft is available.
func ConfigureFallback(state, configPath string) string {
	return fmt.Sprintf("%s\n\n  State: %s\n  Config path: %s\n\n  %s",
		cfgFormTitle.Render("Configure"),
		state,
		configPath,
		cfgFormMuted.Render("No configuration draft available."),
	)
}
