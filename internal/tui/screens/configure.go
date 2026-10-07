package screens

import (
	"fmt"

	"github.com/eshmun84/Atlas-CLI/internal/config"
)

// ConfigureView renders post-init configuration using ConfigDraft.
func ConfigureView(view ConfigFormView) string {
	if view.Title == "" {
		view.Title = "Configure"
	}
	if view.Subtitle == "" {
		view.Subtitle = "Saves config.yaml — Runtime Repair / Context Economy are separate"
	}
	view.ShowBack = true
	view.ShowNext = true
	if view.BackLabel == "" {
		view.BackLabel = "Close"
	}
	if view.NextLabel == "" {
		view.NextLabel = "Apply changes"
	}
	if view.FooterNote == "" {
		view.FooterNote = config.FormatConfigureFooterNote(view.Draft)
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
