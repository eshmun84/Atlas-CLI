/**
 * Bun runtime tests for the Atlas OpenCode hooks plugin.
 * Exercises real subprocess behavior via PATH-shadowed mock `atlas` binaries.
 */
import { afterEach, beforeEach, describe, expect, test } from "bun:test"
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  symlinkSync,
  statSync,
  writeFileSync,
} from "node:fs"
import { homedir, tmpdir } from "node:os"
import { join } from "node:path"

const mod = await import("./index.ts")
const {
  MAX_BYTES,
  MAX_STDERR_BYTES,
  MAX_STDOUT_BYTES,
  TRACE_ENV,
  allowDecision,
  configureAtlasDispatchCwd,
  dispatchAtlas,
  directoryFromSessionRecord,
  dispatchOwnedHook,
  handleSessionCreatedEvent,
  isSessionCreatedEvent,
  isValidSessionID,
  parseAtlasResponse,
  processSubscribeEvent,
  resetAtlasDispatchCwdForTest,
  resolveAtlasDispatchCwd,
  resolveTraceHome,
  sessionIDFrom,
  shouldDenyToolExecution,
  traceHooks,
  utf8ByteLength,
} = mod

const originalPath = process.env.PATH ?? ""
const originalTrace = process.env[TRACE_ENV]
const originalAtlasHome = process.env.ATLAS_HOME
let sandbox = ""

function writeMockAtlas(script: string): void {
  const bin = join(sandbox, "bin")
  mkdirSync(bin, { recursive: true })
  const atlas = join(bin, "atlas")
  writeFileSync(atlas, script, { encoding: "utf8" })
  chmodSync(atlas, 0o755)
  process.env.PATH = `${bin}:${originalPath}`
}

beforeEach(() => {
  sandbox = mkdtempSync(join(tmpdir(), "atlas-hooks-plugin-"))
  process.env.PATH = originalPath
  if (originalTrace === undefined) delete process.env[TRACE_ENV]
  else process.env[TRACE_ENV] = originalTrace
  if (originalAtlasHome === undefined) delete process.env.ATLAS_HOME
  else process.env.ATLAS_HOME = originalAtlasHome
  mod.setTestTimeoutOverrideMs(undefined)
  // Default: project cwd for dispatch (mirrors setup(ctx.location.directory)).
  expect(configureAtlasDispatchCwd(sandbox)).toBe(true)
})

afterEach(() => {
  process.env.PATH = originalPath
  if (originalTrace === undefined) delete process.env[TRACE_ENV]
  else process.env[TRACE_ENV] = originalTrace
  if (originalAtlasHome === undefined) delete process.env.ATLAS_HOME
  else process.env.ATLAS_HOME = originalAtlasHome
  mod.setTestTimeoutOverrideMs(undefined)
  resetAtlasDispatchCwdForTest()
  if (sandbox) {
    rmSync(sandbox, { recursive: true, force: true })
    sandbox = ""
  }
})

describe("utf8ByteLength", () => {
  test("ASCII at boundary", () => {
    const ascii = "a".repeat(MAX_BYTES)
    expect(utf8ByteLength(ascii)).toBe(MAX_BYTES)
    expect(ascii.length).toBe(MAX_BYTES)
  })

  test("multibyte Unicode at boundary", () => {
    // U+1F600 is 4 UTF-8 bytes but 2 UTF-16 code units.
    const emoji = "😀"
    expect(utf8ByteLength(emoji)).toBe(4)
    expect(emoji.length).toBe(2)
    const count = Math.floor(MAX_BYTES / 4)
    const body = emoji.repeat(count)
    expect(utf8ByteLength(body)).toBe(count * 4)
    expect(body.length).toBe(count * 2)
  })

  test("empty payload", () => {
    expect(utf8ByteLength("")).toBe(0)
  })
})

