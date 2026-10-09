package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eshmun84/Atlas-CLI/internal/config"
	"github.com/eshmun84/Atlas-CLI/internal/home"
	"github.com/eshmun84/Atlas-CLI/internal/mcp"
	"github.com/eshmun84/Atlas-CLI/internal/mcp/cursor"
)

// Finding 1: Init Home rollback restores pre-existing project Home tree byte-for-byte.
func TestInitApply_HomeResetMCPFailureRestoresPreexistingHomeTree(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "home-rb", ProjectMode: "new", ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	_ = draft.ToggleMulti("adapters.selected", "opencode")
	fixed := time.Date(2026, 10, 8, 22, 0, 0, 0, time.UTC)
	res, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(), Now: func() time.Time { return fixed },
	})
	if err != nil {
		t.Fatal(err)
	}
	pid := res.ProjectID
	homePath := res.HomePath

	markers := map[string][]byte{
		filepath.Join("context", "marker"):          []byte("ctx-marker\n"),
		filepath.Join("codegraph", "graph.db"):      []byte("GRAPHDB"),
		filepath.Join("codegraph", "metadata.json"): []byte(`{"ok":true}` + "\n"),
		filepath.Join("backups", "marker"):          []byte("bak-marker\n"),
		filepath.Join("diagnostics", "marker"):      []byte("diag-marker\n"),
		filepath.Join("state", "local.yaml"):        []byte("local: true\n"),
		filepath.Join("mcp", "ownership.yaml"):      []byte("schema_version: 1\nproject_id: " + pid + "\n"),
	}
	proj := home.ProjectRoot(homePath, pid)
	for rel, data := range markers {
		path := filepath.Join(proj, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	orig := config.MCPProjectors()
	t.Cleanup(func() {
		for _, p := range orig {
			mcp.RegisterDefaultProjector(p)
		}
	})
	mcp.RegisterDefaultProjector(cursor.New())
	mcp.RegisterDefaultProjector(boomOpenCode{})

	mcpDraft := config.EmptyMCPDraft()
	mcpDraft.EnableBuiltin(config.MCPBuiltinFilesystem)
	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: mcpDraft, AcceptHomeReset: true,
		Now: func() time.Time { return fixed.Add(time.Minute) },
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected MCP rollback, got %v", err)
	}

	for rel, want := range markers {
		got, readErr := os.ReadFile(filepath.Join(proj, rel))
		if readErr != nil {
			t.Fatalf("missing restored %s: %v", rel, readErr)
		}
		if string(got) != string(want) {
			t.Fatalf("restored %s = %q, want %q", rel, got, want)
		}
	}
	// No leftover staging dir and no successful new Apply state for MCP ownership claim.
	entries, _ := os.ReadDir(filepath.Join(homePath, "projects"))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".atlas-init-tx-") {
			t.Fatalf("staging dir leaked: %s", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas", "state.yaml")); err == nil {
		// Pre-existing successful init may still have state; ensure no filesystem MCP claim leaked into new ownership after rollback.
		own, _ := os.ReadFile(filepath.Join(proj, "mcp", "ownership.yaml"))
		if strings.Contains(string(own), "filesystem") && !strings.Contains(string(markers[filepath.Join("mcp", "ownership.yaml")]), "filesystem") {
			t.Fatalf("ownership mutated after rollback: %s", own)
		}
	}
}

