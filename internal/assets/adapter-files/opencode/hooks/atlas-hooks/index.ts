import { appendFileSync, mkdirSync } from "node:fs"
import { homedir } from "node:os"
import { join } from "node:path"

/**
 * Atlas-owned OpenCode V2 hooks plugin.
 * Do not edit — managed by Atlas CLI projection.
 *
 * Invokes: atlas hooks opencode --json
 * Never forwards prompts, tool arguments, file contents, or transcripts.
 *
 * Subprocess timeout cancels the local child only; it is not a guarantee
 * against all external side effects Atlas may have started.
 *
 * OpenCode V2.0.20 discovers project-local plugins under .opencode/plugins/
 * but does not resolve npm package "@opencode/plugin" for those entrypoints.
 * Plugin.define below is therefore a local identity matching the V2 default
 * export shape { id, setup } validated by OpenCode at load time — same
 * surface as Plugin.define from @opencode/plugin, without a package download.
 *
 * Event contract (OpenCode 2.0.20):
 * - Plugin stream (V2EventEncoded / live bus): type "session.created" with
 *   body under `data.sessionID` (confirmed by installed binary consumers and
 *   server INFO logs that emit event.data, not event.properties).
 * - SDK HTTP EventSessionCreated may still use `properties.sessionID`.
 * - Sync bus SyncEventSessionCreated: type "sync", name "session.created.1",
 *   body under `data.sessionID`.
 */

export const DENY_MESSAGE = "blocked by atlas hooks"

/** QA-only: set ATLAS_OPENCODE_HOOKS_TRACE=1 to append diagnostic lines. */
export const TRACE_ENV = "ATLAS_OPENCODE_HOOKS_TRACE"

type PluginDefinition = {
  id: string
  setup: (ctx: OpenCodePluginContext) => Promise<(() => void) | void> | (() => void) | void
}

/** Identity helper equivalent to @opencode/plugin Plugin.define for local load. */
const Plugin = {
  define(def: PluginDefinition): PluginDefinition {
    return def
  },
}

/** Minimal structural typing for OpenCode V2 plugin context (no package import). */
type OpenCodePluginContext = {
  event: {
    subscribe: (opts: { signal?: AbortSignal }) => AsyncIterable<unknown>
  }
  session: {
    hook: (name: string, cb: (event: unknown) => Promise<void> | void) => Promise<unknown>
  }
  tool: {
    hook: (name: string, cb: (event: unknown) => Promise<void> | void) => Promise<unknown>
  }
}
/** Matches hooksengine.MaxRequestBytes / opencode.MaxNativeBytes. */
export const MAX_BYTES = 64 * 1024
export const MAX_STDOUT_BYTES = 64 * 1024
export const MAX_STDERR_BYTES = 16 * 1024
/** Wall-clock bound for the Atlas child (above DefaultActionTimeout). */
export const SUBPROCESS_TIMEOUT_MS = 10_000

/**
 * Test-only timeout override. Production always uses SUBPROCESS_TIMEOUT_MS.
 * Not a public contract — cleared outside automated tests.
 */
let testTimeoutOverrideMs: number | undefined

export function setTestTimeoutOverrideMs(ms: number | undefined): void {
  if (ms === undefined) {
    testTimeoutOverrideMs = undefined
    return
  }
  if (typeof ms === "number" && Number.isFinite(ms) && ms >= 50) {
    testTimeoutOverrideMs = ms
  }
}

function subprocessTimeoutMs(): number {
  if (
    typeof testTimeoutOverrideMs === "number" &&
    Number.isFinite(testTimeoutOverrideMs) &&
    testTimeoutOverrideMs >= 50
  ) {
    return testTimeoutOverrideMs
  }
  return SUBPROCESS_TIMEOUT_MS
}

/** Status values emitted by atlas hooks opencode --json (hooksengine + TranslateResponse). */
export const KNOWN_STATUSES = [
  "success",
  "aborted",
  "denied",
  "hooks_disabled",
  "unsupported_event",
  "invalid_request",
  "invalid_adapter",
  "invalid_event",
  "project_mismatch",
  "not_initialized",
  "unsafe_filesystem",
  "invalid_usage",
  "cancelled",
  "internal_error",
] as const

