package hookproj

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// hooksDoc is a preservation-oriented view of Cursor hooks.json.
// Unknown top-level keys and unknown per-entry fields are retained.
type hooksDoc struct {
	Version int
	Hooks   map[string][]map[string]any
	// Extra holds unknown top-level properties (not version/hooks).
	Extra map[string]json.RawMessage
}

// parseHooksJSON reads existing hooks.json. Empty/missing raw → empty doc.
// Malformed non-empty content is an error (caller blocks, no mutation).
func parseHooksJSON(raw []byte) (hooksDoc, error) {
	doc := hooksDoc{
		Version: 1,
		Hooks:   map[string][]map[string]any{},
		Extra:   map[string]json.RawMessage{},
	}
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 {
		return doc, nil
	}

	// Top-level must be a JSON object (reject null/array/scalar before decode quirks).
	if trim[0] != '{' {
		return doc, fmt.Errorf("invalid hooks.json: top-level must be an object")
	}

	dec := json.NewDecoder(bytes.NewReader(trim))
	dec.UseNumber()
	var root map[string]json.RawMessage
	if err := dec.Decode(&root); err != nil {
		return doc, fmt.Errorf("invalid hooks.json: %w", err)
	}
	if root == nil {
		return doc, fmt.Errorf("invalid hooks.json: top-level must be an object")
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return doc, fmt.Errorf("invalid hooks.json: trailing data after JSON value")
	}

	verRaw, ok := root["version"]
	if !ok {
		return doc, fmt.Errorf("invalid hooks.json: missing version")
	}
	ver, err := parseHooksVersion(verRaw)
	if err != nil {
		return doc, err
	}
	doc.Version = ver

	hooksRaw, ok := root["hooks"]
	if !ok {
		return doc, fmt.Errorf("invalid hooks.json: missing hooks object")
	}
	if err := parseHooksObject(hooksRaw, &doc); err != nil {
		return doc, err
	}

	for k, v := range root {
		if k == "version" || k == "hooks" {
			continue
		}
		doc.Extra[k] = append(json.RawMessage(nil), v...)
	}
	return doc, nil
}

// parseHooksVersion requires exactly integer version 1 (rejects fractions/zero/unsupported).
func parseHooksVersion(raw json.RawMessage) (int, error) {
	trim := bytes.TrimSpace(raw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return 0, fmt.Errorf("invalid hooks.json version")
	}
	dec := json.NewDecoder(bytes.NewReader(trim))
	dec.UseNumber()
	var ver any
	if err := dec.Decode(&ver); err != nil {
		return 0, fmt.Errorf("invalid hooks.json version")
	}
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); err != io.EOF {
		return 0, fmt.Errorf("invalid hooks.json version")
	}
	n, ok := ver.(json.Number)
	if !ok {
		return 0, fmt.Errorf("invalid hooks.json version type")
	}
	s := string(n)
	if strings.ContainsAny(s, ".eE") {
		return 0, fmt.Errorf("invalid hooks.json version: fractional")
	}
	i, err := n.Int64()
	if err != nil {
		return 0, fmt.Errorf("invalid hooks.json version")
	}
	if i != 1 {
		return 0, fmt.Errorf("invalid hooks.json version: unsupported %d", i)
	}
	return 1, nil
}