// Finding 2: post-mutation Init errors roll back (docs scaffold failure injection).
func TestInitApply_DocsFailureRollsBack(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs", "atlas"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Directory where a file is required — EnsureProjectDocsScaffold should fail.
	if err := os.Mkdir(filepath.Join(root, "docs", "atlas", "README.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "docs-fail", ProjectMode: "new", ToolCursorAvailable: true, DocsScaffold: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcpDraft := config.EmptyMCPDraft()
	mcpDraft.EnableBuiltin(config.MCPBuiltinFilesystem)

	// Preflight should catch this before mutation.
	_, err := config.ApplyConfig(config.ApplyInput{Root: root, Draft: draft, MCP: mcpDraft})
	if err == nil {
		t.Fatal("expected docs preflight failure")
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas", "config.yaml")); !os.IsNotExist(err) {
		t.Fatal("preflight failure must leave zero workspace mutation")
	}
}

// Finding 7: Configure Apply docs failure rolls back config + MCP + ownership.
func TestPersistConfigure_DocsFailureRollsBackConfigAndMCP(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "cfg-docs", ProjectMode: "new", ToolCursorAvailable: true,
	})
	_ = draft.ToggleMulti("adapters.selected", "cursor")
	mcp0 := config.EmptyMCPDraft()
	mcp0.EnableBuiltin(config.MCPBuiltinFilesystem)
	fixed := time.Date(2026, 10, 8, 22, 30, 0, 0, time.UTC)
	if _, err := config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: mcp0, Now: func() time.Time { return fixed },
	}); err != nil {
		t.Fatal(err)
	}
	beforeCfg, _ := os.ReadFile(filepath.Join(root, config.FileConfig))
	beforeCursor, _ := os.ReadFile(filepath.Join(root, cursor.ConfigRelPath))
	homePath := os.Getenv("ATLAS_HOME")
	pid, _ := home.ProjectID(root, "cfg-docs")
	beforeOwn, ownErr := os.ReadFile(filepath.Join(homePath, "projects", pid, "mcp", "ownership.yaml"))
	if ownErr != nil {
		t.Fatal(ownErr)
	}

	// Create docs path as a directory so scaffold fails after mutations if preflight is bypassed;
	// with preflight, Apply must fail before mutation — also cover conflict after selecting docs.
	if err := os.MkdirAll(filepath.Join(root, "docs", "atlas", "README.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := config.BuildConfigDraft(config.ConfigModeConfigure, config.ProjectSetupInput{
		ProjectName: "cfg-docs", ProjectMode: "existing", ToolCursorAvailable: true,
	})
	_ = cfg.ToggleMulti("adapters.selected", "cursor")
	_ = cfg.SetValue("project.docs_scaffold", "true")
	mcp1 := config.EmptyMCPDraft()
	mcp1.EnableBuiltin(config.MCPBuiltinFilesystem)
	mcp1.EnableBuiltin(config.MCPBuiltinGitHub)

	_, err := config.PersistConfigure(config.ApplyInput{Root: root, Draft: cfg, MCP: mcp1})
	if err == nil {
		t.Fatal("expected docs failure")
	}
	afterCfg, _ := os.ReadFile(filepath.Join(root, config.FileConfig))
	if string(afterCfg) != string(beforeCfg) {
		t.Fatal("config.yaml not restored")
	}
	afterCursor, _ := os.ReadFile(filepath.Join(root, cursor.ConfigRelPath))
	if string(afterCursor) != string(beforeCursor) {
		t.Fatalf("cursor mcp not restored:\n%s", afterCursor)
	}
	afterOwn, _ := os.ReadFile(filepath.Join(homePath, "projects", pid, "mcp", "ownership.yaml"))
	if string(afterOwn) != string(beforeOwn) {
		t.Fatalf("ownership not restored:\n%s", afterOwn)
	}
	if strings.Contains(string(afterCursor), `"github"`) {
		t.Fatal("github must not remain after docs/configure failure")
	}
}

