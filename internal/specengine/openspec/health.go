package openspec

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ProviderState is the bounded OpenSpec provider health state.
type ProviderState string

const (
	StateMissing      ProviderState = "missing"
	StateIncompatible ProviderState = "incompatible"
	StateError        ProviderState = "error"
	StateAvailable    ProviderState = "available"
	StateReady        ProviderState = "ready"
)

// IncompatibilityKind distinguishes incompatible subtypes for diagnostics.
type IncompatibilityKind string

const (
	IncompatNone            IncompatibilityKind = ""
	IncompatUnsupported     IncompatibilityKind = "unsupported_version"
	IncompatProjectMismatch IncompatibilityKind = "project_version_mismatch"
	IncompatCapability      IncompatibilityKind = "missing_capability"
)

// ProjectInitState classifies OpenSpec project filesystem readiness.
type ProjectInitState string

const (
	ProjectAbsent      ProjectInitState = "absent"
	ProjectReady       ProjectInitState = "ready"
	ProjectLegacyMkdir ProjectInitState = "legacy_mkdir"
	ProjectMalformed   ProjectInitState = "malformed"
)

// Health is the read-only OpenSpec provider health snapshot.
type Health struct {
	State           ProviderState
	IncompatKind    IncompatibilityKind
	Message         string
	InstallHint     string
	Executable      string
	Installed       string // exact installed version string when known
	RequiredPin     string // project exact pin when known
	AtlasCompatible bool
	PinAligned      bool
	CapabilitiesOK  bool
	Project         ProjectInitState
	ProjectReason   string
	GovernedSDDOK   bool // true only when StateReady
}

// ProbeInput configures a read-only provider health probe.
type ProbeInput struct {
	Root          string // project root (may be empty for CLI-only probes)
	ProjectPin    string // exact pin from Atlas config; empty = unset
	LookPath      LookPathFunc
	Runner        Runner
	SkipCapProbes bool // tests only: skip list/status JSON capability checks
}

// Probe evaluates OpenSpec CLI + project readiness without mutating state.
func Probe(ctx context.Context, in ProbeInput) Health {
	look := in.LookPath
	if look == nil {
		look = exec.LookPath
	}
	runner := in.Runner
	if runner == nil {
		runner = ExecRunner{Timeout: probeTimeout}
	}

	exe, err := look(ExecutableName)
	if err != nil || strings.TrimSpace(exe) == "" {
		h := Health{
			State:       StateMissing,
			Message:     "openspec executable not found on PATH",
			InstallHint: missingInstallHint(in.ProjectPin),
			Project:     ProjectAbsent,
		}
		if in.Root != "" {
			h.Project, h.ProjectReason = ClassifyProject(in.Root)
		}
		return h
	}
	exe = filepath.Clean(exe)

	verOut, runErr := runner.Run(ctx, exe, []string{"version", "--json"}, "")
	if runErr != nil {
		return Health{
			State:      StateError,
			Executable: exe,
			Message:    "openspec version --json failed: " + truncateMsg(runErr.Error()),
			Project:    projectOrAbsent(in.Root),
		}
	}
	installedRaw, parseErr := parseVersionJSON(verOut.Stdout)
	if parseErr != nil {
		return Health{
			State:      StateError,
			Executable: exe,
			Message:    "invalid openspec version JSON: " + parseErr.Error(),
			Project:    projectOrAbsent(in.Root),
		}
	}
	installed, err := ParseVersion(installedRaw)
	if err != nil {
		return Health{
			State:      StateError,
			Executable: exe,
			Installed:  installedRaw,
			Message:    "unparseable openspec version: " + err.Error(),
			Project:    projectOrAbsent(in.Root),
		}
	}

	h := Health{
		Executable:  exe,
		Installed:   installed.String(),
		RequiredPin: strings.TrimSpace(in.ProjectPin),
	}

	if !AtlasCompatible(installed) {
		h.State = StateIncompatible
		h.IncompatKind = IncompatUnsupported
		h.Message = fmt.Sprintf("OpenSpec %s is outside Atlas support (>= %s, major < %d)",
			installed.String(), MinSupportedVersion, MaxSupportedMajor)
		h.InstallHint = pinInstallHint(in.ProjectPin)
		h.Project, h.ProjectReason = classifyOrAbsent(in.Root)
		return h
	}
	h.AtlasCompatible = true

	if pin := strings.TrimSpace(in.ProjectPin); pin != "" {
		pinVer, pinErr := ParseVersion(pin)
		if pinErr != nil {
			h.State = StateError
			h.Message = "invalid project openspec_version pin: " + pinErr.Error()
			h.Project, h.ProjectReason = classifyOrAbsent(in.Root)
			return h
		}
		if !ExactMatch(installed, pinVer) {
			h.State = StateIncompatible
			h.IncompatKind = IncompatProjectMismatch
			h.Message = fmt.Sprintf("Project requires OpenSpec %s; installed OpenSpec: %s", pinVer.String(), installed.String())
			h.InstallHint = pinInstallHint(pinVer.String())
			h.Project, h.ProjectReason = classifyOrAbsent(in.Root)
			return h
		}
		h.PinAligned = true
	} else {
		// No pin yet: CLI may still be used for Init adoption.
		h.PinAligned = false
	}

	if !in.SkipCapProbes {
		if capErr := probeCapabilities(ctx, runner, exe, in.Root); capErr != "" {
			h.State = StateIncompatible
			h.IncompatKind = IncompatCapability
			h.Message = capErr
			h.Project, h.ProjectReason = classifyOrAbsent(in.Root)
			return h
		}
	}
	h.CapabilitiesOK = true

	h.Project, h.ProjectReason = classifyOrAbsent(in.Root)
	switch h.Project {
	case ProjectReady:
		if h.RequiredPin != "" && h.PinAligned {
			h.State = StateReady
			h.Message = "OpenSpec CLI aligned and project ready"
			h.GovernedSDDOK = true
			return h
		}
		// Valid project but pin not adopted yet.
		h.State = StateAvailable
		h.Message = "OpenSpec project present; project version pin not set or not aligned"
		return h
	case ProjectAbsent:
		h.State = StateAvailable
		h.Message = "OpenSpec CLI available; project not initialized"
		return h
	case ProjectLegacyMkdir:
		h.State = StateAvailable
		h.Message = "legacy Atlas mkdir-only openspec/ tree; real OpenSpec init required"
		return h
	default:
		h.State = StateError
		if h.Message == "" {
			h.Message = "malformed or ambiguous OpenSpec project state"
		}
		if h.ProjectReason != "" {
			h.Message = h.ProjectReason
		}
		return h
	}
}