type KnownStatus = (typeof KNOWN_STATUSES)[number]

export type AtlasDecision = {
  schema_version: number
  status: string
  decision: "allow" | "deny"
  message?: string
}

const ALLOW: AtlasDecision = {
  schema_version: 1,
  status: "success",
  decision: "allow",
}

const knownStatusSet = new Set<string>(KNOWN_STATUSES)

export function allowDecision(): AtlasDecision {
  return { ...ALLOW }
}

export function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).byteLength
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value)
}

/**
 * Strict runtime validation of Atlas CLI stdout.
 * Returns null when the payload must not be trusted (caller fail-opens).
 */
export function parseAtlasResponse(text: string): AtlasDecision | null {
  const trimmed = text.trim()
  if (!trimmed) return null

  let value: unknown
  try {
    value = JSON.parse(trimmed)
  } catch {
    return null
  }

  // JSON.parse accepts only one value; reject non-objects and arrays/null.
  if (!isPlainObject(value)) return null

  if (value.schema_version !== 1) return null
  if (typeof value.status !== "string" || !knownStatusSet.has(value.status)) return null
  if (value.decision !== "allow" && value.decision !== "deny") return null

  // Strict status/decision pairing (matches opencode.TranslateResponse):
  //   denied + deny  → valid explicit block for tool.execute.before
  //   denied + allow → reject (contradictory; Go never emits this)
  //   other  + deny  → reject
  if (value.decision === "deny") {
    if (value.status !== "denied") return null
  } else if (value.status === "denied") {
    return null
  }

  if (value.message !== undefined && typeof value.message !== "string") return null

  const out: AtlasDecision = {
    schema_version: 1,
    status: value.status as KnownStatus,
    decision: value.decision,
  }
  if (typeof value.message === "string") {
    out.message = value.message
  }
  return out
}

/** tool.execute.before blocks only on an explicit, valid denial pair. */
export function shouldDenyToolExecution(res: AtlasDecision): boolean {
  return res.decision === "deny" && res.status === "denied"
}

/**
 * True when value is a non-empty session identifier string that is not a path.
 * Rejects empty/whitespace and filesystem-looking values (never invent IDs).
 */
export function isValidSessionID(value: unknown): value is string {
  if (typeof value !== "string") return false
  const t = value.trim()
  if (!t) return false
  // Paths must never be used as session identifiers.
  if (t.includes("/") || t.includes("\\")) return false
  if (t === "." || t === "..") return false
  return true
}

/**
 * Extract a session ID from OpenCode event / hook payloads.
 *
 * Confirmed OpenCode 2.0.20 shapes (in priority order for nested walk):
 * - data.sessionID          (live V2 bus / binary consumers)
 * - properties.sessionID    (SDK EventSessionCreated)
 * - payload…                (wrapped GlobalEvent-like envelopes)
 * - sessionID / sessionId / session_id at the current object
 * - bare `id` only AFTER nested containers (event envelopes also have `id`)
 */
export function sessionIDFrom(value: unknown, depth = 0): string {
  if (!value || typeof value !== "object" || depth > 5) return ""
  const v = value as Record<string, unknown>

  for (const key of ["sessionID", "sessionId", "session_id"]) {
    if (isValidSessionID(v[key])) return (v[key] as string).trim()
  }

  // Nested containers before bare `id` — event envelopes carry evt_* in `id`.
  for (const nest of ["data", "properties", "payload"]) {
    const nested = v[nest]
    if (nested && typeof nested === "object") {
      const sid = sessionIDFrom(nested, depth + 1)
      if (sid) return sid
    }
  }

  if (isValidSessionID(v.id)) return (v.id as string).trim()
  return ""
}

/**
 * True for native session-created events on confirmed OpenCode 2.0.20 buses.
 * Does not treat prompts, tool hooks, or unrelated session.* events as new sessions.
 */
export function isSessionCreatedEvent(event: unknown): boolean {
  if (!event || typeof event !== "object") return false
  const e = event as Record<string, unknown>

  if (e.type === "session.created") return true

  // SDK SyncEventSessionCreated: { type: "sync", name: "session.created.1", data }
  if (e.type === "sync" && e.name === "session.created.1") return true

  // Wrapped envelope: { directory?, payload: Event }
  if (e.payload && typeof e.payload === "object") {
    return isSessionCreatedEvent(e.payload)
  }
  return false
}

