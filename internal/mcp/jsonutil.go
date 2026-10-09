package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

func stableJSON(v any) string {
	normalized, err := normalizeJSONValue(v)
	if err != nil {
		return fmt.Sprintf("err:%v", err)
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return fmt.Sprintf("err:%v", err)
	}
	return string(data)
}

func normalizeJSONValue(v any) (any, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(t))
		for _, k := range keys {
			nv, err := normalizeJSONValue(t[k])
			if err != nil {
				return nil, err
			}
			out[k] = nv
		}
		return out, nil
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			nv, err := normalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			out[i] = nv
		}
		return out, nil
	case []string:
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = item
		}
		return out, nil
	default:
		// Round-trip through JSON to normalize numbers/bools.
		data, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		var decoded any
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.UseNumber()
		if err := dec.Decode(&decoded); err != nil {
			return nil, err
		}
		return decoded, nil
	}
}

func mapsEqual(a, b map[string]any) bool {
	return stableJSON(a) == stableJSON(b)
}
