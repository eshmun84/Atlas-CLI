package cursor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
)

const (
	// ConfigRelPath is the project-level Cursor MCP config (official Cursor docs).
	ConfigRelPath = ".cursor/mcp.json"
	adapterID     = mcp.AdapterCursor
)

// Projector implements mcp.Projector for Cursor.
type Projector struct{}

// New returns a Cursor MCP projector.
func New() Projector { return Projector{} }

func (Projector) ID() mcp.AdapterID     { return adapterID }
func (Projector) SupportsMCP() bool     { return true }
func (Projector) ConfigRelPath() string { return ConfigRelPath }

// InspectMCPProjection reads `.cursor/mcp.json` without mutation.
func (Projector) InspectMCPProjection(root string) (mcp.NativeSnapshot, error) {
	snap := mcp.NativeSnapshot{
		Adapter: adapterID,
		Path:    ConfigRelPath,
		Servers: map[string]map[string]any{},
	}
	full, err := mcp.ContainedJoin(root, ConfigRelPath)
	if err != nil {
		snap.Malformed = true
		snap.MalformError = err.Error()
		return snap, nil
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return snap, nil
	}
	if err != nil {
		return snap, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		snap.Exists = true
		snap.Malformed = true
		snap.MalformError = "mcp.json is a symlink"
		return snap, nil
	}
	if info.IsDir() {
		snap.Exists = true
		snap.Malformed = true
		snap.MalformError = "mcp.json is a directory"
		return snap, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return snap, err
	}
	snap.Exists = true
	snap.Mode = info.Mode().Perm()
	snap.Raw = append([]byte(nil), data...)
	servers, malform, msg := parseCursorMCP(data)
	if malform {
		snap.Malformed = true
		snap.MalformError = msg
		return snap, nil
	}
	snap.Servers = servers
	return snap, nil
}

// BuildMCPProjection maps a definition to Cursor mcpServers entry shape.
func (Projector) BuildMCPProjection(_ string, def mcp.Definition) (mcp.NativeEntry, error) {
	key := mcp.NativeKeyFor(def)
	payload := map[string]any{}
	switch mcp.NormalizeTransport(string(def.Transport)) {
	case mcp.TransportStdio:
		payload["type"] = "stdio"
		payload["command"] = def.Command
		if len(def.Args) > 0 {
			args := make([]any, len(def.Args))
			for i, a := range def.Args {
				args[i] = a
			}
			payload["args"] = args
		}
		if len(def.EnvRefs) > 0 {
			env := map[string]any{}
			for _, ref := range def.EnvRefs {
				name := mcp.EnvRefName(ref)
				env[name] = "${env:" + name + "}"
			}
			payload["env"] = env
		}
	case mcp.TransportStreamableHTTP, mcp.TransportSSE:
		payload["url"] = def.Endpoint
		if len(def.HeaderRefs) > 0 {
			headers := map[string]any{}
			for header, ref := range def.HeaderRefs {
				headers[header] = ref.RenderCursor()
			}
			payload["headers"] = headers
		}
	default:
		return mcp.NativeEntry{}, fmt.Errorf("cursor: unsupported transport %q", def.Transport)
	}
	return mcp.NativeEntry{Key: key, Payload: payload}, nil
}

// ApplyMCPProjection merges Atlas-managed entries into `.cursor/mcp.json`.
// Backups are handled by reconcile under Atlas Home — never written into the repo.
func (p Projector) ApplyMCPProjection(root string, entries []mcp.NativeEntry, managedKeys []string, removeKeys []string) error {
	snap, err := p.InspectMCPProjection(root)
	if err != nil {
		return err
	}
	if snap.Malformed {
		return fmt.Errorf("cursor: blocked — native MCP config malformed: %s", snap.MalformError)
	}

	doc := map[string]any{}
	if len(bytes.TrimSpace(snap.Raw)) > 0 {
		if err := json.Unmarshal(snap.Raw, &doc); err != nil {
			return fmt.Errorf("cursor: blocked — native MCP config malformed: %v", err)
		}
	}
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
	}

	managed := setOf(managedKeys)
	for _, key := range removeKeys {
		if _, ok := managed[key]; !ok {
			return fmt.Errorf("cursor: refused to remove non-managed key %q", key)
		}
		delete(servers, key)
	}
	for _, entry := range entries {
		if _, ok := managed[entry.Key]; !ok {
			return fmt.Errorf("cursor: refused to write non-managed key %q", entry.Key)
		}
		servers[entry.Key] = entry.Payload
	}
	doc["mcpServers"] = servers

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	perm := os.FileMode(0o644)
	if snap.Exists && snap.Mode != 0 {
		perm = snap.Mode
	}
	return mcp.AtomicWriteContained(root, ConfigRelPath, data, perm)
}

// RemoveManagedMCPProjection removes only Atlas-managed keys.
func (p Projector) RemoveManagedMCPProjection(root string, managedKeys []string) error {
	if len(managedKeys) == 0 {
		return nil
	}
	return p.ApplyMCPProjection(root, nil, managedKeys, managedKeys)
}

func parseCursorMCP(data []byte) (map[string]map[string]any, bool, string) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]map[string]any{}, true, "empty MCP config"
	}
	var raw any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, true, "invalid JSON: " + err.Error()
	}
	root, ok := raw.(map[string]any)
	if !ok {
		return nil, true, "MCP config root must be an object"
	}
	serversRaw, ok := root["mcpServers"]
	if !ok {
		return nil, true, "missing mcpServers object"
	}
	serversObj, ok := serversRaw.(map[string]any)
	if !ok {
		return nil, true, "mcpServers must be an object"
	}
	out := map[string]map[string]any{}
	for name, val := range serversObj {
		entry, ok := val.(map[string]any)
		if !ok {
			return nil, true, fmt.Sprintf("mcpServers.%s must be an object", name)
		}
		out[name] = entry
	}
	return out, false, ""
}

func setOf(keys []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, k := range keys {
		out[k] = struct{}{}
	}
	return out
}