func parseHooksObject(hooksRaw json.RawMessage, doc *hooksDoc) error {
	trim := bytes.TrimSpace(hooksRaw)
	if len(trim) == 0 || bytes.Equal(trim, []byte("null")) {
		return fmt.Errorf("hooks.json hooks must be an object")
	}
	if trim[0] != '{' {
		return fmt.Errorf("hooks.json hooks must be an object")
	}
	var hooksObj map[string]json.RawMessage
	if err := json.Unmarshal(hooksRaw, &hooksObj); err != nil || hooksObj == nil {
		return fmt.Errorf("hooks.json hooks must be an object")
	}
	for event, listRaw := range hooksObj {
		listTrim := bytes.TrimSpace(listRaw)
		if len(listTrim) == 0 || bytes.Equal(listTrim, []byte("null")) || listTrim[0] != '[' {
			return fmt.Errorf("hooks.json hooks.%s must be an array", event)
		}
		var arr []json.RawMessage
		if err := json.Unmarshal(listRaw, &arr); err != nil {
			return fmt.Errorf("hooks.json hooks.%s must be an array", event)
		}
		entries := make([]map[string]any, 0, len(arr))
		for i, item := range arr {
			itemTrim := bytes.TrimSpace(item)
			if len(itemTrim) == 0 || bytes.Equal(itemTrim, []byte("null")) || itemTrim[0] != '{' {
				return fmt.Errorf("hooks.json hooks.%s[%d] must be an object", event, i)
			}
			var obj map[string]any
			itemDec := json.NewDecoder(bytes.NewReader(item))
			itemDec.UseNumber()
			if err := itemDec.Decode(&obj); err != nil || obj == nil {
				return fmt.Errorf("hooks.json hooks.%s[%d] must be an object", event, i)
			}
			if cmd, ok := obj["command"].(string); ok {
				obj["command"] = strings.TrimSpace(cmd)
			}
			entries = append(entries, obj)
		}
		doc.Hooks[event] = entries
	}
	return nil
}

func entryCommand(e map[string]any) string {
	cmd, _ := e["command"].(string)
	return strings.TrimSpace(cmd)
}

// hasManagedCommand reports whether any entry uses the Atlas managed command path.
func hasManagedCommand(doc hooksDoc, command string) bool {
	command = strings.TrimSpace(command)
	for _, entries := range doc.Hooks {
		for _, e := range entries {
			if entryCommand(e) == command {
				return true
			}
		}
	}
	return false
}

// mergeManaged ensures exactly one Atlas-managed entry per managed event.
// User entries and unknown fields on non-Atlas entries are preserved.
// Duplicate Atlas entries are collapsed deterministically (first wins field-wise,
// then normalized to the canonical Atlas command-only entry).
func mergeManaged(doc hooksDoc, command string, events []string) hooksDoc {
	if doc.Hooks == nil {
		doc.Hooks = map[string][]map[string]any{}
	}
	if doc.Version < 1 {
		doc.Version = 1
	}
	if doc.Extra == nil {
		doc.Extra = map[string]json.RawMessage{}
	}
	command = strings.TrimSpace(command)
	canonical := map[string]any{"command": command}

	for _, event := range events {
		entries := doc.Hooks[event]
		var preserved []map[string]any
		seenAtlas := false
		for _, e := range entries {
			if entryCommand(e) == command {
				if seenAtlas {
					continue // drop duplicate Atlas entries
				}
				seenAtlas = true
				// Normalize Atlas entry to owned command identity only.
				preserved = append(preserved, map[string]any{"command": command})
				continue
			}
			preserved = append(preserved, e)
		}
		if !seenAtlas {
			preserved = append(preserved, canonical)
		}
		doc.Hooks[event] = preserved
	}
	return doc
}

// removeManaged strips Atlas-managed command entries; leaves user entries intact.
func removeManaged(doc hooksDoc, command string) hooksDoc {
	command = strings.TrimSpace(command)
	for event, entries := range doc.Hooks {
		out := make([]map[string]any, 0, len(entries))
		for _, e := range entries {
			if entryCommand(e) == command {
				continue
			}
			out = append(out, e)
		}
		if len(out) == 0 {
			delete(doc.Hooks, event)
		} else {
			doc.Hooks[event] = out
		}
	}
	return doc
}

// managedFingerprint digests complete Atlas-managed entries for ownership evidence.
// Fingerprint covers every attribute of each Atlas-owned entry (matcher, timeout,
// failClosed, unknown/future fields) via deterministic JSON canonicalization.
// User-owned entries and unknown top-level properties do not affect this digest.
// Ownership itself is never inferred from command/path equality — only ownership.yaml.
func managedFingerprint(doc hooksDoc, command string, events []string) string {
	command = strings.TrimSpace(command)
	type pair struct {
		event string
		raw   string
	}
	var pairs []pair
	for _, event := range events {
		for _, e := range doc.Hooks[event] {
			if entryCommand(e) != command {
				continue
			}
			raw, err := marshalMapStable(e)
			if err != nil {
				// Unmarshalable entry content is treated as distinct evidence.
				raw = []byte("\x00marshal-error:" + err.Error())
			}
			pairs = append(pairs, pair{event: event, raw: string(raw)})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].event != pairs[j].event {
			return pairs[i].event < pairs[j].event
		}
		return pairs[i].raw < pairs[j].raw
	})
	var b strings.Builder
	for _, p := range pairs {
		b.WriteString(p.event)
		b.WriteByte(0)
		b.WriteString(p.raw)
		b.WriteByte(0)
	}
	return DigestBytes([]byte(b.String()))
}

