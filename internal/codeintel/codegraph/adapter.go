package codegraph

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/codeintel"
)

// Adapter is the CodeGraph provider behind Atlas Code Intelligence.
// It never installs, updates, uninstalls, or repairs CodeGraph.
type Adapter struct {
	LookPath LookPathFunc
	Runner   Runner
}

// New returns an Adapter with stdlib LookPath and ExecRunner defaults.
func New() *Adapter {
	return &Adapter{
		LookPath: exec.LookPath,
		Runner:   ExecRunner{},
	}
}

// ID implements codeintel.Provider.
func (a *Adapter) ID() codeintel.ProviderID {
	return codeintel.ProviderCodeGraph
}

// Probe detects CodeGraph availability and compatibility. Read-only.
// Uses only the verified invocation: codegraph --version
func (a *Adapter) Probe(ctx context.Context) (codeintel.Capability, error) {
	look := a.LookPath
	if look == nil {
		look = exec.LookPath
	}
	runner := a.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	cap := codeintel.Capability{
		Provider: codeintel.ProviderCodeGraph,
	}

	path, err := look(ExecutableName)
	if err != nil || strings.TrimSpace(path) == "" {
		cap.State = codeintel.StateUnavailable
		cap.Message = "CodeGraph executable not found on PATH"
		return cap, nil
	}
	cap.Executable = path

	result, runErr := runner.Run(ctx, path, VersionArgs())
	if runErr != nil {
		cap.State = codeintel.StateError
		cap.Message = normalizeRunError(runErr, result.Stderr)
		return cap, nil
	}

	version, err := decodeVersion(result.Stdout)
	if err != nil {
		cap.State = codeintel.StateError
		cap.Message = err.Error()
		return cap, nil
	}
	cap.Version = version

	if !IsCompatible(version) {
		cap.State = codeintel.StateIncompatible
		cap.Message = CompatibilityMessage(version)
		return cap, nil
	}

	cap.State = codeintel.StateAvailable
	// Only report what Probe actually verified at runtime.
	cap.Capabilities = []string{ProbedCapabilityVersion}
	cap.Message = CompatibilityMessage(version)
	return cap, nil
}

// Status reports provider availability plus read-only Atlas Home graph presence.
// It does not run CodeGraph stats/query/build and never creates DB/metadata.
func (a *Adapter) Status(ctx context.Context, project codeintel.Project) (codeintel.ProjectStatus, error) {
	cap, err := a.Probe(ctx)
	st := codeintel.ProjectStatus{
		Capability:   cap,
		ProjectID:    strings.TrimSpace(project.ID),
		StorageDir:   codeintel.ProviderStorageDir(project.HomePath, project.ID, codeintel.ProviderCodeGraph),
		GraphDBPath:  codeintel.GraphDBPath(project.HomePath, project.ID, codeintel.ProviderCodeGraph),
		MetadataPath: codeintel.MetadataPath(project.HomePath, project.ID, codeintel.ProviderCodeGraph),
	}
	if err != nil {
		return st, err
	}

	st.GraphPresent = fileExists(st.GraphDBPath)
	st.MetadataPresent = fileExists(st.MetadataPath)

	switch cap.State {
	case codeintel.StateUnavailable, codeintel.StateIncompatible, codeintel.StateError, codeintel.StateMissing:
		return st, nil
	}

	// Provider state stays availability-oriented. Graph presence is reported
	// separately — Slice 31 does not invent freshness/ready from unverified CLI.
	switch {
	case st.GraphPresent && st.MetadataPresent:
		st.Message = fmt.Sprintf("%s; Atlas-owned graph.db and metadata present", cap.Message)
	case st.GraphPresent:
		st.Message = fmt.Sprintf("%s; Atlas-owned graph.db present (metadata absent)", cap.Message)
	default:
		st.Message = fmt.Sprintf("%s; Atlas-owned graph.db not present yet", cap.Message)
	}
	return st, nil
}

func fileExists(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func normalizeRunError(err error, stderr []byte) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if errors.Is(err, ErrTimeout) {
		msg = err.Error()
	}
	if side := stderrMessage(stderr); side != "" {
		msg = msg + ": " + side
	}
	return msg
}

// Ensure Adapter satisfies the Atlas provider contract.
var _ codeintel.Provider = (*Adapter)(nil)
