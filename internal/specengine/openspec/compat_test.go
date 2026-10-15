package openspec_test

import (
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/specengine/openspec"
)

func TestParseVersionAndAtlasCompatible(t *testing.T) {
	cases := []struct {
		raw    string
		ok     bool
		compat bool
	}{
		{"1.13.9", true, false},
		{"1.14.0", true, true},
		{"1.14.2", true, true},
		{"v1.15.0", true, true},
		{"2.0.0", true, false},
		{"1.14.0-rc.1", true, false}, // prerelease at floor rejected
		{"1.14.2-beta.1", true, true},
		{"", false, false},
		{"nope", false, false},
	}
	for _, tc := range cases {
		v, err := openspec.ParseVersion(tc.raw)
		if tc.ok && err != nil {
			t.Fatalf("%s: unexpected err %v", tc.raw, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s: expected error", tc.raw)
		}
		if tc.ok && openspec.AtlasCompatible(v) != tc.compat {
			t.Fatalf("%s: compat=%v want %v", tc.raw, openspec.AtlasCompatible(v), tc.compat)
		}
	}
}

func TestExactMatch_ProjectPin(t *testing.T) {
	cases := []struct {
		name      string
		installed string
		pin       string
		aligned   bool
	}{
		{"same stable", "1.14.2", "1.14.2", true},
		{"v prefix pin", "1.14.2", "v1.14.2", true},
		{"older", "1.14.1", "1.14.2", false},
		{"newer", "1.15.0", "1.14.2", false},
		{"prerelease vs stable", "1.14.2-beta.1", "1.14.2", false},
		{"stable vs prerelease pin", "1.14.2", "1.14.2-beta.1", false},
		{"same prerelease", "1.14.2-beta.1", "1.14.2-beta.1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inst, err := openspec.ParseVersion(tc.installed)
			if err != nil {
				t.Fatal(err)
			}
			pin, err := openspec.ParseVersion(tc.pin)
			if err != nil {
				t.Fatal(err)
			}
			if got := openspec.ExactMatch(inst, pin); got != tc.aligned {
				t.Fatalf("ExactMatch(%q,%q)=%v want %v", tc.installed, tc.pin, got, tc.aligned)
			}
		})
	}
}

func TestParsedVersion_StringPreservesPrerelease(t *testing.T) {
	v, err := openspec.ParseVersion("1.14.2-beta.1")
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "1.14.2-beta.1" {
		t.Fatalf("String()=%q", v.String())
	}
	stable, err := openspec.ParseVersion("1.14.2")
	if err != nil {
		t.Fatal(err)
	}
	if stable.String() != "1.14.2" {
		t.Fatalf("String()=%q", stable.String())
	}
}

func TestToolsForAdapters(t *testing.T) {
	if g := openspec.ToolsForAdapters([]string{"cursor"}); g != "cursor" {
		t.Fatalf("got %q", g)
	}
	if g := openspec.ToolsForAdapters([]string{"opencode"}); g != "opencode" {
		t.Fatalf("got %q", g)
	}
	if g := openspec.ToolsForAdapters([]string{"cursor", "opencode"}); g != "cursor,opencode" {
		t.Fatalf("got %q", g)
	}
	if g := openspec.ToolsForAdapters(nil); g != "none" {
		t.Fatalf("got %q", g)
	}
}