export type SessionCreatedHandle =
  | { kind: "dispatch"; session_id: string }
  | { kind: "reject"; reason: "not_session_created" | "missing_session_id" | "duplicate" }

/**
 * Normalize a subscribe-loop event into a session.created dispatch decision.
 * Dedupes by session_id within one plugin instance (lease absorbs cross-process).
 */
export function handleSessionCreatedEvent(
  event: unknown,
  seenSessionIDs?: Set<string>,
): SessionCreatedHandle {
  if (!isSessionCreatedEvent(event)) {
    return { kind: "reject", reason: "not_session_created" }
  }
  const sid = sessionIDFrom(event)
  if (!sid) {
    return { kind: "reject", reason: "missing_session_id" }
  }
  if (seenSessionIDs) {
    if (seenSessionIDs.has(sid)) {
      return { kind: "reject", reason: "duplicate" }
    }
    seenSessionIDs.add(sid)
  }
  return { kind: "dispatch", session_id: sid }
}

type TraceFields = {
  phase: string
  event_type?: string
  has_session_id?: boolean
  reason?: string
  status?: string
  decision?: string
}

/**
 * Minimal QA diagnostics. Disabled unless ATLAS_OPENCODE_HOOKS_TRACE=1.
 * Never records prompts, tool args, transcripts, paths as secrets, or code.
 */
export function traceHooks(fields: TraceFields): void {
  if (process.env[TRACE_ENV] !== "1") return
  try {
    const home =
      typeof process.env.ATLAS_HOME === "string" && process.env.ATLAS_HOME.trim()
        ? process.env.ATLAS_HOME.trim()
        : join(homedir(), ".atlas")
    if (!home) return
    const dir = join(home, "diagnostics")
    const line = JSON.stringify({
      ts: new Date().toISOString(),
      component: "opencode-hooks",
      phase: fields.phase,
      ...(fields.event_type !== undefined ? { event_type: fields.event_type } : {}),
      ...(fields.has_session_id !== undefined ? { has_session_id: fields.has_session_id } : {}),
      ...(fields.reason !== undefined ? { reason: fields.reason } : {}),
      ...(fields.status !== undefined ? { status: fields.status } : {}),
      ...(fields.decision !== undefined ? { decision: fields.decision } : {}),
    })
    mkdirSync(dir, { recursive: true })
    appendFileSync(join(dir, "opencode-hooks.jsonl"), line + "\n", { encoding: "utf8" })
  } catch {
    // Diagnostics must never affect the agent session.
  }
}

function eventTypeOf(event: unknown): string {
  if (!event || typeof event !== "object") return ""
  const e = event as Record<string, unknown>
  if (typeof e.type === "string") {
    if (e.type === "sync" && typeof e.name === "string") return `${e.type}:${e.name}`
    return e.type
  }
  return ""
}

async function readBounded(
  stream: ReadableStream<Uint8Array> | null,
  maxBytes: number,
): Promise<{ bytes: Uint8Array; truncated: boolean }> {
  if (!stream) return { bytes: new Uint8Array(), truncated: false }
  const reader = stream.getReader()
  const chunks: Uint8Array[] = []
  let total = 0
  let truncated = false
  try {
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      if (!value || value.byteLength === 0) continue
      if (total + value.byteLength > maxBytes) {
        const take = maxBytes - total
        if (take > 0) chunks.push(value.subarray(0, take))
        total = maxBytes
        truncated = true
        try {
          await reader.cancel()
        } catch {
          // ignore cancel errors
        }
        break
      }
      chunks.push(value)
      total += value.byteLength
    }
  } catch {
    truncated = true
  } finally {
    try {
      reader.releaseLock()
    } catch {
      // already released
    }
  }

  const bytes = new Uint8Array(total)
  let offset = 0
  for (const chunk of chunks) {
    bytes.set(chunk, offset)
    offset += chunk.byteLength
  }
  return { bytes, truncated }
}

function decodeUTF8(bytes: Uint8Array): string | null {
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes)
  } catch {
    return null
  }
}

function killProc(proc: { kill: () => void } | undefined): void {
  if (!proc) return
  try {
    proc.kill()
  } catch {
    // already exited
  }
}

