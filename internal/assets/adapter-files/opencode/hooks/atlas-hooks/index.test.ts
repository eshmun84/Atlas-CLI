/**
 * Bun runtime tests for the Atlas OpenCode hooks plugin.
 * Exercises real subprocess behavior via PATH-shadowed mock `atlas` binaries.
 */
import { afterEach, beforeEach, describe, expect, test } from "bun:test"
import { chmodSync, mkdirSync, mkdtempSync, rmSync, writeFileSync, existsSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"

const mod = await import("./index.ts")
const {
  MAX_BYTES,
  MAX_STDERR_BYTES,
  MAX_STDOUT_BYTES,
  allowDecision,
  dispatchAtlas,
  parseAtlasResponse,
  shouldDenyToolExecution,
  utf8ByteLength,
} = mod

const originalPath = process.env.PATH ?? ""
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
  mod.setTestTimeoutOverrideMs(undefined)
})

afterEach(() => {
  process.env.PATH = originalPath
  mod.setTestTimeoutOverrideMs(undefined)
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

describe("dispatchAtlas subprocess", () => {
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
