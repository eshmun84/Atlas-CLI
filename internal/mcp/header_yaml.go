package mcp

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// UnmarshalYAML accepts either a bare env name string or {env, prefix} object.
// Legacy configs store header_refs as map[string]string; new configs use objects.
func (h *HeaderValueRef) UnmarshalYAML(value *yaml.Node) error {
	if h == nil {
		return fmt.Errorf("nil HeaderValueRef")
	}
	switch value.Kind {
	case yaml.ScalarNode:
		h.Env = strings.TrimSpace(value.Value)
		h.Prefix = ""
		return nil
	case yaml.MappingNode:
		var raw struct {
			Env    string `yaml:"env"`
			Prefix string `yaml:"prefix"`
		}
		if err := value.Decode(&raw); err != nil {
			return err
		}
		h.Env = strings.TrimSpace(raw.Env)
		h.Prefix = raw.Prefix
		return nil
	default:
		return fmt.Errorf("header ref must be a string or {env, prefix} object")
	}
}

// MarshalYAML always emits the structured object form.
func (h HeaderValueRef) MarshalYAML() (any, error) {
	out := map[string]string{"env": h.Env}
	if h.Prefix != "" {
		out["prefix"] = h.Prefix
	}
	return out, nil
}