type ChildOutcome =
  | {
      kind: "done"
      stdout: { bytes: Uint8Array; truncated: boolean }
      stderr: { bytes: Uint8Array; truncated: boolean }
      code: number
    }
  | { kind: "fail" }
  | { kind: "timeout" }

/**
 * Invoke Atlas CLI with bounded IO and wall-clock timeout.
 * The timeout starts immediately after spawn and covers stdin write,
 * stdout/stderr reads, process exit, and cleanup. Fail-open on any
 * transport/validation failure. Valid deny preserved.
 */
export async function dispatchAtlas(payload: Record<string, string>): Promise<AtlasDecision> {
  const hookName = payload.hook_event_name ?? ""
  let body: string
  try {
    body = JSON.stringify(payload)
  } catch {
    traceHooks({ phase: "dispatch_failed", reason: "stringify", event_type: hookName })
    return allowDecision()
  }
  if (utf8ByteLength(body) > MAX_BYTES) {
    traceHooks({ phase: "dispatch_failed", reason: "oversized", event_type: hookName })
    return allowDecision()
  }

  let proc: ReturnType<typeof Bun.spawn> | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    proc = Bun.spawn(["atlas", "hooks", "opencode", "--json"], {
      stdin: "pipe",
      stdout: "pipe",
      stderr: "pipe",
      env: { ...process.env },
    })
  } catch {
    traceHooks({ phase: "dispatch_failed", reason: "spawn", event_type: hookName })
    return allowDecision()
  }

  // Bound the entire child lifecycle from the moment of spawn.
  const timeoutPromise = new Promise<ChildOutcome>((resolve) => {
    timer = setTimeout(() => {
      killProc(proc)
      resolve({ kind: "timeout" })
    }, subprocessTimeoutMs())
  })

  const workPromise: Promise<ChildOutcome> = (async () => {
    try {
      const stdin = proc!.stdin
      if (!stdin) {
        killProc(proc)
        return { kind: "fail" }
      }
      try {
        stdin.write(body)
        stdin.end()
      } catch {
        // Child exited early, pipe closed, or write failed.
        killProc(proc)
        return { kind: "fail" }
      }

      const [stdout, stderr, code] = await Promise.all([
        readBounded(proc!.stdout as ReadableStream<Uint8Array> | null, MAX_STDOUT_BYTES),
        readBounded(proc!.stderr as ReadableStream<Uint8Array> | null, MAX_STDERR_BYTES),
        proc!.exited,
      ])
      return { kind: "done", stdout, stderr, code }
    } catch {
      killProc(proc)
      return { kind: "fail" }
    }
  })()

  try {
    const outcome = await Promise.race([workPromise, timeoutPromise])

    if (outcome.kind === "timeout") {
      // Prevent an unresolved workPromise from keeping the hook event alive.
      try {
        await Promise.race([workPromise, new Promise((r) => setTimeout(r, 250))])
      } catch {
        // ignore post-kill drain errors
      }
      traceHooks({ phase: "dispatch_failed", reason: "timeout", event_type: hookName })
      return allowDecision()
    }

    if (outcome.kind === "fail") {
      traceHooks({ phase: "dispatch_failed", reason: "io", event_type: hookName })
      return allowDecision()
    }

    if (outcome.stdout.truncated || outcome.stderr.truncated) {
      killProc(proc)
      traceHooks({ phase: "dispatch_failed", reason: "truncated", event_type: hookName })
      return allowDecision()
    }

    // Never trust stdout when the process failed.
    if (outcome.code !== 0) {
      traceHooks({ phase: "dispatch_failed", reason: "exit_nonzero", event_type: hookName })
      return allowDecision()
    }

    const text = decodeUTF8(outcome.stdout.bytes)
    if (text === null) {
      traceHooks({ phase: "dispatch_failed", reason: "utf8", event_type: hookName })
      return allowDecision()
    }

    const parsed = parseAtlasResponse(text)
    if (!parsed) {
      traceHooks({ phase: "dispatch_failed", reason: "invalid_json", event_type: hookName })
      return allowDecision()
    }
    traceHooks({
      phase: "dispatch_completed",
      event_type: hookName,
      status: parsed.status,
      decision: parsed.decision,
    })
    return parsed
  } catch {
    killProc(proc)
    traceHooks({ phase: "dispatch_failed", reason: "exception", event_type: hookName })
    return allowDecision()
  } finally {
    if (timer !== undefined) clearTimeout(timer)
  }
}