describe("parseAtlasResponse", () => {
  test("valid allow", () => {
    expect(parseAtlasResponse(`{"schema_version":1,"status":"success","decision":"allow"}`)).toEqual({
      schema_version: 1,
      status: "success",
      decision: "allow",
    })
  })

  test("valid deny", () => {
    const res = parseAtlasResponse(
      `{"schema_version":1,"status":"denied","decision":"deny","message":"blocked by atlas hooks"}`,
    )
    expect(res?.decision).toBe("deny")
    expect(res?.status).toBe("denied")
    expect(shouldDenyToolExecution(res!)).toBe(true)
  })

  test("invalid schema version", () => {
    expect(parseAtlasResponse(`{"schema_version":2,"status":"success","decision":"allow"}`)).toBeNull()
  })

  test("unknown decision", () => {
    expect(parseAtlasResponse(`{"schema_version":1,"status":"success","decision":"block"}`)).toBeNull()
  })

  test("unknown status", () => {
    expect(parseAtlasResponse(`{"schema_version":1,"status":"weird","decision":"allow"}`)).toBeNull()
  })

  test("missing required fields", () => {
    expect(parseAtlasResponse(`{"schema_version":1,"decision":"allow"}`)).toBeNull()
    expect(parseAtlasResponse(`{"status":"success","decision":"allow"}`)).toBeNull()
  })

  test("invalid types", () => {
    expect(parseAtlasResponse(`{"schema_version":"1","status":"success","decision":"allow"}`)).toBeNull()
    expect(parseAtlasResponse(`{"schema_version":1,"status":1,"decision":"allow"}`)).toBeNull()
    expect(
      parseAtlasResponse(`{"schema_version":1,"status":"denied","decision":"deny","message":1}`),
    ).toBeNull()
  })

  test("contradictory response", () => {
    expect(parseAtlasResponse(`{"schema_version":1,"status":"success","decision":"deny"}`)).toBeNull()
  })

  test("contradictory status=denied with decision=allow is rejected", () => {
    // Go TranslateResponse never emits this pairing; plugin must fail-open.
    expect(parseAtlasResponse(`{"schema_version":1,"status":"denied","decision":"allow"}`)).toBeNull()
  })

  test("shouldDenyToolExecution requires denied+deny pair", () => {
    expect(
      shouldDenyToolExecution({ schema_version: 1, status: "denied", decision: "deny" }),
    ).toBe(true)
    expect(
      shouldDenyToolExecution({ schema_version: 1, status: "success", decision: "allow" }),
    ).toBe(false)
  })

  test("native plugin blocks tool execution only on valid deny", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"denied","decision":"deny","message":"blocked by atlas hooks"}'
exit 0
`)
    const res = await dispatchAtlas({
      hook_event_name: "tool.execute.before",
      tool_name: "bash",
    })
    expect(shouldDenyToolExecution(res)).toBe(true)
    // Mirrors execute.before handler: throw fixed public message only.
    expect(() => {
      if (shouldDenyToolExecution(res)) throw new Error(mod.DENY_MESSAGE)
    }).toThrow(mod.DENY_MESSAGE)
  })

  test("truncated JSON", () => {
    expect(parseAtlasResponse(`{"schema_version":1,"status":"denied","decision":`)).toBeNull()
  })

  test("concatenated JSON", () => {
    expect(
      parseAtlasResponse(
        `{"schema_version":1,"status":"denied","decision":"deny"}{"schema_version":1,"status":"success","decision":"allow"}`,
      ),
    ).toBeNull()
  })

  test("non-object JSON", () => {
    expect(parseAtlasResponse(`["deny"]`)).toBeNull()
    expect(parseAtlasResponse(`null`)).toBeNull()
    expect(parseAtlasResponse(`"deny"`)).toBeNull()
  })

  test("sensitive-looking fields ignored", () => {
    const res = parseAtlasResponse(
      `{"schema_version":1,"status":"success","decision":"allow","secret":"TOKEN","prompt":"LEAK"}`,
    )
    expect(res).toEqual({ schema_version: 1, status: "success", decision: "allow" })
    expect(Object.prototype.hasOwnProperty.call(res, "secret")).toBe(false)
  })
})

describe("resolveAtlasDispatchCwd", () => {
  test("accepts existing directory", () => {
    expect(resolveAtlasDispatchCwd(sandbox)).toBe(sandbox)
  })

  test("rejects missing path, file, empty, and non-string", () => {
    expect(resolveAtlasDispatchCwd(join(sandbox, "missing"))).toBeUndefined()
    const file = join(sandbox, "not-a-dir")
    writeFileSync(file, "x")
    expect(resolveAtlasDispatchCwd(file)).toBeUndefined()
    expect(resolveAtlasDispatchCwd("")).toBeUndefined()
    expect(resolveAtlasDispatchCwd("   ")).toBeUndefined()
    expect(resolveAtlasDispatchCwd(null)).toBeUndefined()
    expect(resolveAtlasDispatchCwd(42)).toBeUndefined()
  })
})

describe("dispatchAtlas subprocess", () => {
  test("spawns Atlas with configured project cwd (not process cwd)", async () => {
    const project = join(sandbox, "project-root")
    mkdirSync(project, { recursive: true })
    expect(configureAtlasDispatchCwd(project)).toBe(true)
    const marker = join(sandbox, "cwd.txt")
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
pwd > "${marker}"
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"success","decision":"allow"}'
exit 0
`)
    const previous = process.cwd()
    const foreign = join(sandbox, "foreign-cwd")
    mkdirSync(foreign, { recursive: true })
    try {
      process.chdir(foreign)
      const res = await dispatchAtlas({
        hook_event_name: "tool.execute.after",
        tool_name: "write",
        status: "success",
        session_id: "s1",
      })
      expect(res.decision).toBe("allow")
      expect(res.status).toBe("success")
      const cwdUsed = realpathSync(readFileSync(marker, "utf8").trim())
      expect(cwdUsed).toBe(realpathSync(project))
      expect(cwdUsed).not.toBe(realpathSync(foreign))
    } finally {
      process.chdir(previous)
    }
  })

  test("invalid cwd fails open without spawning Atlas", async () => {
    resetAtlasDispatchCwdForTest()
    const marker = join(sandbox, "spawned")
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
echo spawned > "${marker}"
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"denied","decision":"deny"}'
`)
    const res = await dispatchAtlas({ hook_event_name: "session.created", session_id: "s1" })
    expect(res.decision).toBe("allow")
    expect(existsSync(marker)).toBe(false)

    expect(configureAtlasDispatchCwd(join(sandbox, "gone"))).toBe(false)
    const res2 = await dispatchAtlas({
      hook_event_name: "tool.execute.after",
      tool_name: "write",
      status: "success",
    })
    expect(res2.decision).toBe("allow")
    expect(existsSync(marker)).toBe(false)
  })

  test("preserves ATLAS_HOME from parent env and does not invent it", async () => {
    const homeMarker = join(sandbox, "atlas-home-env.txt")
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
printf '%s' "\${ATLAS_HOME-__UNSET__}" > "${homeMarker}"
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"success","decision":"allow"}'
exit 0
`)
    delete process.env.ATLAS_HOME
    await dispatchAtlas({ hook_event_name: "session.created", session_id: "s1" })
    expect(readFileSync(homeMarker, "utf8")).toBe("__UNSET__")

    const qaHome = join(sandbox, "qa-atlas-home")
    mkdirSync(qaHome, { recursive: true })
    process.env.ATLAS_HOME = qaHome
    await dispatchAtlas({
      hook_event_name: "tool.execute.after",
      tool_name: "write",
      status: "success",
      session_id: "s2",
    })
    expect(readFileSync(homeMarker, "utf8")).toBe(qaHome)
  })

  test("session.created (session.start path) still dispatches", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"success","decision":"allow"}'
