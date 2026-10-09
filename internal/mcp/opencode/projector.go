package opencode

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
)

const (
	// ConfigRelPath is the project OpenCode config hosting MCP under `mcp`
	// (verified against https://opencode.ai/docs/mcp-servers/).
	ConfigRelPath = "opencode.json"
	adapterID     = mcp.AdapterOpenCode
)

// Projector implements mcp.Projector for OpenCode.
type Projector struct{}

// New returns an OpenCode MCP projector.
func New() Projector { return Projector{} }

func (Projector) ID() mcp.AdapterID     { return adapterID }
func (Projector) SupportsMCP() bool     { return true }
func (Projector) ConfigRelPath() string { return ConfigRelPath }

// InspectMCPProjection reads `opencode.json` without mutation.
func (Projector) InspectMCPProjection(root string) (mcp.NativeSnapshot, error) {
	snap := mcp.NativeSnapshot{
		Adapter:       adapterID,
		Path:          ConfigRelPath,
		Servers:       map[string]map[string]any{},
		OpenCodeShape: mcp.OpenCodeShapeEmpty,
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
		snap.MalformError = "opencode.json is a symlink"
		return snap, nil
	}
	if info.IsDir() {
		snap.Exists = true
		snap.Malformed = true
		snap.MalformError = "opencode.json is a directory"
		return snap, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return snap, err
	}
	snap.Exists = true
	snap.Mode = info.Mode().Perm()
	snap.Raw = append([]byte(nil), data...)
	servers, shape, malform, msg := parseOpenCodeMCP(data)
	snap.OpenCodeShape = shape
	if malform {
		snap.Malformed = true
		snap.MalformError = msg
		return snap, nil
	}
	snap.Servers = servers
	return snap, nil
}

// BuildMCPProjection maps a definition to OpenCode mcp server payload.
func (Projector) BuildMCPProjection(root string, def mcp.Definition) (mcp.NativeEntry, error) {
	key := mcp.NativeKeyFor(def)
	payload := map[string]any{"enabled": true}
	switch mcp.NormalizeTransport(string(def.Transport)) {
	case mcp.TransportStdio:
		payload["type"] = "local"
		cmd := make([]any, 0, 1+len(def.Args))
		cmd = append(cmd, def.Command)
		for _, a := range def.Args {
			if a == mcp.WorkspaceFolderToken {
				abs, err := filepath.Abs(root)
				if err != nil {
					return mcp.NativeEntry{}, err
				}
				cmd = append(cmd, abs)
				continue
			}
			cmd = append(cmd, a)
		}
		payload["command"] = cmd
		if len(def.EnvRefs) > 0 {
			env := map[string]any{}
			for _, ref := range def.EnvRefs {
				name := mcp.EnvRefName(ref)
				env[name] = "{env:" + name + "}"
			}
			payload["environment"] = env
		}
	case mcp.TransportStreamableHTTP, mcp.TransportSSE:
		payload["type"] = "remote"
		payload["url"] = def.Endpoint
		if len(def.HeaderRefs) > 0 {
			headers := map[string]any{}
			for header, ref := range def.HeaderRefs {
				headers[header] = ref.RenderOpenCode()
			}
			payload["headers"] = headers
		}
		switch def.AuthRequirement {
		case mcp.AuthOAuthExternal:
			// OpenCode remote OAuth object enables client-managed OAuth.
			payload["oauth"] = map[string]any{}
		case mcp.AuthNone, mcp.AuthEnvironmentReference:
			// Explicitly disable OpenCode OAuth when Atlas uses none or env headers.
			payload["oauth"] = false
		case mcp.AuthProviderManaged:
			// Provider-managed auth: omit oauth so the agent/provider owns the flow.
			// Atlas does not inject oauth:false or oauth:{}.
		}
	default:
		return mcp.NativeEntry{}, fmt.Errorf("opencode: unsupported transport %q", def.Transport)
	}
	return mcp.NativeEntry{Key: key, Payload: payload}, nil
}

