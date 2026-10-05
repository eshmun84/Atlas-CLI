#!/usr/bin/env bash
# Atlas MVP release-readiness smoke.
# Automates build, version, CLI routing, init/materialization, status/doctor
# surfaces, and runtime repair lower-level flows in temporary workspaces.
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

require_cmd go
require_cmd make

log "== Atlas MVP smoke =="
log "repo: $ROOT"
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
log "-- lower-level MVP matrix (temp workspaces) --"
if go run ./scripts/smokemvp; then
  ok "scripts/smokemvp helper"
else
  bad "scripts/smokemvp helper"
fi

log
log "-- focused package regression --"
if go test ./internal/config/ ./internal/workspace/ ./internal/doctor/ ./internal/tui/ ./internal/tui/screens/ -count=1; then
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

Manual TUI smoke (not automated — requires an interactive terminal):

  1. From a non-Atlas project directory: run `atlas` → Dashboard loads.
  2. `atlas init` → complete Init Step 1–3 → Apply config → confirm
     `.atlas/`, `AGENTS.md`, and selected adapter projections exist.
  3. After init: sidebar shows Configure, Status, Doctor, Runtime Repair.
  4. `atlas status` / `atlas doctor` open TUI screens (no console reports).
  5. Runtime Repair → Review healthy workspace → Apply is a no-op.
  6. Delete `AGENTS.md` or a selected projection → Repair recreates them.
  7. Introduce `CLAUDE.md` / broken markers → Repair backups then quarantines.
  8. `atlas start` / `atlas change` / `atlas mcp` → Error dialog (unsupported).
  9. Confirm Atlas repo root still has no generated runtime artifacts.

EOF

log "smoke-mvp: all automated checks passed"