func marshalHooksJSON(doc hooksDoc) ([]byte, error) {
	if doc.Version < 1 {
		doc.Version = 1
	}
	if doc.Hooks == nil {
		doc.Hooks = map[string][]map[string]any{}
	}
	if doc.Extra == nil {
		doc.Extra = map[string]json.RawMessage{}
	}

	// Build a raw object map so unknown top-level keys survive.
	root := map[string]json.RawMessage{}
	ver, err := json.Marshal(doc.Version)
	if err != nil {
		return nil, err
	}
	root["version"] = ver

	eventNames := make([]string, 0, len(doc.Hooks))
	for event := range doc.Hooks {
		eventNames = append(eventNames, event)
	}
	sort.Strings(eventNames)

	hooksObj := map[string]json.RawMessage{}
	for _, event := range eventNames {
		entries := doc.Hooks[event]
		raw, err := marshalEntriesStable(entries)
		if err != nil {
			return nil, err
		}
		hooksObj[event] = raw
	}
	hooksRaw, err := marshalOrderedObject(hooksObj, eventNames)
	if err != nil {
		return nil, err
	}
	root["hooks"] = hooksRaw

	extraKeys := make([]string, 0, len(doc.Extra))
	for k := range doc.Extra {
		extraKeys = append(extraKeys, k)
	}
	sort.Strings(extraKeys)
	for _, k := range extraKeys {
		root[k] = doc.Extra[k]
	}

	allKeys := make([]string, 0, 2+len(extraKeys))
	allKeys = append(allKeys, "version", "hooks")
	allKeys = append(allKeys, extraKeys...)
	data, err := marshalOrderedObject(root, allKeys)
	if err != nil {
		return nil, err
	}
	// Pretty-print for human-editable project files while preserving content.
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		return append(data, '\n'), nil
	}
	pretty.WriteByte('\n')
	return pretty.Bytes(), nil
}

func marshalOrderedObject(obj map[string]json.RawMessage, keys []string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	first := true
	for _, k := range keys {
		v, ok := obj[k]
		if !ok {
			continue
		}
		if !first {
			buf.WriteByte(',')
		}
		first = false
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func marshalEntriesStable(entries []map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('[')
	for i, e := range entries {
		if i > 0 {
			buf.WriteByte(',')
		}
		raw, err := marshalMapStable(e)
		if err != nil {
			return nil, err
		}
		buf.Write(raw)
	}
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

func marshalMapStable(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		vb, err := canonicalJSONValue(m[k])
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		buf.Write(vb)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// canonicalJSONValue produces deterministic JSON for fingerprinting and stable writes.
// Object keys are sorted recursively; arrays preserve order; numbers use json.Number text.
func canonicalJSONValue(v any) ([]byte, error) {
	switch x := v.(type) {
	case nil:
		return []byte("null"), nil
	case map[string]any:
		return marshalMapStable(x)
	case []any:
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, el := range x {
			if i > 0 {
				buf.WriteByte(',')
			}
			raw, err := canonicalJSONValue(el)
			if err != nil {
				return nil, err
			}
			buf.Write(raw)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	case json.Number:
		s := string(x)
		if s == "" {
			return nil, fmt.Errorf("empty json.Number")
		}
		return []byte(s), nil
	case json.RawMessage:
		trim := bytes.TrimSpace(x)
		if len(trim) == 0 {
			return []byte("null"), nil
		}
		dec := json.NewDecoder(bytes.NewReader(trim))
		dec.UseNumber()
		var anyVal any
		if err := dec.Decode(&anyVal); err != nil {
			return nil, err
		}
		return canonicalJSONValue(anyVal)
	default:
		return json.Marshal(x)
	}
}

// isEmptyHooks reports whether no hook events remain (extras may still exist).
func isEmptyHooks(doc hooksDoc) bool {
	return len(doc.Hooks) == 0
}