// ApplyMCPProjection merges Atlas-managed MCP entries, preserving flat or nested shape.
func (p Projector) ApplyMCPProjection(root string, entries []mcp.NativeEntry, managedKeys []string, removeKeys []string) error {
	snap, err := p.InspectMCPProjection(root)
	if err != nil {
		return err
	}
	if snap.Malformed {
		return fmt.Errorf("opencode: blocked — native MCP config malformed: %s", snap.MalformError)
	}
	if snap.OpenCodeShape == mcp.OpenCodeShapeAmbiguous {
		return fmt.Errorf("opencode: blocked — ambiguous mcp representation (flat and nested present)")
	}

	doc := map[string]any{}
	if len(bytes.TrimSpace(snap.Raw)) > 0 {
		if err := json.Unmarshal(snap.Raw, &doc); err != nil {
			return fmt.Errorf("opencode: blocked — native MCP config malformed: %v", err)
		}
	}

	shape := snap.OpenCodeShape
	if shape == mcp.OpenCodeShapeEmpty {
		shape = mcp.OpenCodeShapeFlat // Atlas default for new files
	}

	servers := map[string]any{}
	for k, v := range snap.Servers {
		servers[k] = v
	}

	managed := setOf(managedKeys)
	for _, key := range removeKeys {
		if _, ok := managed[key]; !ok {
			return fmt.Errorf("opencode: refused to remove non-managed key %q", key)
		}
		delete(servers, key)
	}
	for _, entry := range entries {
		if _, ok := managed[entry.Key]; !ok {
			return fmt.Errorf("opencode: refused to write non-managed key %q", entry.Key)
		}
		servers[entry.Key] = entry.Payload
	}

	switch shape {
	case mcp.OpenCodeShapeNested:
		doc["mcp"] = map[string]any{"servers": servers}
	default:
		doc["mcp"] = servers
	}
	if _, ok := doc["$schema"]; !ok {
		doc["$schema"] = "https://opencode.ai/config.json"
	}

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

func parseOpenCodeMCP(data []byte) (map[string]map[string]any, mcp.OpenCodeShape, bool, string) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]map[string]any{}, mcp.OpenCodeShapeEmpty, true, "empty OpenCode config"
	}
	var raw any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, mcp.OpenCodeShapeEmpty, true, "invalid JSON: " + err.Error()
	}
	root, ok := raw.(map[string]any)
	if !ok {
		return nil, mcp.OpenCodeShapeEmpty, true, "OpenCode config root must be an object"
	}
	mcpRaw, ok := root["mcp"]
	if !ok {
		return map[string]map[string]any{}, mcp.OpenCodeShapeEmpty, false, ""
	}
	mcpObj, ok := mcpRaw.(map[string]any)
	if !ok {
		return nil, mcp.OpenCodeShapeEmpty, true, "mcp must be an object"
	}
	if len(mcpObj) == 0 {
		return map[string]map[string]any{}, mcp.OpenCodeShapeEmpty, false, ""
	}

	serversRaw, hasServers := mcpObj["servers"]
	otherKeys := 0
	for k := range mcpObj {
		if k == "servers" {
			continue
		}
		otherKeys++
	}

	if hasServers && otherKeys > 0 {
		// Ambiguous: nested servers key plus flat-style sibling keys.
		return nil, mcp.OpenCodeShapeAmbiguous, true, "ambiguous mcp representation: both mcp.servers and flat mcp entries present"
	}
	if hasServers {
		serversObj, ok := serversRaw.(map[string]any)
		if !ok {
			return nil, mcp.OpenCodeShapeNested, true, "mcp.servers must be an object"
		}
		out, malform, msg := asServerMap(serversObj)
		return out, mcp.OpenCodeShapeNested, malform, msg
	}
	out, malform, msg := asServerMap(mcpObj)
	return out, mcp.OpenCodeShapeFlat, malform, msg
}

func asServerMap(obj map[string]any) (map[string]map[string]any, bool, string) {
	out := map[string]map[string]any{}
	for name, val := range obj {
		if name == "servers" {
			continue
		}
		entry, ok := val.(map[string]any)
		if !ok {
			return nil, true, fmt.Sprintf("mcp.%s must be an object", name)
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