// Finding 9: general backup IDs are unique at identical Now.
func TestBackupExistingTargets_UniqueStampSameNow(t *testing.T) {
	withTempAtlasHome(t)
	root := t.TempDir()
	homePath := os.Getenv("ATLAS_HOME")
	pid := "backup-uniq-test"
	if err := home.EnsureProjectLayout(homePath, pid); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 8, 23, 0, 0, 0, time.UTC)
	dir1, m1, _, err := config.BackupExistingTargets(root, homePath, pid, []string{"AGENTS.md"}, now)
	if err != nil || dir1 == "" {
		t.Fatalf("backup1: %q %v", dir1, err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir2, m2, _, err := config.BackupExistingTargets(root, homePath, pid, []string{"AGENTS.md"}, now)
	if err != nil || dir2 == "" {
		t.Fatalf("backup2: %q %v", dir2, err)
	}
	if dir1 == dir2 {
		t.Fatalf("backup dirs collided: %s", dir1)
	}
	if len(m1.Entries) != 1 || len(m2.Entries) != 1 {
		t.Fatalf("manifests: %#v %#v", m1, m2)
	}
	b1, err := os.ReadFile(filepath.Join(homePath, filepath.FromSlash(m1.Entries[0].BackupPath)))
	if err != nil || string(b1) != "v1\n" {
		t.Fatalf("backup1 content: %q %v", b1, err)
	}
	b2, err := os.ReadFile(filepath.Join(homePath, filepath.FromSlash(m2.Entries[0].BackupPath)))
	if err != nil || string(b2) != "v2\n" {
		t.Fatalf("backup2 content: %q %v", b2, err)
	}
}

// Finding 12: reset human gate before any Home mutation (including EnsureAndMirror).
func TestApplyConfig_HomeResetGateBeforeEnsureAndMirror(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("ATLAS_HOME", homeDir)
	root := t.TempDir()
	draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
		ProjectName: "gate", ProjectMode: "new",
	})
	pid, err := home.ProjectID(root, "gate")
	if err != nil {
		t.Fatal(err)
	}
	markerRel := filepath.Join("projects", pid, "context", "marker")
	markerPath := filepath.Join(homeDir, markerRel)
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(markerPath, []byte("pre\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	infoBefore, err := os.Stat(markerPath)
	if err != nil {
		t.Fatal(err)
	}

	_, err = config.ApplyConfig(config.ApplyInput{
		Root: root, Draft: draft, MCP: config.EmptyMCPDraft(), AcceptHomeReset: false,
	})
	if err == nil || !strings.Contains(err.Error(), "explicit reset") {
		t.Fatalf("expected reset gate, got %v", err)
	}

	// EnsureAndMirror must not have run: no mirrored assets, marker unchanged.
	if _, err := os.Stat(filepath.Join(homeDir, "assets")); !os.IsNotExist(err) {
		t.Fatal("EnsureAndMirror must not run before reset acceptance")
	}
	if _, err := os.Stat(filepath.Join(homeDir, "state")); !os.IsNotExist(err) {
		t.Fatal("Home state must remain absent before reset acceptance")
	}
	got, _ := os.ReadFile(markerPath)
	if string(got) != "pre\n" {
		t.Fatalf("marker mutated: %q", got)
	}
	infoAfter, _ := os.Stat(markerPath)
	if !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatal("marker mtime changed")
	}
	if _, err := os.Stat(filepath.Join(root, ".atlas")); !os.IsNotExist(err) {
		t.Fatal("workspace must remain unchanged")
	}
}

// Finding 3 (workspace): Init/Configure block symlink parents with zero external write.
func TestApplyConfig_BlocksWorkspaceSymlinkParents(t *testing.T) {
	withTempAtlasHome(t)
	for _, link := range []string{".atlas", ".cursor", ".opencode"} {
		t.Run(link, func(t *testing.T) {
			root := t.TempDir()
			outside := t.TempDir()
			if err := os.Symlink(outside, filepath.Join(root, link)); err != nil {
				t.Fatal(err)
			}
			draft := config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
				ProjectName: "sym-" + link, ProjectMode: "new",
				ToolCursorAvailable: true, ToolOpenCodeAvailable: true,
			})
			_ = draft.ToggleMulti("adapters.selected", "cursor")
			_ = draft.ToggleMulti("adapters.selected", "opencode")
			_, err := config.ApplyConfig(config.ApplyInput{Root: root, Draft: draft, MCP: config.EmptyMCPDraft()})
			if err == nil {
				t.Fatal("expected symlink block")
			}
			entries, _ := os.ReadDir(outside)
			if len(entries) != 0 {
				t.Fatalf("external write via %s: %v", link, entries)
			}
		})
	}
}

