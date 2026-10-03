package initplan_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/initplan"
)

func TestWriteReport_IncludesRequiredSections(t *testing.T) {
	t.Parallel()

	plan := initplan.Plan{
		RootPath:    "/tmp/Atlas-CLI",
		ProjectName: "Atlas-CLI",
		ProjectMode: "existing",
		Steps: []initplan.Step{
			{Status: initplan.StatusCreate, Path: "AGENTS.md", Reason: "project runtime governance entrypoint"},
			{Status: initplan.StatusSkipExisting, Path: ".atlas/config.yaml", Reason: "existing Atlas config detected"},
			{Status: initplan.StatusFuture, Path: ".atlas/memory/atlas.sqlite", Reason: "SQLite memory store"},
		},
		Warnings: []string{
			"This is a dry-run plan.",
			"No files were created.",
		},
	}

	var buf bytes.Buffer
	initplan.WriteReport(&buf, plan)
	out := buf.String()

	for _, want := range []string{
		"Atlas Init Plan",
		"Project:",
		"Name: Atlas-CLI",
		"Mode: existing",
		"Root: /tmp/Atlas-CLI",
		"Planned Files:",
		"create AGENTS.md — project runtime governance entrypoint",
		"skip-existing .atlas/config.yaml — existing Atlas config detected",
		"future .atlas/memory/atlas.sqlite — SQLite memory store",
		"Notes:",
		"- This is a dry-run plan.",
		"- No files were created.",
		"Result:",
		"ready to initialize",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in report:\n%s", want, out)
		}
	}
}