exit 0
`)
    const res = await dispatchAtlas({ hook_event_name: "session.created", session_id: "s1" })
    expect(res).toEqual(allowDecision())
  })

  test("normal completion allow", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"success","decision":"allow"}'
exit 0
`)
    const res = await dispatchAtlas({ hook_event_name: "session.created", session_id: "s1" })
    expect(res).toEqual(allowDecision())
  })

  test("normal completion deny", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"denied","decision":"deny","message":"blocked by atlas hooks"}'
exit 0
`)
    const res = await dispatchAtlas({
      hook_event_name: "tool.execute.before",
      tool_name: "bash",
    })
    expect(shouldDenyToolExecution(res)).toBe(true)
  })

  test("missing Atlas executable", async () => {
    process.env.PATH = join(sandbox, "empty-bin")
    mkdirSync(process.env.PATH, { recursive: true })
    const res = await dispatchAtlas({ hook_event_name: "session.created", session_id: "s1" })
    expect(res.decision).toBe("allow")
  })

  test("subprocess startup failure via non-executable", async () => {
    const bin = join(sandbox, "bin")
    mkdirSync(bin, { recursive: true })
    const atlas = join(bin, "atlas")
    writeFileSync(atlas, "not a script", { encoding: "utf8" })
    chmodSync(atlas, 0o644)
    process.env.PATH = `${bin}:${originalPath}`
    const res = await dispatchAtlas({ hook_event_name: "session.created", session_id: "s1" })
    expect(res.decision).toBe("allow")
  })

  test("timeout after reading stdin", async () => {
    mod.setTestTimeoutOverrideMs(200)
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
sleep 30
printf '%s\\n' '{"schema_version":1,"status":"denied","decision":"deny"}'
`)
    const start = Date.now()
    const res = await dispatchAtlas({ hook_event_name: "tool.execute.before", tool_name: "bash" })
    const elapsed = Date.now() - start
    expect(res.decision).toBe("allow")
    expect(shouldDenyToolExecution(res)).toBe(false)
    expect(elapsed).toBeLessThan(5_000)
  })

  test("timeout when child never reads stdin", async () => {
    mod.setTestTimeoutOverrideMs(200)
    // Child ignores stdin entirely and sleeps — timeout must still fire from spawn.
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
sleep 30
`)
    const start = Date.now()
    const res = await dispatchAtlas({ hook_event_name: "tool.execute.before", tool_name: "bash" })
    expect(res.decision).toBe("allow")
    expect(shouldDenyToolExecution(res)).toBe(false)
    expect(Date.now() - start).toBeLessThan(5_000)
  })

  test("child exits before reading stdin", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
exit 0
`)
    const res = await dispatchAtlas({ hook_event_name: "session.created", session_id: "s1" })
    expect(res.decision).toBe("allow")
    expect(shouldDenyToolExecution(res)).toBe(false)
  })

  test("stdin write failure when child dies immediately", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
kill -9 $$
`)
    const res = await dispatchAtlas({ hook_event_name: "tool.execute.before", tool_name: "bash" })
    expect(res.decision).toBe("allow")
    expect(shouldDenyToolExecution(res)).toBe(false)
  })

  test("process termination mid-flight", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
kill -9 $$
`)
    const res = await dispatchAtlas({ hook_event_name: "session.created", session_id: "s1" })
    expect(res.decision).toBe("allow")
  })

  test("non-zero exit with apparent deny must fail-open", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"denied","decision":"deny","message":"blocked by atlas hooks"}'
