#!/usr/bin/env bash
# Atlas Alpha release-readiness smoke.
# Automates build, local install (temp PREFIX), version, CLI routing,
# init/materialization, Atlas Home, Context Economy, status/doctor,
# and runtime repair in temporary workspaces with ATLAS_HOME isolation.
# Interactive TUI navigation is listed as manual steps at the end.
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

BINARY="${BINARY:-bin/atlas}"
PASS=0
FAIL=0

log() { printf '%s\n' "$*"; }
ok()  { PASS=$((PASS + 1)); log "PASS  $*"; }
bad() { FAIL=$((FAIL + 1)); log "FAIL  $*"; }

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    log "missing required command: $1"
    exit 1
  fi
}

assert_absent_in_repo_root() {
  local path="$1"
  if [[ -e "$ROOT/$path" ]]; then
    bad "repo root must not contain generated runtime artifact: $path"
    return 1
  fi
  ok "repo root clean of $path"
}

# Temp workspaces for the Go helper stay inside the repo so adapter dirs
# (e.g. .cursor) can be created under restricted sandboxes. Do NOT export
# TMPDIR globally — go test uses t.TempDir() and must stay outside the git tree.
SMOKE_TMP="$ROOT/.tmp-smoke-mvp"
rm -rf "$SMOKE_TMP"
mkdir -p "$SMOKE_TMP"
trap 'rm -rf "$SMOKE_TMP"' EXIT
export ATLAS_SMOKE_TMP="$SMOKE_TMP"

# Isolate Atlas Home for the whole smoke suite (including focused package tests).
SMOKE_ATLAS_HOME="$SMOKE_TMP/atlas-home"
mkdir -p "$SMOKE_ATLAS_HOME"
export ATLAS_HOME="$SMOKE_ATLAS_HOME"

require_cmd go
require_cmd make

log "== Atlas Alpha smoke =="
log "repo: $ROOT"
log "ATLAS_HOME: $ATLAS_HOME"
log

log "-- build --"
if make build; then
  ok "make build"
else
  bad "make build"
  exit 1
fi

if [[ ! -x "$BINARY" ]]; then
  bad "binary missing or not executable: $BINARY"
  exit 1
fi
ok "binary present ($BINARY)"

log
log "-- local install (temporary PREFIX) --"
SMOKE_PREFIX="$SMOKE_TMP/prefix"
if PREFIX="$SMOKE_PREFIX" make install; then
  ok "make install PREFIX=$SMOKE_PREFIX"
else
  bad "make install"
fi
INSTALLED="$SMOKE_PREFIX/bin/atlas"
if [[ -x "$INSTALLED" ]]; then
  installed_ver="$("$INSTALLED" --version 2>/dev/null | tr -d '\r' | sed -e 's/[[:space:]]*$//')"
  if [[ -n "$installed_ver" ]]; then
    ok "installed binary --version => $installed_ver"
  else
    bad "installed binary --version empty"
  fi
else
  bad "installed binary missing: $INSTALLED"
fi

log
log "-- atlas --version --"
version_out="$("$BINARY" --version 2>/tmp/atlas-smoke-version.err || true)"
version_err="$(cat /tmp/atlas-smoke-version.err 2>/dev/null || true)"
rm -f /tmp/atlas-smoke-version.err
version_out="$(printf '%s' "$version_out" | tr -d '\r' | sed -e 's/[[:space:]]*$//')"
line_count="$(printf '%s\n' "$version_out" | sed '/^$/d' | wc -l | tr -d ' ')"
if [[ -z "$version_out" ]]; then
  bad "atlas --version produced empty stdout"
elif [[ -n "$version_err" ]]; then
  bad "atlas --version wrote stderr: $version_err"
elif [[ "$line_count" != "1" ]]; then
  bad "atlas --version should print a single line, got: $version_out"
elif [[ "$version_out" == *$'\n'* ]]; then
  bad "atlas --version should be a single token line, got: $version_out"
else
  ok "atlas --version => $version_out"
fi

log
log "-- CLI routing unit checks (TUI does not dump reports to console) --"
if go test ./internal/cli/ -count=1 -run 'TestResolve_Routes|TestExecute_VersionOnlyConsoleOutput|TestExecute_LaunchesTUIRoutes|TestExecute_NoLongConsoleReports'; then
  ok "cli routing / console-discipline tests"
else
  bad "cli routing / console-discipline tests"
fi

log
log "-- lower-level Alpha matrix (temp workspaces + ATLAS_HOME) --"
if go run ./scripts/smokemvp; then
  ok "scripts/smokemvp helper"
else
  bad "scripts/smokemvp helper"
fi

log
log "-- focused package regression --"
if go test ./internal/config/ ./internal/workspace/ ./internal/doctor/ ./internal/home/ ./internal/context/ ./internal/tui/ ./internal/tui/screens/ -count=1; then
  ok "focused package tests"
else
  bad "focused package tests"
fi

log
log "-- repo root must stay free of generated runtime artifacts --"
for path in \
  AGENTS.md \
  AGENT.md \
  CLAUDE.md \
  GEMINI.md \
  .atlas \
  .agents \
  .claude \
  .cursor \
  .opencode
do
  assert_absent_in_repo_root "$path" || true
done

log
log "== Summary: $PASS passed, $FAIL failed =="
if [[ "$FAIL" -ne 0 ]]; then
  exit 1
fi

cat <<'EOF'

Manual TUI / Alpha smoke (not fully automated — requires an interactive terminal):

  Use a temporary project and ATLAS_HOME so ~/.atlas is never touched:

    export ATLAS_HOME=/tmp/atlas-home-test-22
    rm -rf "$ATLAS_HOME"
    mkdir -p /tmp/atlas-alpha-project && cd /tmp/atlas-alpha-project
    # from Atlas repo: make build && ./bin/atlas …
    # or: make install PREFIX="$HOME/.local" && atlas …

  1. `atlas` → Dashboard loads (TUI-first).
  2. `atlas init` → Init Step 1–3 → select Cursor + OpenCode → Apply.
  3. Verify project files:
       AGENTS.md
       .cursor/rules/atlas.mdc
       .opencode/atlas.md
       14 agents under .cursor/agents/ and .opencode/agents/
       .atlas/agent-registry.md
       .atlas/runtime-manifest.yaml
       .atlas/assets.lock.yaml
       .atlas/contracts/sdd-openspec.md
  4. Sidebar shows Configure, Status, Doctor, Runtime Repair, Context Economy.
  5. Context Economy → Review → Apply Update → verify
       $ATLAS_HOME/context/projects/<id>/{index.yaml,capsule.md,packs/…}
       and .atlas/state.yaml refs. No product-repo context/ directory.
  6. `atlas status` / `atlas doctor` are read-only (no mutation, no Home create).
  7. Runtime Repair healthy → Apply is a no-op.
  8. Drift an Atlas agent or delete the SDD contract → Repair restores;
     developer-owned files (README, external agents) stay preserved;
     Context Economy payloads under Atlas Home stay intact.
  9. `atlas start` / `atlas change` / `atlas mcp` → Error dialog (unsupported).
 10. Confirm ~/.atlas was not written while ATLAS_HOME was set.
 11. Confirm Atlas repo root still has no generated runtime artifacts.

EOF

log "smoke-mvp: all automated checks passed"
