// Physical disposable Home/project schema compatibility dogfood (DOGFOOD-021).
// Usage: go run ./scripts/dogfoodschema
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/doctor"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/inspect"
	"gopkg.in/yaml.v3"
)

func must(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
}

func fingerprint(root string) string {
	var b strings.Builder
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		fmt.Fprintf(&b, "%s:%x\n", filepath.ToSlash(rel), data)
		return nil
	})
	return b.String()
}

func main() {
	homeDir, err := os.MkdirTemp("", "atlas-d4-home-*")
	must(err)
	defer func() { _ = os.RemoveAll(homeDir) }()
	must(os.Setenv("ATLAS_HOME", homeDir))
	_, err = home.EnsureAndMirror(time.Now().UTC())
	must(err)
	fmt.Println("A healthy Home schema", home.CurrentSchemaVersion)

	root, err := os.MkdirTemp("", "atlas-d4-proj-*")
	must(err)
	defer func() { _ = os.RemoveAll(root) }()

	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "d4-schema", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
	})
	_ = draft.SetValue("adapters.selected", "cursor")
	fixed := time.Date(2026, 10, 15, 23, 0, 0, 0, time.UTC)
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(),
		Now: func() time.Time { return fixed },
	})
	must(err)
	doc, err := config.LoadProjectDocumentAt(root)
	must(err)
	fmt.Println("A healthy project schema", doc.SchemaVersion)

	// D: older atlas_version, current schema — still OK
	statePath := filepath.Join(homeDir, "state", "home.yaml")
	raw, err := os.ReadFile(statePath)
	must(err)
	var hdoc home.StateDocument
	must(yaml.Unmarshal(raw, &hdoc))
	hdoc.AtlasVersion = "0.0.9"
	hdoc.SchemaVersion = home.CurrentSchemaVersion
	out, err := yaml.Marshal(&hdoc)
	must(err)
	must(os.WriteFile(statePath, out, 0o600))
	_, err = home.RequireHealthy()
	must(err)
	fmt.Println("D older atlas_version + current schema: OK")

	// B: Home schema artificially newer
	hdoc.SchemaVersion = home.CurrentSchemaVersion + 1
	out, err = yaml.Marshal(&hdoc)
	must(err)
	must(os.WriteFile(statePath, out, 0o600))
	fpBefore := fingerprint(homeDir)
	_, err = home.EnsureAndMirror(time.Now().UTC())
	if err == nil {
		must(fmt.Errorf("expected remirror blocked"))
	}
	fmt.Println("B remirror blocked:", err.Error())
	if fingerprint(homeDir) != fpBefore {
		must(fmt.Errorf("Home fingerprint changed after blocked remirror"))
	}
	res, err := inspect.Inspect(root)
	must(err)
	rep := doctor.Evaluate(res)
	saw := false
	for _, c := range rep.Checks {
		if c.Name == "atlas home schema" && c.Severity == doctor.SeverityFail {
			saw = true
			fmt.Println("B doctor:", c.Message, "repairability=", c.Repairability)
		}
	}
	if !saw {
		must(fmt.Errorf("doctor missing home schema FAIL"))
	}
	// Restore Home schema for project tests.
	hdoc.SchemaVersion = home.CurrentSchemaVersion
	out, err = yaml.Marshal(&hdoc)
	must(err)
	must(os.WriteFile(statePath, out, 0o600))

	// C: Project schema artificially newer
	cfgPath := filepath.Join(root, ".atlas", "config.yaml")
	craw, err := os.ReadFile(cfgPath)
	must(err)
	var pdoc config.ProjectDocument
	must(yaml.Unmarshal(craw, &pdoc))
	pdoc.SchemaVersion = config.CurrentProjectSchemaVersion + 1
	pout, err := yaml.Marshal(&pdoc)
	must(err)
	must(os.WriteFile(cfgPath, pout, 0o644))
	fpProj := fingerprint(root)
	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "d4-schema", ProjectMode: "new", DefaultRemote: "origin",
		CursorDetected: true, ToolCursorAvailable: true,
	})
	_ = cfg.SetValue("adapters.selected", "cursor")
	_, err = config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: config.EmptyMCPDraft()})
	if err == nil {
		must(fmt.Errorf("expected configure blocked"))
	}
	fmt.Println("C configure blocked:", err.Error())
	if fingerprint(root) != fpProj {
		must(fmt.Errorf("project fingerprint changed after blocked configure"))
	}
	res2, err := inspect.Inspect(root)
	must(err)
	rep2 := doctor.Evaluate(res2)
	sawP := false
	for _, c := range rep2.Checks {
		if c.Name == "atlas project schema" && c.Severity == doctor.SeverityFail {
			sawP = true
			fmt.Println("C doctor:", c.Message, "repairability=", c.Repairability)
		}
	}
	if !sawP {
		must(fmt.Errorf("doctor missing project schema FAIL"))
	}

	fmt.Println("PHYSICAL_SCHEMA_OK")
}