exit 1
`)
    const res = await dispatchAtlas({
      hook_event_name: "tool.execute.before",
      tool_name: "bash",
    })
    expect(res.decision).toBe("allow")
    expect(shouldDenyToolExecution(res)).toBe(false)
  })

  test("excessive stdout fail-open", async () => {
    const n = MAX_STDOUT_BYTES + 64
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
dd if=/dev/zero bs=1 count=${n} status=none
exit 0
`)
    const res = await dispatchAtlas({ hook_event_name: "session.created", session_id: "s1" })
    expect(res.decision).toBe("allow")
  })

  test("excessive stderr fail-open", async () => {
    const n = MAX_STDERR_BYTES + 64
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
dd if=/dev/zero bs=1 count=${n} status=none 1>&2
printf '%s\\n' '{"schema_version":1,"status":"denied","decision":"deny"}'
exit 0
`)
    const res = await dispatchAtlas({
      hook_event_name: "tool.execute.before",
      tool_name: "bash",
    })
    expect(res.decision).toBe("allow")
    expect(shouldDenyToolExecution(res)).toBe(false)
  })

  test("partial output fail-open", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf '%s' '{"schema_version":1,"status":"denied","decision":'
exit 0
`)
    const res = await dispatchAtlas({
      hook_event_name: "tool.execute.before",
      tool_name: "bash",
    })
    expect(res.decision).toBe("allow")
  })

  test("oversized UTF-8 payload rejects without spawning", async () => {
    const marker = join(sandbox, "spawned")
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
echo spawned > "${marker}"
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"denied","decision":"deny"}'
`)
    const pad = "é".repeat(Math.ceil((MAX_BYTES + 64) / 2))
    const payload = { hook_event_name: "session.prompt", session_id: pad }
    expect(utf8ByteLength(JSON.stringify(payload))).toBeGreaterThan(MAX_BYTES)
    const res = await dispatchAtlas(payload)
    expect(res.decision).toBe("allow")
    expect(existsSync(marker)).toBe(false)
  })

  test("repeated invocations remain stable", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"success","decision":"allow"}'
exit 0
`)
    for (let i = 0; i < 5; i++) {
      const res = await dispatchAtlas({ hook_event_name: "session.created", session_id: `s${i}` })
      expect(res.decision).toBe("allow")
    }
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"denied","decision":"deny","message":"blocked by atlas hooks"}'
exit 0
`)
    const deny = await dispatchAtlas({ hook_event_name: "tool.execute.before", tool_name: "bash" })
    expect(shouldDenyToolExecution(deny)).toBe(true)
  })
})

/** Fixtures mirror OpenCode 2.0.20 contracts (binary + SDK types.gen.ts). */
const REAL_SESSION_ID = "ses_14f5inventoryQA3158da2a"

describe("plugin discovery contract", () => {
  test("default export has id and setup (OpenCode V2 load shape)", () => {
    const def = mod.default
    expect(def).toBeTruthy()
    expect(def.id).toBe("atlas.hooks")
    expect(typeof def.setup).toBe("function")
    expect(Object.prototype.hasOwnProperty.call(def, "effect")).toBe(false)
  })

  test("setup registers subscribe + hooks and cleanup aborts", async () => {
    const hooks: string[] = []
    let subscribed = false
    let aborted = false
    const events: unknown[] = []

    const cleanup = await mod.default.setup({
      location: { directory: sandbox },
      event: {
        subscribe: ({ signal }: { signal?: AbortSignal }) => {
          subscribed = true
          signal?.addEventListener("abort", () => {
            aborted = true
          })
          return {
            async *[Symbol.asyncIterator]() {
              for (const ev of events) yield ev
            },
          }
        },
      },
      session: {
        hook: async (name: string) => {
          hooks.push(`session:${name}`)
          return { dispose: async () => {} }
        },
      },
      tool: {
        hook: async (name: string) => {
          hooks.push(`tool:${name}`)
          return { dispose: async () => {} }
        },
      },
    })

    expect(subscribed).toBe(true)
    expect(hooks).toContain("session:prompt")
    expect(hooks).toContain("session:compaction")
    expect(hooks).toContain("tool:execute.before")
    expect(hooks).toContain("tool:execute.after")
    expect(typeof cleanup).toBe("function")
    cleanup?.()
    expect(aborted).toBe(true)
  })
})

describe("sessionIDFrom / OpenCode 2.0.20 payloads", () => {
  test("V2 live bus data.sessionID (OpenCode 2.0.20 binary consumers)", () => {
    const event = {
      id: "evt_14f533005001GXL38sAVtifuxl",
      type: "session.created",
      data: {
        sessionID: REAL_SESSION_ID,
        info: { id: REAL_SESSION_ID, title: "QA" },
      },
    }
    expect(sessionIDFrom(event)).toBe(REAL_SESSION_ID)
    // Must not prefer envelope evt_* id over data.sessionID
    expect(sessionIDFrom(event)).not.toBe(event.id)
  })

  test("SDK EventSessionCreated properties.sessionID", () => {
    expect(
      sessionIDFrom({
        id: "evt_sdk",
        type: "session.created",
        properties: { sessionID: REAL_SESSION_ID, info: { id: REAL_SESSION_ID } },
      }),
    ).toBe(REAL_SESSION_ID)
  })

  test("SyncEventSessionCreated data.sessionID", () => {
    expect(
      sessionIDFrom({
        type: "sync",
        name: "session.created.1",
        id: "sync_1",
        seq: 1,
        aggregateID: "sessionID",
        data: { sessionID: REAL_SESSION_ID, info: { id: REAL_SESSION_ID } },
      }),
    ).toBe(REAL_SESSION_ID)
  })

  test("wrapped payload envelope", () => {
    expect(
      sessionIDFrom({
        directory: "/tmp/project",
        payload: {
          type: "session.created",
          data: { sessionID: REAL_SESSION_ID },
        },
      }),
    ).toBe(REAL_SESSION_ID)
  })

  test("missing / empty / malformed / path rejected", () => {
    expect(sessionIDFrom(null)).toBe("")
    expect(sessionIDFrom(undefined)).toBe("")
    expect(sessionIDFrom("ses_x")).toBe("")
    expect(sessionIDFrom({ type: "session.created", data: {} })).toBe("")
    expect(sessionIDFrom({ type: "session.created", data: { sessionID: "  " } })).toBe("")
    expect(sessionIDFrom({ sessionID: "/tmp/atlas-project" })).toBe("")
    expect(sessionIDFrom({ id: "C:\\Users\\x" })).toBe("")
    expect(isValidSessionID("")).toBe(false)
    expect(isValidSessionID("/abs/path")).toBe(false)
    expect(isValidSessionID(REAL_SESSION_ID)).toBe(true)
  })

  test("hook shapes still resolve sessionID", () => {
    expect(sessionIDFrom({ sessionID: "s_prompt" })).toBe("s_prompt")
    expect(sessionIDFrom({ session_id: "s_snake" })).toBe("s_snake")
    expect(sessionIDFrom({ sessionId: "s_camel" })).toBe("s_camel")
  })
})

describe("isSessionCreatedEvent / handleSessionCreatedEvent", () => {
  test("accepts confirmed session.created variants", () => {
    expect(isSessionCreatedEvent({ type: "session.created", data: { sessionID: "s1" } })).toBe(true)
    expect(
      isSessionCreatedEvent({
        type: "session.created",
        properties: { sessionID: "s1" },
      }),
    ).toBe(true)
    expect(
      isSessionCreatedEvent({
        type: "sync",
        name: "session.created.1",
        data: { sessionID: "s1" },
      }),
    ).toBe(true)
    expect(
      isSessionCreatedEvent({
        payload: { type: "session.created", data: { sessionID: "s1" } },
      }),
    ).toBe(true)
  })

  test("rejects prompts, tools, and unrelated session events", () => {
    expect(isSessionCreatedEvent({ type: "session.prompt", sessionID: "s1" })).toBe(false)
    expect(isSessionCreatedEvent({ type: "session.updated", data: { sessionID: "s1" } })).toBe(false)
    expect(isSessionCreatedEvent({ type: "session.execution.started", data: { sessionID: "s1" } })).toBe(
      false,
    )
    expect(isSessionCreatedEvent({ type: "tool.execute.before", sessionID: "s1" })).toBe(false)
    expect(isSessionCreatedEvent({ type: "provider.updated", data: {} })).toBe(false)
    expect(isSessionCreatedEvent({ type: "sync", name: "session.updated.1", data: { sessionID: "s1" } })).toBe(
      false,
    )
  })

  test("missing session id rejects; duplicates rejected once seen", () => {
    expect(handleSessionCreatedEvent({ type: "session.created", data: {} }, undefined, sandbox)).toEqual({
      kind: "reject",
      reason: "missing_session_id",
    })
    const seen = new Set<string>()
    const owned = {
      type: "session.created",
      data: { sessionID: "s1", location: { directory: sandbox } },
    }
    const first = handleSessionCreatedEvent(owned, seen, sandbox)
    expect(first.kind).toBe("dispatch")
    if (first.kind === "dispatch") {
      expect(first.session_id).toBe("s1")
      expect(first.session_directory).toBe(realpathSync(sandbox))
    }
    expect(handleSessionCreatedEvent(owned, seen, sandbox)).toEqual({
      kind: "reject",
      reason: "duplicate",
    })
    expect(handleSessionCreatedEvent({ type: "session.prompt", sessionID: "s1" }, seen, sandbox)).toEqual({
      kind: "reject",
      reason: "not_session_created",
    })
  })

  test("foreign session is not deduped before a later owned event", () => {
    const seen = new Set<string>()
    const other = mkdtempSync(join(tmpdir(), "atlas-hooks-other-"))
    try {
      expect(
        handleSessionCreatedEvent(
          { type: "session.created", data: { sessionID: "s1", location: { directory: other } } },
          seen,
          sandbox,
        ),
      ).toEqual({ kind: "reject", reason: "foreign_session_directory" })
      expect(seen.has("s1")).toBe(false)
      const owned = handleSessionCreatedEvent(
        { type: "session.created", data: { sessionID: "s1", location: { directory: sandbox } } },
        seen,
        sandbox,
      )
      expect(owned.kind).toBe("dispatch")
    } finally {
      rmSync(other, { recursive: true, force: true })
    }
  })

  test("missing and contradictory directories do not dispatch", () => {
    const other = mkdtempSync(join(tmpdir(), "atlas-hooks-other-"))
    try {
      expect(
        handleSessionCreatedEvent(
          { type: "session.created", data: { sessionID: "s1" } },
          undefined,
          sandbox,
        ).reason,
      ).toBe("missing_session_directory")
      const both = handleSessionCreatedEvent(
        {
          type: "session.created",
          data: {
            sessionID: "s1",
            location: { directory: sandbox },
            info: { directory: other },
          },
        },
        undefined,
        sandbox,
      )
      expect(both).toEqual({ kind: "reject", reason: "contradictory_session_directory" })
    } finally {
      rmSync(other, { recursive: true, force: true })
    }
  })
})

describe("processSubscribeEvent integration", () => {
  test("data-shaped session.created dispatches once; prompt does not", async () => {
    const calls: Record<string, string>[] = []
    const seen = new Set<string>()
    const dispatch = async (payload: Record<string, string>) => {
      calls.push(payload)
      return allowDecision()
    }

    const owned = {
      id: "evt_1",
      type: "session.created",
      data: { sessionID: REAL_SESSION_ID, location: { directory: sandbox } },
    }
    const first = await processSubscribeEvent(owned, seen, dispatch, sandbox)
    expect(first.kind).toBe("dispatch")
    expect(calls).toEqual([
      {
        hook_event_name: "session.created",
        session_id: REAL_SESSION_ID,
        session_directory: realpathSync(sandbox),
      },
    ])

    const dup = await processSubscribeEvent(
      {
        id: "evt_2",
        type: "session.created",
        data: { sessionID: REAL_SESSION_ID, location: { directory: sandbox } },
      },
      seen,
      dispatch,
      sandbox,
    )
    expect(dup.kind).toBe("reject")
    expect(calls).toHaveLength(1)

    const prompt = await processSubscribeEvent(
      { type: "session.prompt", sessionID: REAL_SESSION_ID },
      seen,
      dispatch,
    )
    expect(prompt.kind).toBe("reject")
    expect(calls).toHaveLength(1)
  })

  test("end-to-end: real-shaped event → mock atlas spawn", async () => {
    writeMockAtlas(`#!/usr/bin/env bash
