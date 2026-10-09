package codegraph

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

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

// Refresh runs codegraph build (+ stats verification) against Atlas-owned DB.
// Does not write Atlas metadata.json (owned by codeintel.Service.Refresh).
func (a *Adapter) Refresh(ctx context.Context, req codeintel.RefreshRequest) (codeintel.RefreshResult, error) {
	look := a.LookPath
	if look == nil {
		look = exec.LookPath
	}
	runner := a.Runner
	if runner == nil {
		runner = ExecRunner{Timeout: 2 * time.Minute}
	}

	path, err := look(ExecutableName)
	if err != nil || strings.TrimSpace(path) == "" {
		return codeintel.RefreshResult{}, fmt.Errorf("codegraph: executable not found on PATH")
	}

	// Refuse unsafe graph.db leaf before invoking the external provider.
	if req.HomePath != "" {
		if _, err := codeintel.InspectStorageLeaf(req.HomePath, req.DBPath); err != nil {
			return codeintel.RefreshResult{}, err
		}
	}

	mode := req.Mode
	var args []string
	switch mode {
	case codeintel.RefreshModeFull:
		args = FullBuildArgs(req.Root, req.DBPath)
	case codeintel.RefreshModeIncremental, codeintel.RefreshModeInitial, "":
		mode = codeintel.RefreshModeIncremental
		args = BuildArgs(req.Root, req.DBPath)
	default:
		return codeintel.RefreshResult{}, fmt.Errorf("codegraph: unsupported refresh mode %q", mode)
	}

	buildResult, runErr := runner.Run(ctx, path, args)
	if runErr != nil {
		return codeintel.RefreshResult{}, fmt.Errorf("codegraph: build: %s", normalizeRunError(runErr, buildResult.Stderr))
	}
	if req.HomePath != "" {
		leaf, leafErr := codeintel.InspectStorageLeaf(req.HomePath, req.DBPath)
		if leafErr != nil {
			return codeintel.RefreshResult{}, leafErr
		}
		if !leaf.Present {
			return codeintel.RefreshResult{}, fmt.Errorf("codegraph: build finished but graph.db missing at %s", req.DBPath)
		}
	} else if !fileExistsRegular(req.DBPath) {
		return codeintel.RefreshResult{}, fmt.Errorf("codegraph: build finished but graph.db missing at %s", req.DBPath)
	}

	statsResult, statsErr := runner.Run(ctx, path, StatsArgs(req.DBPath))
	if statsErr != nil {
		return codeintel.RefreshResult{}, fmt.Errorf("codegraph: stats: %s", normalizeRunError(statsErr, statsResult.Stderr))
	}
	nodes, files, err := decodeStatsTotals(statsResult.Stdout)
	if err != nil {
		return codeintel.RefreshResult{}, err
	}
	if nodes <= 0 || files <= 0 {
		return codeintel.RefreshResult{}, fmt.Errorf("codegraph: stats verification failed: nodes=%d files=%d", nodes, files)
	}
	return codeintel.RefreshResult{
		Mode:       mode,
		Message:    fmt.Sprintf("codegraph %s: nodes=%d files=%d", mode, nodes, files),
		NodesTotal: nodes,
		FilesTotal: files,
	}, nil
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

	if project.HomePath != "" {
		graphLeaf, graphErr := codeintel.InspectStorageLeaf(project.HomePath, st.GraphDBPath)
		if graphErr != nil {
			st.State = codeintel.StateError
			st.Message = graphErr.Error()
			st.GraphPresent = false
			st.MetadataPresent = false
			return st, nil
		}
		st.GraphPresent = graphLeaf.Present
		metaLeaf, metaErr := codeintel.InspectStorageLeaf(project.HomePath, st.MetadataPath)
		if metaErr != nil {
			st.State = codeintel.StateError
			st.Message = metaErr.Error()
			st.MetadataPresent = false
			return st, nil
		}
		st.MetadataPresent = metaLeaf.Present
	}

	switch cap.State {
	case codeintel.StateUnavailable, codeintel.StateIncompatible, codeintel.StateError, codeintel.StateMissing:
		return st, nil
	}
	if st.State == codeintel.StateError {
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

func fileExistsRegular(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular()
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
