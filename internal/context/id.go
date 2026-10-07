package context

import "github.com/eshmun84/Atlas-CLI/internal/home"

// ProjectID returns a stable Home project identity for a project root.
// Delegates to home.ProjectID so Context Economy and Atlas Home share one model.
func ProjectID(root, projectName string) (string, error) {
	return home.ProjectID(root, projectName)
}
