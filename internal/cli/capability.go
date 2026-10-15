package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/capability"
)

// CapabilityCommand is a resolved non-TUI capability planning action.
type CapabilityCommand struct {
	Name           string // plan
	Task           string
	Phase          string
	Classification string
	JSON           bool
}

func resolveCapability(args []string) Action {
	if len(args) == 0 {
		return Action{
			Mode:           ModeCapability,
			Capability:     CapabilityCommand{},
			UnknownCommand: "capability",
		}
	}
	if args[0] != "plan" {
		return Action{
			Mode:           ModeCapability,
			Capability:     CapabilityCommand{},
			UnknownCommand: joinCommand(append([]string{"capability"}, args...)),
		}
	}

	cmd := CapabilityCommand{Name: "plan"}
	rest := args[1:]
	positional := make([]string, 0, 1)
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case arg == "--json":
			cmd.JSON = true
		case arg == "--phase":
			if i+1 >= len(rest) {
				return Action{Mode: ModeCapability, Capability: cmd, UnknownCommand: "capability plan --phase"}
			}
			cmd.Phase = rest[i+1]
			i++
		case strings.HasPrefix(arg, "--phase="):
			cmd.Phase = strings.TrimPrefix(arg, "--phase=")
		case arg == "--classification":
			if i+1 >= len(rest) {
				return Action{Mode: ModeCapability, Capability: cmd, UnknownCommand: "capability plan --classification"}
			}
			cmd.Classification = rest[i+1]
			i++
		case strings.HasPrefix(arg, "--classification="):
			cmd.Classification = strings.TrimPrefix(arg, "--classification=")
		case strings.HasPrefix(arg, "-"):
			return Action{Mode: ModeCapability, Capability: cmd, UnknownCommand: joinCommand(append([]string{"capability", "plan"}, rest...))}
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 1 {
		cmd.Task = positional[0]
	} else if len(positional) > 1 {
		return Action{Mode: ModeCapability, Capability: cmd, UnknownCommand: joinCommand(append([]string{"capability", "plan"}, rest...))}
	}
	return Action{Mode: ModeCapability, Capability: cmd}
}

// CapabilityPlanService gathers evidence and builds a plan. Overridable in tests.
var CapabilityPlanService = func(cmd CapabilityCommand) capabilityPlanEnvelope {
	gathered, err := capability.GatherEvidence(capability.GatherInput{
		Task:           cmd.Task,
		Phase:          cmd.Phase,
		Classification: cmd.Classification,
	})
	if err != nil {
		return capabilityPlanEnvelope{
			Command: "plan",
			CapabilityPlan: capability.CapabilityPlan{
				SchemaVersion: capability.SchemaVersion,
				Status:        capability.StatusInvalidRequest,
				Error: &capability.ErrorBody{
					Code:    capability.StatusInvalidRequest,
					Message: err.Error(),
				},
			},
		}
	}
	plan := capability.DiscoverPlan(gathered.Evidence)
	return capabilityPlanEnvelope{
		Command:        "plan",
		ProjectID:      gathered.ProjectID,
		Warnings:       gathered.Warnings,
		CapabilityPlan: plan,
		ComposeSources: capability.ComposeSourcesFromPlan(plan),
	}
}

type capabilityPlanEnvelope struct {
	capability.CapabilityPlan
	Command        string   `json:"command"`
	ProjectID      string   `json:"project_id,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
	ComposeSources []string `json:"compose_sources,omitempty"`
}

func executeCapability(stdout io.Writer, cmd CapabilityCommand, unknown string) error {
	var env capabilityPlanEnvelope
	switch {
	case unknown != "":
		env = capabilityUsage("invalid capability command usage")
	case cmd.Name != "plan":
		env = capabilityUsage("usage: atlas capability plan \"<task>\" --json")
	case !cmd.JSON:
		env = capabilityUsage("--json is required")
	case strings.TrimSpace(cmd.Task) == "":
		env = capabilityUsage("usage: atlas capability plan \"<task>\" --json")
	default:
		env = CapabilityPlanService(cmd)
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(env); err != nil {
		return fmt.Errorf("capability: encode json: %w", err)
	}
	if env.Status == capability.StatusInvalidRequest {
		return fmt.Errorf("capability plan: %s", statusMessage(env))
	}
	if env.Status == capability.StatusBlocked {
		return fmt.Errorf("capability plan blocked")
	}
	return nil
}

func capabilityUsage(message string) capabilityPlanEnvelope {
	return capabilityPlanEnvelope{
		Command: "plan",
		CapabilityPlan: capability.CapabilityPlan{
			SchemaVersion: capability.SchemaVersion,
			Status:        capability.StatusInvalidRequest,
			Error: &capability.ErrorBody{
				Code:    capability.StatusInvalidRequest,
				Message: message,
			},
		},
	}
}

func statusMessage(env capabilityPlanEnvelope) string {
	if env.Error != nil && env.Error.Message != "" {
		return env.Error.Message
	}
	return env.Status
}
