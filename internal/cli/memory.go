package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/eshmun84/Atlas-CLI/internal/memoryretrieve"
)

// MemoryCommand is a resolved non-TUI memory CLI action.
type MemoryCommand struct {
	Name   string // search | get | context
	Query  string // search query / context task / get id
	Limit  int
	Budget int
	JSON   bool
}

func resolveMemory(args []string) Action {
	if len(args) == 0 {
		return Action{
			Mode:           ModeMemory,
			Memory:         MemoryCommand{},
			UnknownCommand: "memory",
		}
	}
	cmd := MemoryCommand{Name: args[0]}
	rest := args[1:]

	switch cmd.Name {
	case "search", "get", "context":
	default:
		return Action{
			Mode:           ModeMemory,
			Memory:         MemoryCommand{},
			UnknownCommand: joinCommand(append([]string{"memory"}, args...)),
		}
	}

	positional := make([]string, 0, 1)
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		switch {
		case arg == "--json":
			cmd.JSON = true
		case arg == "--limit":
			if i+1 >= len(rest) {
				return Action{Mode: ModeMemory, Memory: cmd, UnknownCommand: "memory " + cmd.Name + " --limit"}
			}
			n, err := strconv.Atoi(rest[i+1])
			if err != nil || n < 0 {
				return Action{Mode: ModeMemory, Memory: cmd, UnknownCommand: "memory " + cmd.Name + " --limit"}
			}
			cmd.Limit = n
			i++
		case strings.HasPrefix(arg, "--limit="):
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--limit="))
			if err != nil || n < 0 {
				return Action{Mode: ModeMemory, Memory: cmd, UnknownCommand: "memory " + cmd.Name + " --limit"}
			}
			cmd.Limit = n
		case arg == "--budget":
			if cmd.Name != "context" {
				return Action{Mode: ModeMemory, Memory: cmd, UnknownCommand: joinCommand(append([]string{"memory"}, args...))}
			}
			if i+1 >= len(rest) {
				return Action{Mode: ModeMemory, Memory: cmd, UnknownCommand: "memory context --budget"}
			}
			n, err := strconv.Atoi(rest[i+1])
			if err != nil || n < 0 {
				return Action{Mode: ModeMemory, Memory: cmd, UnknownCommand: "memory context --budget"}
			}
			cmd.Budget = n
			i++
		case strings.HasPrefix(arg, "--budget="):
			if cmd.Name != "context" {
				return Action{Mode: ModeMemory, Memory: cmd, UnknownCommand: joinCommand(append([]string{"memory"}, args...))}
			}
			n, err := strconv.Atoi(strings.TrimPrefix(arg, "--budget="))
			if err != nil || n < 0 {
				return Action{Mode: ModeMemory, Memory: cmd, UnknownCommand: "memory context --budget"}
			}
			cmd.Budget = n
		case strings.HasPrefix(arg, "-"):
			return Action{Mode: ModeMemory, Memory: cmd, UnknownCommand: joinCommand(append([]string{"memory"}, args...))}
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) == 1 {
		cmd.Query = positional[0]
	} else if len(positional) > 1 {
		return Action{Mode: ModeMemory, Memory: cmd, UnknownCommand: joinCommand(append([]string{"memory"}, args...))}
	}
	return Action{Mode: ModeMemory, Memory: cmd}
}

// MemoryService builds the retrieval service. Overridable in tests.
var MemoryService = func() memoryretrieve.Service {
	return memoryretrieve.Service{}
}

func executeMemory(stdout io.Writer, cmd MemoryCommand, unknown string) error {
	var resp memoryretrieve.Response
	switch {
	case cmd.Name == "":
		resp = usageResponse("memory", "usage: atlas memory <search|get|context> ... --json")
	case unknown != "":
		name := cmd.Name
		if name == "" {
			name = "memory"
		}
		resp = usageResponse(name, "invalid memory command usage")
	case !cmd.JSON:
		resp = usageResponse(cmd.Name, "--json is required")
	default:
		svc := MemoryService()
		switch cmd.Name {
		case "search":
			resp = svc.Search(memoryretrieve.SearchInput{Query: cmd.Query, Limit: cmd.Limit})
		case "get":
			resp = svc.Get(memoryretrieve.GetInput{ID: cmd.Query})
		case "context":
			resp = svc.Context(memoryretrieve.ContextInput{Task: cmd.Query, Limit: cmd.Limit, Budget: cmd.Budget})
		default:
			resp = usageResponse("memory", "usage: atlas memory <search|get|context> ... --json")
		}
	}

	enc := json.NewEncoder(stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(resp); err != nil {
		return fmt.Errorf("memory: encode json: %w", err)
	}
	if memoryretrieve.ShouldExitNonZero(resp.Status) {
		return memoryretrieve.ExitError{Status: resp.Status}
	}
	return nil
}

func usageResponse(command, message string) memoryretrieve.Response {
	return memoryretrieve.Response{
		SchemaVersion: memoryretrieve.SchemaVersion,
		Command:       command,
		Status:        memoryretrieve.StatusInvalidUsage,
		Count:         0,
		Error: &memoryretrieve.ErrorBody{
			Code:    memoryretrieve.StatusInvalidUsage,
			Message: message,
		},
	}
}