// Finding 3 (Home): writers block when ATLAS_HOME / projects / project subdirs are symlinks.
func TestHomeWriters_BlockSymlinkComponents(t *testing.T) {
	t.Run("home_root_symlink", func(t *testing.T) {
		real := t.TempDir()
		outside := t.TempDir()
		link := filepath.Join(t.TempDir(), "home-link")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}
		t.Setenv("ATLAS_HOME", link)
		root := t.TempDir()
		_, err := config.ApplyConfig(config.ApplyInput{
			Root: root,
			Draft: config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
				ProjectName: "sym-home", ProjectMode: "new",
			}),
			MCP: config.EmptyMCPDraft(),
		})
		if err == nil {
			t.Fatal("expected ATLAS_HOME symlink block")
		}
		assertOutsideEmpty(t, outside)
	})

	t.Run("projects_symlink", func(t *testing.T) {
		homeDir := t.TempDir()
		outside := t.TempDir()
		t.Setenv("ATLAS_HOME", homeDir)
		if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(filepath.Join(homeDir, "projects")); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(homeDir, "projects")); err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		_, err := config.ApplyConfig(config.ApplyInput{
			Root: root,
			Draft: config.BuildConfigDraft(config.ConfigModeInit, config.ProjectSetupInput{
				ProjectName: "sym-projects", ProjectMode: "new",
			}),
			MCP: config.EmptyMCPDraft(),
		})
		if err == nil {
			t.Fatal("expected projects symlink block")
		}
		assertOutsideEmpty(t, outside)
	})

	for _, sub := range []string{"context", "codegraph", "mcp", "backups"} {
		sub := sub
		t.Run(sub+"_symlink", func(t *testing.T) {
			homeDir := t.TempDir()
			outside := t.TempDir()
			t.Setenv("ATLAS_HOME", homeDir)
			root := t.TempDir()
			pid, err := home.ProjectID(root, "sym-"+sub)
			if err != nil {
				t.Fatal(err)
			}
			base := filepath.Join(homeDir, "projects", pid)
			if err := os.MkdirAll(base, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(outside, filepath.Join(base, sub)); err != nil {
				t.Fatal(err)
			}
			switch sub {
			case "mcp":
				err = mcp.SaveOwnership(homeDir, pid, mcp.OwnershipDocument{SchemaVersion: 1, ProjectID: pid})
			case "backups":
				if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("x\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				_, _, _, err = config.BackupExistingTargets(root, homeDir, pid, []string{"AGENTS.md"}, time.Now().UTC())
			case "context":
				err = writeViaFsafetyHome(homeDir, filepath.Join("projects", pid, "context", "marker"), []byte("x\n"))
			case "codegraph":
				err = writeViaFsafetyHome(homeDir, filepath.Join("projects", pid, "codegraph", "metadata.json"), []byte("{}\n"))
			}
			if err == nil {
				t.Fatalf("expected %s symlink block", sub)
			}
			assertOutsideEmpty(t, outside)
		})
	}

	t.Run("project_id_symlink", func(t *testing.T) {
		homeDir := t.TempDir()
		outside := t.TempDir()
		t.Setenv("ATLAS_HOME", homeDir)
		root := t.TempDir()
		pid, err := home.ProjectID(root, "sym-pid")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(homeDir, "projects"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, filepath.Join(homeDir, "projects", pid)); err != nil {
			t.Fatal(err)
		}
		err = home.EnsureProjectLayout(homeDir, pid)
		if err == nil {
			t.Fatal("expected project id symlink block")
		}
		assertOutsideEmpty(t, outside)
	})
}

func assertOutsideEmpty(t *testing.T, outside string) {
	t.Helper()
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("external write detected: %v", entries)
	}
}

func writeViaFsafetyHome(homePath, rel string, data []byte) error {
	return mcp.AtomicWriteContained(homePath, filepath.ToSlash(rel), data, 0o644)
}

func TestInvalidTransportRejectedOnCustomAdd(t *testing.T) {
	t.Parallel()
	draft := config.EmptyMCPDraft()
	_, err := draft.AddCustom("Bad", config.MCPTransport("streamble_http"), "https://example.com/mcp", "", "")
	if err == nil {
		t.Fatal("typo transport must be rejected")
	}
	_, err = draft.AddCustom("Bad2", config.MCPTransport("foobar"), "https://example.com/mcp", "", "")
	if err == nil {
		t.Fatal("unknown transport must be rejected")
	}
}

func TestRenderAgentsMD_UsesTrustedCanonicalDespiteHomeDrift(t *testing.T) {
	withTempAtlasHome(t)
	homePath := os.Getenv("ATLAS_HOME")
	if _, err := home.EnsureAndMirror(time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	tampered := filepath.Join(homePath, "assets", "agents", "base.md")
	if err := os.WriteFile(tampered, []byte("TAMPERED BASE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := mustRenderAgentsMD(t, "demo", true, []string{"cursor"}, nil)
	if strings.Contains(out, "TAMPERED BASE") {
		t.Fatal("RenderAgentsMD trusted drifted Home content")
	}
	if len(out) < 40 {
		t.Fatalf("unexpected empty render: %q", out)
	}
}