func probeCapabilities(ctx context.Context, runner Runner, exe, root string) string {
	// version --json already succeeded. Probe list --json (read-only).
	dir := ""
	if strings.TrimSpace(root) != "" {
		dir = root
	}
	out, err := runner.Run(ctx, exe, []string{"list", "--json"}, dir)
	if err != nil {
		// list exits non-zero when no root; still proves JSON capability if stdout parses.
		if len(out.Stdout) == 0 {
			return "missing capability: openspec list --json (" + truncateMsg(err.Error()) + ")"
		}
	}
	var listObj map[string]any
	if jsonErr := json.Unmarshal(out.Stdout, &listObj); jsonErr != nil {
		return "missing capability: openspec list --json returns invalid JSON"
	}
	if _, ok := listObj["root"]; !ok {
		return "missing capability: openspec list --json missing root field"
	}

	// Help-based probes for mutating commands (do not execute them).
	for _, args := range [][]string{
		{"init", "--help"},
		{"update", "--help"},
		{"status", "--help"},
		{"instructions", "--help"},
		{"validate", "--help"},
		{"archive", "--help"},
	} {
		helpOut, helpErr := runner.Run(ctx, exe, args, "")
		if helpErr != nil && len(helpOut.Stdout) == 0 && len(helpOut.Stderr) == 0 {
			return "missing capability: openspec " + args[0]
		}
		text := string(helpOut.Stdout) + string(helpOut.Stderr)
		if args[0] == "status" || args[0] == "instructions" || args[0] == "validate" || args[0] == "archive" {
			if !strings.Contains(text, "--json") {
				return "missing capability: openspec " + args[0] + " --json"
			}
		}
		if args[0] == "init" && !strings.Contains(text, "--tools") {
			return "missing capability: openspec init --tools"
		}
	}
	return ""
}

func parseVersionJSON(raw []byte) (string, error) {
	var obj struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}
	if strings.TrimSpace(obj.Version) == "" {
		return "", fmt.Errorf("%w: empty version field", ErrInvalidJSON)
	}
	return strings.TrimSpace(obj.Version), nil
}

func projectOrAbsent(root string) ProjectInitState {
	if strings.TrimSpace(root) == "" {
		return ProjectAbsent
	}
	st, _ := ClassifyProject(root)
	return st
}

func classifyOrAbsent(root string) (ProjectInitState, string) {
	if strings.TrimSpace(root) == "" {
		return ProjectAbsent, ""
	}
	return ClassifyProject(root)
}

func missingInstallHint(pin string) string {
	if p := strings.TrimSpace(pin); p != "" {
		return fmt.Sprintf("Install OpenSpec %s: npm install -g @fission-ai/openspec@%s (requires Node.js >= 20.19.0)", p, p)
	}
	return "Install OpenSpec first (Node.js >= 20.19.0): npm install -g @fission-ai/openspec@" + MinSupportedVersion +
		" (or a compatible 1.14.x). Then re-run Atlas Init."
}

func pinInstallHint(pin string) string {
	p := strings.TrimSpace(pin)
	if p == "" {
		p = MinSupportedVersion
	}
	return fmt.Sprintf("Install the project OpenSpec version: npm install -g @fission-ai/openspec@%s", p)
}

func truncateMsg(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 240 {
		return s[:240] + "…"
	}
	return s
}
