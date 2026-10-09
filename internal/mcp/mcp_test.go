package mcp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/opencode"
)

func TestNormalizeTransport(t *testing.T) {
	t.Parallel()
	cases := map[string]mcp.Transport{
		"":                mcp.TransportStdio,
		"stdio":           mcp.TransportStdio,
		"http":            mcp.TransportStreamableHTTP,
		"streamable_http": mcp.TransportStreamableHTTP,
		"sse":             mcp.TransportSSE,
		"SSE":             mcp.TransportSSE,
	}
	for in, want := range cases {
		if got := mcp.NormalizeTransport(in); got != want {
			t.Fatalf("%q => %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"streamble_http", "foobar", "websocket", "STDIOO"} {
		if got := mcp.NormalizeTransport(bad); got != mcp.TransportInvalid {
			t.Fatalf("%q => %q, want TransportInvalid", bad, got)
		}
		if mcp.IsKnownTransport(mcp.Transport(bad)) {
			t.Fatalf("%q must not be a known transport", bad)
		}
	}
	for _, tr := range mcp.SelectableTransports() {
		if tr == mcp.TransportSSE {
			t.Fatal("sse must not be selectable")
		}
	}
}

func TestBuiltinCatalog(t *testing.T) {
	t.Parallel()
	cat := mcp.BuiltinCatalog()
	if len(cat) < 5 {
		t.Fatalf("catalog size = %d", len(cat))
	}
	fs, ok := mcp.BuiltinByID(mcp.BuiltinFilesystem)
	if !ok || fs.Transport != mcp.TransportStdio || fs.Command != "npx" {
		t.Fatalf("filesystem = %#v", fs)
	}
	gh, ok := mcp.BuiltinByID(mcp.BuiltinGitHub)
	if !ok || gh.Endpoint != "https://api.githubcopilot.com/mcp/" || gh.AuthRequirement != mcp.AuthOAuthExternal {
		t.Fatalf("github = %#v", gh)
	}
	jira, ok := mcp.BuiltinByID(mcp.BuiltinJira)
	if !ok || jira.Materializable {
		t.Fatalf("jira must not be materializable without verified endpoint: %#v", jira)
	}
	c7, ok := mcp.BuiltinByID(mcp.BuiltinContext7)
	if !ok || c7.Endpoint != "https://mcp.context7.com/mcp" {
		t.Fatalf("context7 endpoint = %#v", c7)
	}
	ref, ok := c7.HeaderRefs["Authorization"]
	if !ok || ref.Env != "CONTEXT7_API_KEY" || ref.Prefix != "Bearer " {
		t.Fatalf("context7 Authorization header ref = %#v", c7.HeaderRefs)
	}
	if _, has := c7.HeaderRefs["CONTEXT7_API_KEY"]; has {
		t.Fatal("CONTEXT7_API_KEY must not be a header name")
	}
}

func TestValidateHTTPHeaderNames(t *testing.T) {
	t.Parallel()
	base := mcp.Definition{
		ID: "custom-1", DisplayName: "Tools", Source: mcp.SourceCustom,
		Transport: mcp.TransportStreamableHTTP, Endpoint: "https://example.com/mcp",
		Enabled: true, Materializable: true,
	}
	for _, name := range []string{"Authorization", "X-API-Key"} {
		def := base
		def.HeaderRefs = map[string]mcp.HeaderValueRef{name: {Env: "TOKEN"}}
		if err := mcp.ValidateDefinition(def); err != nil {
			t.Fatalf("%q should be valid: %v", name, err)
		}
	}
	for _, name := range []string{"Bad Header", "Authorization:", "Authorization\nX-Evil", " X-Test"} {
		def := base
		def.HeaderRefs = map[string]mcp.HeaderValueRef{name: {Env: "TOKEN"}}
		if err := mcp.ValidateDefinition(def); err == nil {
			t.Fatalf("%q must be rejected", name)
		}
	}
}

func TestValidateDefinitionAndSecrets(t *testing.T) {
	t.Parallel()
	good := mcp.Definition{
		ID: "custom-1", DisplayName: "Tools", Source: mcp.SourceCustom,
		Transport: mcp.TransportStdio, Command: "npx", Args: []string{"-y", "pkg"},
		Materializable: true, AuthRequirement: mcp.AuthNone,
	}
	if err := mcp.ValidateDefinition(good); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.Command = "npx && rm -rf /"
	if err := mcp.ValidateDefinition(bad); err == nil {
		t.Fatal("shell concat must fail")
	}
	httpDef := mcp.Definition{
		ID: "custom-2", DisplayName: "Remote", Source: mcp.SourceCustom,
		Transport: mcp.TransportStreamableHTTP, Endpoint: "https://example.com/mcp",
		Materializable: true, EnvRefs: []string{"API_TOKEN"},
		HeaderRefs:      map[string]mcp.HeaderValueRef{"Authorization": {Env: "API_TOKEN", Prefix: "Bearer "}},
		AuthRequirement: mcp.AuthEnvironmentReference,
	}
	if err := mcp.ValidateDefinition(httpDef); err != nil {
		t.Fatal(err)
	}
	httpDef.Endpoint = "not-a-url"
	if err := mcp.ValidateDefinition(httpDef); err == nil {
		t.Fatal("bad url must fail")
	}
	if !mcp.ContainsSecretMaterial("Bearer ghp_abcdefghijklmnopqrstuvwxyz012345") {
		t.Fatal("expected secret detection")
	}
	if mcp.ContainsSecretMaterial("API_TOKEN") {
		t.Fatal("env name is not a secret")
	}
}

func TestBuildDesiredStateCustom(t *testing.T) {
	t.Parallel()
	state, err := mcp.BuildDesiredState(mcp.Selection{
		BuiltinEnabled: map[string]bool{mcp.BuiltinFilesystem: true},
		Custom: []mcp.CustomSpec{{
			ID: "custom-1", Name: "Company", Transport: "streamable_http",
			CommandOrURL: "https://mcp.example.com/v1", EnvRefs: []string{"COMPANY_TOKEN"},
			Enabled: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.SelectedMaterializable()) != 2 {
		t.Fatalf("materializable = %#v", state.SelectedMaterializable())
	}
}

func TestPlanCreateUpdateNoopRemoveBlock(t *testing.T) {
	t.Parallel()
	def := mustBuiltin(t, mcp.BuiltinFilesystem)
	def.Enabled = true
	snap := mcp.NativeSnapshot{Servers: map[string]map[string]any{}}
	plan := mcp.BuildPlan(mcp.AdapterCursor, []mcp.Definition{def}, snap, nil)
	built := map[string]mcp.NativeEntry{
		"filesystem": {Key: "filesystem", Payload: map[string]any{"command": "npx"}},
	}
	plan = mcp.RefinePlanWithPayloads(plan, built, snap)
	if !plan.HasKind(mcp.ActionCreate) {
		t.Fatalf("want create: %#v", plan.Actions)
	}

	snap.Servers["filesystem"] = map[string]any{"command": "npx"}
	plan = mcp.RefinePlanWithPayloads(
		mcp.BuildPlan(mcp.AdapterCursor, []mcp.Definition{def}, snap, []mcp.OwnedEntry{{DefinitionID: "filesystem", NativeKey: "filesystem"}}),
		built, snap,
	)
	if !plan.HasKind(mcp.ActionUnchanged) {
		t.Fatalf("want unchanged: %#v", plan.Actions)
	}

	snap.Servers["filesystem"] = map[string]any{"command": "other"}
	plan = mcp.RefinePlanWithPayloads(
		mcp.BuildPlan(mcp.AdapterCursor, []mcp.Definition{def}, snap, []mcp.OwnedEntry{{DefinitionID: "filesystem", NativeKey: "filesystem"}}),
		built, snap,
	)
	if !plan.HasKind(mcp.ActionUpdate) {
		t.Fatalf("want update: %#v", plan.Actions)
	}

	plan = mcp.BuildPlan(mcp.AdapterCursor, nil, snap, []mcp.OwnedEntry{{DefinitionID: "filesystem", NativeKey: "filesystem"}})
	if !plan.HasKind(mcp.ActionRemove) {
		t.Fatalf("want remove: %#v", plan.Actions)
	}

	snap.Malformed = true
	snap.MalformError = "bad"
	plan = mcp.BuildPlan(mcp.AdapterCursor, []mcp.Definition{def}, snap, nil)
	if !plan.Blocked || !plan.HasKind(mcp.ActionBlocked) {
		t.Fatalf("want blocked: %#v", plan)
	}
}

func TestCursorSafeMergeAndIdempotence(t *testing.T) {
	root := t.TempDir()
	proj := cursor.New()
	// Pre-existing developer MCP.
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	userCfg := map[string]any{
		"mcpServers": map[string]any{
			"personal-db": map[string]any{"command": "node", "args": []any{"db.js"}},
		},
	}
	writeJSON(t, filepath.Join(root, cursor.ConfigRelPath), userCfg)

	def := mustBuiltin(t, mcp.BuiltinFilesystem)
	def.Enabled = true
	home := t.TempDir()
	pid := "proj-test"
	res, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: pid,
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{def}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: proj},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Blocked {
		t.Fatalf("blocked: %v", res.Errors)
	}
	snap, err := proj.InspectMCPProjection(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snap.Servers["personal-db"]; !ok {
		t.Fatal("user entry not preserved")
	}
	if _, ok := snap.Servers["filesystem"]; !ok {
		t.Fatal("atlas entry missing")
	}

	// Idempotent second apply.
	before, _ := os.ReadFile(filepath.Join(root, cursor.ConfigRelPath))
	res2, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: pid,
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{def}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: proj},
		Ownership:  res.Ownership,
	})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(root, cursor.ConfigRelPath))
	if string(before) != string(after) {
		t.Fatalf("second apply mutated file\n before=%s\n after=%s", before, after)
	}
	if res2.Plans[mcp.AdapterCursor].NeedsWrite() {
		t.Fatalf("second plan should be noop: %#v", res2.Plans[mcp.AdapterCursor])
	}

	// Malformed blocks.
	if err := os.WriteFile(filepath.Join(root, cursor.ConfigRelPath), []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	malformedBefore, _ := os.ReadFile(filepath.Join(root, cursor.ConfigRelPath))
	_, err = mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: pid,
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{def}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: proj},
		Ownership:  res.Ownership,
	})
	if err == nil {
		t.Fatal("expected block on malformed")
	}
	malformedAfter, _ := os.ReadFile(filepath.Join(root, cursor.ConfigRelPath))
	if string(malformedBefore) != string(malformedAfter) {
		t.Fatal("malformed config was overwritten")
	}
}

