package hookproj

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseMarshal_PreservesUnknownFields(t *testing.T) {
	raw := `{
  "version": 1,
  "experimentalFlag": true,
  "hooks": {
    "afterFileEdit": [
      {"command": "./user.sh", "matcher": "Write", "customField": 42, "nested": {"a": 1}}
    ],
    "preToolUse": [
      {"command": ".cursor/hooks/atlas/dispatch"},
      {"command": "./other.sh", "timeout": 5}
    ]
  }
}`
	doc, err := parseHooksJSON([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Extra["experimentalFlag"]; !ok {
		t.Fatal("top-level unknown field lost")
	}
	user := doc.Hooks["afterFileEdit"][0]
	if user["customField"] == nil || user["nested"] == nil {
		t.Fatalf("entry unknown fields lost: %#v", user)
	}

	merged := mergeManaged(doc, ".cursor/hooks/atlas/dispatch", []string{"preToolUse", "sessionStart"})
	out, err := marshalHooksJSON(merged)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatal(err)
	}
	if root["experimentalFlag"] != true {
		t.Fatalf("experimentalFlag=%v", root["experimentalFlag"])
	}
	hooks := root["hooks"].(map[string]any)
	after := hooks["afterFileEdit"].([]any)[0].(map[string]any)
	if after["customField"] == nil || after["nested"] == nil {
		t.Fatalf("preserved entry fields lost: %#v", after)
	}
	pre := hooks["preToolUse"].([]any)
	if len(pre) != 2 {
		t.Fatalf("preToolUse entries=%d", len(pre))
	}
	if _, ok := hooks["sessionStart"]; !ok {
		t.Fatal("sessionStart missing after merge")
	}
}

func TestMergeManaged_CollapsesDuplicateAtlasEntries(t *testing.T) {
	doc := hooksDoc{
		Version: 1,
		Hooks: map[string][]map[string]any{
			"preToolUse": {
				{"command": ".cursor/hooks/atlas/dispatch", "matcher": "Shell"},
				{"command": "./user.sh"},
				{"command": ".cursor/hooks/atlas/dispatch"},
			},
		},
	}
	merged := mergeManaged(doc, ".cursor/hooks/atlas/dispatch", []string{"preToolUse"})
	entries := merged.Hooks["preToolUse"]
	atlasCount := 0
	for _, e := range entries {
		if entryCommand(e) == ".cursor/hooks/atlas/dispatch" {
			atlasCount++
			if _, ok := e["matcher"]; ok {
				t.Fatal("atlas entry must be normalized to command-only")
			}
		}
	}
	if atlasCount != 1 {
		t.Fatalf("atlasCount=%d entries=%#v", atlasCount, entries)
	}
	if len(entries) != 2 {
		t.Fatalf("len=%d", len(entries))
	}
}

func TestParseHooksJSON_StrictValidation(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "null", raw: `null`, wantErr: "top-level"},
		{name: "array", raw: `[]`, wantErr: "top-level"},
		{name: "string", raw: `"hooks"`, wantErr: "top-level"},
		{name: "scalar", raw: `42`, wantErr: "top-level"},
		{name: "missing version", raw: `{"hooks":{}}`, wantErr: "missing version"},
		{name: "missing hooks", raw: `{"version":1}`, wantErr: "missing hooks"},
		{name: "version 0", raw: `{"version":0,"hooks":{}}`, wantErr: "version"},
		{name: "version 1.5", raw: `{"version":1.5,"hooks":{}}`, wantErr: "version"},
		{name: "version 2", raw: `{"version":2,"hooks":{}}`, wantErr: "version"},
		{name: "hooks null", raw: `{"version":1,"hooks":null}`, wantErr: "hooks"},
		{name: "hooks array", raw: `{"version":1,"hooks":[]}`, wantErr: "hooks"},
		{name: "hooks string", raw: `{"version":1,"hooks":"x"}`, wantErr: "hooks"},
		{name: "event entry null", raw: `{"version":1,"hooks":{"preToolUse":[null]}}`, wantErr: "object"},
		{name: "event entry string", raw: `{"version":1,"hooks":{"preToolUse":["x"]}}`, wantErr: "object"},
		{name: "concatenated", raw: `{"version":1,"hooks":{}}{"version":1}`, wantErr: "trailing"},
		{name: "malformed", raw: `{`, wantErr: "invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseHooksJSON([]byte(tc.raw))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err=%v want substring %q", err, tc.wantErr)
			}
		})
	}

	doc, err := parseHooksJSON([]byte(`{"version":1,"hooks":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Version != 1 {
		t.Fatalf("version=%d", doc.Version)
	}

	// Unknown properties preserved; empty input allowed for create path.
	withExtra, err := parseHooksJSON([]byte(`{"version":1,"hooks":{},"team":"platform","flag":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := withExtra.Extra["team"]; !ok {
		t.Fatal("unknown top-level property lost")
	}
	if _, ok := withExtra.Extra["flag"]; !ok {
		t.Fatal("unknown top-level flag lost")
	}
	empty, err := parseHooksJSON(nil)
	if err != nil || empty.Version != 1 {
		t.Fatalf("empty: err=%v version=%d", err, empty.Version)
	}
}

func TestManagedFingerprint_CompleteEntryContent(t *testing.T) {
	cmd := ".cursor/hooks/atlas/dispatch"
	events := []string{"preToolUse"}

	base := hooksDoc{Hooks: map[string][]map[string]any{
		"preToolUse": {{"command": cmd}},
	}}
	baseFP := managedFingerprint(base, cmd, events)

	t.Run("user entries ignored", func(t *testing.T) {
		other := hooksDoc{Hooks: map[string][]map[string]any{
			"preToolUse": {{"command": cmd}, {"command": "./user.sh", "timeout": 9}},
			"stop":       {{"command": "./stop.sh"}},
		}}
		if managedFingerprint(other, cmd, events) != baseFP {
			t.Fatal("user entries must not affect managed fingerprint")
		}
	})

	t.Run("top-level extras ignored", func(t *testing.T) {
		withExtra := hooksDoc{
			Hooks: map[string][]map[string]any{"preToolUse": {{"command": cmd}}},
			Extra: map[string]json.RawMessage{"meta": []byte(`{"a":1}`)},
		}
		if managedFingerprint(withExtra, cmd, events) != baseFP {
			t.Fatal("top-level extras must not affect managed fingerprint")
		}
	})

	t.Run("matcher", func(t *testing.T) {
		mod := hooksDoc{Hooks: map[string][]map[string]any{
			"preToolUse": {{"command": cmd, "matcher": "Shell"}},
		}}
		if managedFingerprint(mod, cmd, events) == baseFP {
			t.Fatal("matcher change must alter fingerprint")
		}
	})

	t.Run("timeout", func(t *testing.T) {
		mod := hooksDoc{Hooks: map[string][]map[string]any{
			"preToolUse": {{"command": cmd, "timeout": json.Number("30")}},
		}}
		if managedFingerprint(mod, cmd, events) == baseFP {
			t.Fatal("timeout change must alter fingerprint")
		}
	})

	t.Run("failClosed", func(t *testing.T) {
		mod := hooksDoc{Hooks: map[string][]map[string]any{
			"preToolUse": {{"command": cmd, "failClosed": true}},
		}}
		if managedFingerprint(mod, cmd, events) == baseFP {
			t.Fatal("failClosed change must alter fingerprint")
		}
	})

	t.Run("unknown attribute", func(t *testing.T) {
		mod := hooksDoc{Hooks: map[string][]map[string]any{
			"preToolUse": {{"command": cmd, "futureAttr": "x"}},
		}}
		if managedFingerprint(mod, cmd, events) == baseFP {
			t.Fatal("unknown attribute must alter fingerprint")
		}
	})

	t.Run("remove managed entry", func(t *testing.T) {
		mod := hooksDoc{Hooks: map[string][]map[string]any{
			"preToolUse": {{"command": "./user.sh"}},
		}}
		if managedFingerprint(mod, cmd, events) == baseFP {
			t.Fatal("removed managed entry must alter fingerprint")
		}
	})

	t.Run("duplicate managed entry", func(t *testing.T) {
		mod := hooksDoc{Hooks: map[string][]map[string]any{
			"preToolUse": {{"command": cmd}, {"command": cmd}},
		}}
		if managedFingerprint(mod, cmd, events) == baseFP {
			t.Fatal("duplicate managed entry must alter fingerprint")
		}
	})

	t.Run("canonical key order", func(t *testing.T) {
		a := hooksDoc{Hooks: map[string][]map[string]any{
			"preToolUse": {{"timeout": json.Number("1"), "command": cmd, "matcher": "A"}},
		}}
		b := hooksDoc{Hooks: map[string][]map[string]any{
			"preToolUse": {{"command": cmd, "matcher": "A", "timeout": json.Number("1")}},
		}}
		if managedFingerprint(a, cmd, events) != managedFingerprint(b, cmd, events) {
			t.Fatal("canonicalization must be key-order independent")
		}
	})
}