/**
 * Process one subscribe-loop event: normalize, dedupe, and optionally dispatch.
 * Exported for contract tests (fixtures mirror OpenCode 2.0.20 payloads).
 */
export async function processSubscribeEvent(
  event: unknown,
  seenSessionIDs: Set<string>,
  dispatch: (payload: Record<string, string>) => Promise<AtlasDecision> = dispatchAtlas,
): Promise<SessionCreatedHandle> {
  const eventType = eventTypeOf(event)
  if (eventType) {
    traceHooks({ phase: "event_received", event_type: eventType })
  }

  const handled = handleSessionCreatedEvent(event, seenSessionIDs)
  if (handled.kind === "reject") {
    if (handled.reason === "missing_session_id") {
      traceHooks({
        phase: "event_rejected",
        event_type: eventType || "session.created",
        has_session_id: false,
        reason: handled.reason,
      })
    } else if (handled.reason === "duplicate") {
      traceHooks({
        phase: "event_rejected",
        event_type: eventType || "session.created",
        has_session_id: true,
        reason: handled.reason,
      })
    }
    // not_session_created: silent at default; type already traced as event_received
    return handled
  }

  traceHooks({
    phase: "dispatch_attempted",
    event_type: "session.created",
    has_session_id: true,
  })
  await dispatch({
    hook_event_name: "session.created",
    session_id: handled.session_id,
  })
  return handled
}

export default Plugin.define({
  id: "atlas.hooks",
  async setup(ctx) {
    traceHooks({ phase: "setup_enter" })
    const controller = new AbortController()
    const seenSessionIDs = new Set<string>()

    void (async () => {
      try {
        for await (const event of ctx.event.subscribe({ signal: controller.signal })) {
          if (!event || typeof event !== "object") continue
          await processSubscribeEvent(event, seenSessionIDs)
        }
      } catch {
        // Subscription aborted on unload — fail open.
        traceHooks({ phase: "subscribe_ended", reason: "aborted_or_error" })
      }
    })()

    await ctx.session.hook("prompt", async (event) => {
      const sid = sessionIDFrom(event)
      await dispatchAtlas({
        hook_event_name: "session.prompt",
        session_id: sid,
      })
      // Observational only — OpenCode V2 prompt hooks have no typed rejection API.
      // Does NOT trigger session.start maintenance (session.created only).
    })

    await ctx.tool.hook("execute.before", async (event) => {
      const ev = event as { tool?: unknown; callID?: unknown; id?: unknown }
      const tool = typeof ev.tool === "string" ? ev.tool : ""
      const sid = sessionIDFrom(event)
      const callID =
        typeof ev.callID === "string" ? ev.callID : typeof ev.id === "string" ? ev.id : ""
      const res = await dispatchAtlas({
        hook_event_name: "tool.execute.before",
        session_id: sid,
        tool_name: tool,
        tool_call_id: callID,
      })
      if (shouldDenyToolExecution(res)) {
        throw new Error(DENY_MESSAGE)
      }
    })

    await ctx.tool.hook("execute.after", async (event) => {
      const ev = event as { tool?: unknown; callID?: unknown; status?: unknown }
      const tool = typeof ev.tool === "string" ? ev.tool : ""
      const sid = sessionIDFrom(event)
      const callID = typeof ev.callID === "string" ? ev.callID : ""
      const status = typeof ev.status === "string" ? ev.status : "success"
      await dispatchAtlas({
        hook_event_name: "tool.execute.after",
        session_id: sid,
        tool_name: tool,
        tool_call_id: callID,
        status: status === "error" || status === "failed" ? "error" : "success",
      })
    })

    await ctx.session.hook("compaction", async (event) => {
      const sid = sessionIDFrom(event)
      await dispatchAtlas({
        hook_event_name: "session.compaction",
        session_id: sid,
        reason: "compaction",
      })
    })

    traceHooks({ phase: "setup_complete" })
    return () => {
      controller.abort()
      traceHooks({ phase: "cleanup" })
    }
  },
})