func TestOpenCodeProjectionAndAdapterSwitch(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	pid := "proj-oc"
	fs := mustBuiltin(t, mcp.BuiltinFilesystem)
	fs.Enabled = true
	gh := mustBuiltin(t, mcp.BuiltinGitHub)
	gh.Enabled = true

	cProj := cursor.New()
	oProj := opencode.New()
	projectors := map[mcp.AdapterID]mcp.Projector{
		mcp.AdapterCursor:   cProj,
		mcp.AdapterOpenCode: oProj,
	}

	// Seed developer cursor MCP.
	if err := os.MkdirAll(filepath.Join(root, ".cursor"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(root, cursor.ConfigRelPath), map[string]any{
		"mcpServers": map[string]any{"private-tool": map[string]any{"command": "tool"}},
	})

	res, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: pid,
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs, gh}},
		Adapters:   []mcp.AdapterID{mcp.AdapterCursor},
		Projectors: projectors,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Switch to OpenCode.
	res2, err := mcp.Reconcile(mcp.ReconcileInput{
		Root: root, HomePath: home, ProjectID: pid,
		Desired:    mcp.DesiredState{Definitions: []mcp.Definition{fs, gh}},
		Adapters:   []mcp.AdapterID{mcp.AdapterOpenCode},
		Projectors: projectors,
		Ownership:  res.Ownership,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = res2
	cSnap, _ := cProj.InspectMCPProjection(root)
	if _, ok := cSnap.Servers["private-tool"]; !ok {
		t.Fatal("developer cursor MCP removed")
	}
	if _, ok := cSnap.Servers["filesystem"]; ok {
		t.Fatal("atlas filesystem should be removed from cursor")
	}
	if _, ok := cSnap.Servers["github"]; ok {
		t.Fatal("atlas github should be removed from cursor")
	}
	oSnap, _ := oProj.InspectMCPProjection(root)
	if _, ok := oSnap.Servers["filesystem"]; !ok {
		t.Fatal("opencode filesystem missing")
	}
	if _, ok := oSnap.Servers["github"]; !ok {
		t.Fatal("opencode github missing")
	}
	// No raw credential in github projection.
	raw, _ := os.ReadFile(filepath.Join(root, opencode.ConfigRelPath))
	if strings.Contains(string(raw), "ghp_") || strings.Contains(strings.ToLower(string(raw)), "bearer ") {
		t.Fatalf("raw credential leaked: %s", raw)
	}
}

func TestHealthReadOnly(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	path := filepath.Join(root, cursor.ConfigRelPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, path, map[string]any{"mcpServers": map[string]any{}})
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	before := info.ModTime()
	beforeBytes, _ := os.ReadFile(path)

	def := mustBuiltin(t, mcp.BuiltinGitHub)
	def.Enabled = true
	h := mcp.EvaluateHealth(root, home, "p", mcp.DesiredState{Definitions: []mcp.Definition{def}},
		[]mcp.AdapterID{mcp.AdapterCursor},
		map[mcp.AdapterID]mcp.Projector{mcp.AdapterCursor: cursor.New()})
	if h.SelectedCount != 1 {
		t.Fatalf("selected=%d", h.SelectedCount)
	}
	info2, _ := os.Stat(path)
	afterBytes, _ := os.ReadFile(path)
	if !info2.ModTime().Equal(before) || string(beforeBytes) != string(afterBytes) {
		t.Fatal("health mutated native config")
	}
}

func mustBuiltin(t *testing.T, id string) mcp.Definition {
	t.Helper()
	def, ok := mcp.BuiltinByID(id)
	if !ok {
		t.Fatalf("missing builtin %s", id)
	}
	return def
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