set -euo pipefail
cat >/dev/null
printf '%s\\n' '{"schema_version":1,"status":"success","decision":"allow"}'
exit 0
`)
    const seen = new Set<string>()
    const res = await processSubscribeEvent(
      {
        id: "evt_live",
        type: "session.created",
        data: { sessionID: REAL_SESSION_ID, location: { directory: sandbox } },
      },
      seen,
      undefined,
      sandbox,
    )
    expect(res.kind).toBe("dispatch")
    if (res.kind === "dispatch") {
      expect(res.session_id).toBe(REAL_SESSION_ID)
      expect(res.session_directory).toBe(realpathSync(sandbox))
    }
  })
})

describe("QA diagnostics trace", () => {
  test("trace disabled → no filesystem writes", () => {
    delete process.env[TRACE_ENV]
    process.env.ATLAS_HOME = join(sandbox, "atlas-home")
    traceHooks({ phase: "setup_enter" })
    expect(existsSync(join(sandbox, "atlas-home", "diagnostics"))).toBe(false)
    expect(existsSync(join(sandbox, "atlas-home", "diagnostics", "opencode-hooks.jsonl"))).toBe(false)
  })

  test("trace enabled + explicit ATLAS_HOME → isolated write with restrictive perms", () => {
    process.env[TRACE_ENV] = "1"
    const home = join(sandbox, "atlas-home")
    process.env.ATLAS_HOME = home
    expect(resolveTraceHome()).toBe(home)

    traceHooks({ phase: "setup_enter" })
    traceHooks({
      phase: "event_received",
      event_type: "session.created",
      has_session_id: true,
    })
    const dir = join(home, "diagnostics")
    const logPath = join(dir, "opencode-hooks.jsonl")
    expect(existsSync(logPath)).toBe(true)
    const body = readFileSync(logPath, "utf8")
    expect(body).toContain('"phase":"setup_enter"')
    expect(body).toContain('"event_type":"session.created"')
    expect(body).not.toContain("SECRET")
    // Restrictive modes (ignore high bits / umask variance on sticky platforms).
    expect(statSync(dir).mode & 0o777).toBe(0o700)
    expect(statSync(logPath).mode & 0o777).toBe(0o600)
  })

  test("trace enabled + missing ATLAS_HOME → no writes (never ~/.atlas)", () => {
    process.env[TRACE_ENV] = "1"
    delete process.env.ATLAS_HOME
    expect(resolveTraceHome()).toBeUndefined()

    const realDiag = join(homedir(), ".atlas", "diagnostics", "opencode-hooks.jsonl")
    const before = existsSync(realDiag) ? readFileSync(realDiag, "utf8") : null
    const beforeMtime = existsSync(realDiag) ? statSync(realDiag).mtimeMs : null

    traceHooks({ phase: "setup_enter" })
    traceHooks({ phase: "event_received", event_type: "session.created", has_session_id: true })

    // Must not create sandbox diagnostics either (no invented home).
    expect(existsSync(join(sandbox, "atlas-home", "diagnostics"))).toBe(false)
    // Must not append to real Atlas Home.
    if (before === null) {
      expect(existsSync(realDiag)).toBe(false)
    } else {
      expect(readFileSync(realDiag, "utf8")).toBe(before)
      expect(statSync(realDiag).mtimeMs).toBe(beforeMtime)
    }
  })

  test("invalid diagnostic destination → fail-open (no throw, no write)", () => {
    process.env[TRACE_ENV] = "1"
    const asFile = join(sandbox, "not-a-home")
    writeFileSync(asFile, "x", { encoding: "utf8" })
    process.env.ATLAS_HOME = asFile
    expect(resolveTraceHome()).toBeUndefined()
    expect(() => traceHooks({ phase: "setup_enter" })).not.toThrow()
    expect(existsSync(join(asFile, "diagnostics"))).toBe(false)

    process.env.ATLAS_HOME = `${sandbox}/home-with\0null`
    expect(resolveTraceHome()).toBeUndefined()
    expect(() => traceHooks({ phase: "dispatch_failed", reason: "spawn" })).not.toThrow()
  })
})

describe("cross-project isolation", () => {
  test("two subscribers: only the owning instance dispatches", async () => {
    const other = mkdtempSync(join(tmpdir(), "atlas-hooks-b-"))
    const calls: string[] = []
    const dispatch = async (payload: Record<string, string>) => {
      calls.push(payload.session_directory ?? "")
      return allowDecision()
    }
    const event = {
      type: "session.created",
      data: { sessionID: "ses_shared", location: { directory: sandbox } },
    }
    try {
      const owned = await processSubscribeEvent(event, new Set(), dispatch, sandbox)
      const foreign = await processSubscribeEvent(event, new Set(), dispatch, other)
      expect(owned.kind).toBe("dispatch")
      expect(foreign).toEqual({ kind: "reject", reason: "foreign_session_directory" })
      expect(calls).toEqual([realpathSync(sandbox)])
    } finally {
      rmSync(other, { recursive: true, force: true })
    }
  })

  test("symlink to the instance directory is the same project", () => {
    const link = join(sandbox, "link-to-root")
    symlinkSync(sandbox, link)
    const handled = handleSessionCreatedEvent(
      { type: "session.created", data: { sessionID: "s-link", location: { directory: link } } },
      undefined,
      sandbox,
    )
    expect(handled.kind).toBe("dispatch")
  })

  test("tool.execute.after uses session.get and does not dispatch a foreign session", async () => {
    const other = mkdtempSync(join(tmpdir(), "atlas-hooks-tool-b-"))
    const calls: Record<string, string>[] = []
    const dispatch = async (payload: Record<string, string>) => {
      calls.push(payload)
      return allowDecision()
    }
    const event = { tool: "write", sessionID: "ses_tool", callID: "c1", status: "completed" }
    try {
      const foreign = await dispatchOwnedHook(
        { hook_event_name: "tool.execute.after", session_id: "ses_tool", tool_name: "write", status: "success" },
        event,
        sandbox,
        async () => ({ location: { directory: other } }),
        dispatch,
      )
      expect(foreign.decision).toBe("allow")
      expect(calls).toHaveLength(0)

      const owned = await dispatchOwnedHook(
        { hook_event_name: "tool.execute.after", session_id: "ses_tool", tool_name: "write", status: "success" },
        event,
        sandbox,
        async () => ({ location: { directory: sandbox } }),
        dispatch,
      )
      expect(owned.decision).toBe("allow")
      expect(calls).toEqual([
        {
          hook_event_name: "tool.execute.after",
          session_id: "ses_tool",
          tool_name: "write",
          status: "success",
          session_directory: realpathSync(sandbox),
        },
      ])
      expect(directoryFromSessionRecord({ location: { directory: sandbox } })).toBe(sandbox)
      expect(directoryFromSessionRecord({ directory: sandbox })).toBeUndefined()
    } finally {
      rmSync(other, { recursive: true, force: true })
    }
  })

  test("tool hook without session.get does not invent a directory", async () => {
    const calls: string[] = []
    const res = await dispatchOwnedHook(
      { hook_event_name: "tool.execute.before", session_id: "ses_x", tool_name: "write" },
      { tool: "write", sessionID: "ses_x" },
      sandbox,
      undefined,
      async () => {
        calls.push("spawn")
        return allowDecision()
      },
    )
    expect(res.decision).toBe("allow")
    expect(calls).toHaveLength(0)
  })
})
