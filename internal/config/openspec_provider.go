package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

// OpenSpecRequired reports whether this document requires a real OpenSpec provider.
func OpenSpecRequired(doc ProjectDocument) bool {
	return NeedsOpenSpecBootstrap(doc)
}

// OpenSpecToolsFlag returns the openspec init --tools value for selected adapters.
func OpenSpecToolsFlag(doc ProjectDocument) string {
	return openspec.ToolsForAdapters(doc.Adapters.Selected)
}

// ProbeOpenSpecHealth is a read-only OpenSpec provider health probe for root+doc.
func ProbeOpenSpecHealth(ctx context.Context, root string, doc ProjectDocument) openspec.Health {
	pin := ""
	if OpenSpecRequired(doc) {
		pin = strings.TrimSpace(doc.Governance.OpenSpecVersion)
	}
	return openspec.Probe(ctx, openspec.ProbeInput{
		Root:       root,
		ProjectPin: pin,
	})
}

// PreflightOpenSpecForInit blocks Apply when OpenSpec is required but not usable
// for initialization or adoption. Returns the health snapshot for planning.
func PreflightOpenSpecForInit(ctx context.Context, root string, doc ProjectDocument) (openspec.Health, error) {
	if !OpenSpecRequired(doc) {
		return openspec.Health{}, nil
	}
	h := ProbeOpenSpecHealth(ctx, root, doc)
	switch h.State {
	case openspec.StateMissing:
		return h, fmt.Errorf("apply config: OpenSpec CLI missing: %s. %s", h.Message, h.InstallHint)
	case openspec.StateIncompatible:
		return h, fmt.Errorf("apply config: OpenSpec incompatible: %s. %s", h.Message, h.InstallHint)
	case openspec.StateError:
		if h.Project == openspec.ProjectMalformed {
			return h, fmt.Errorf("apply config: OpenSpec project malformed: %s", h.Message)
		}
		return h, fmt.Errorf("apply config: OpenSpec probe error: %s", h.Message)
	}
	switch h.Project {
	case openspec.ProjectMalformed:
		return h, fmt.Errorf("apply config: OpenSpec project malformed: %s", h.ProjectReason)
	case openspec.ProjectLegacyMkdir:
		// Allow Init to replace via real openspec init after approval (handled in Apply).
		if h.State == openspec.StateAvailable || h.State == openspec.StateReady {
			return h, nil
		}
		return h, fmt.Errorf("apply config: OpenSpec CLI not usable for legacy bootstrap remediation: %s", h.Message)
	}
	if h.State != openspec.StateAvailable && h.State != openspec.StateReady {
		return h, fmt.Errorf("apply config: OpenSpec not ready: %s", h.Message)
	}
	return h, nil
}

// AdoptOpenSpecPin sets the exact project pin from an installed compatible CLI.
// Call only after Probe confirms Atlas-compatible CLI.
func AdoptOpenSpecPin(doc *ProjectDocument, installed string) error {
	if doc == nil {
		return fmt.Errorf("openspec pin: nil document")
	}
	v, err := openspec.ParseVersion(installed)
	if err != nil {
		return err
	}
	if !openspec.AtlasCompatible(v) {
		return fmt.Errorf("openspec pin: %s outside Atlas support", v.String())
	}
	// Persist the exact pin including prerelease; never collapse to numeric core only.
	doc.Governance.OpenSpecVersion = v.String()
	return nil
}
