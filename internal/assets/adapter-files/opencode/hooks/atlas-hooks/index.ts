import { appendFileSync, chmodSync, mkdirSync, realpathSync, statSync } from "node:fs"
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
 * Event contract (OpenCode 2.0.20, installed binary):
 * - Plugin stream: type "session.created" with data.sessionID.
 * - SDK shapes may use properties.sessionID.
 * - Sync bus: type "sync", name "session.created.1", data.sessionID.
 * - Session directory is Location.Ref: data.location.directory, the event
 *   envelope location.directory, or SessionV1 data.info.directory.
 *   A bare plugin cwd is not session identity.
 * - tool.execute.before / tool.execute.after carry sessionID, not a
 *   directory. Directory comes from ctx.session.get({sessionID}).location.directory.
 * - Missing, contradictory, or foreign directories skip dispatch.
 *   They do not deny the tool and do not write another project's Home.
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
  location?: {
    directory?: unknown
    project?: {
      directory?: unknown
      canonical?: unknown
    }
  }
  event: {
    subscribe: (opts: { signal?: AbortSignal }) => AsyncIterable<unknown>
  }
  session: {
    hook: (name: string, cb: (event: unknown) => Promise<void> | void) => Promise<unknown>
    get?: (input: { sessionID: string }) => Promise<unknown>
  }
  tool: {
    hook: (name: string, cb: (event: unknown) => Promise<void> | void) => Promise<unknown>
  }
}

/**
 * Atlas child working directory. Set from ctx.location.directory in setup.
 * Empty means no reliable project root — dispatch must not spawn against a wrong cwd.
 */
let atlasDispatchCwd: string | undefined

/**
 * Validate a candidate project directory for Atlas hook dispatch.
 * Returns an absolute-or-as-given path string when the path exists and is a directory.
 * Does not invent ATLAS_HOME or rewrite paths.
 */
export function resolveAtlasDispatchCwd(directory: unknown): string | undefined {
  if (typeof directory !== "string") return undefined
  const trimmed = directory.trim()
  if (!trimmed || trimmed.includes("\0")) return undefined
  try {
    if (!statSync(trimmed).isDirectory()) return undefined
  } catch {
    return undefined
  }
  return trimmed
}

/**
 * Configure the cwd used by Bun.spawn for atlas hooks.
 * Returns true when the directory is usable.
 */
export function configureAtlasDispatchCwd(directory: unknown): boolean {
  const resolved = resolveAtlasDispatchCwd(directory)
  atlasDispatchCwd = resolved
  return resolved !== undefined
}

/** Test helper: clear configured dispatch cwd. */
export function resetAtlasDispatchCwdForTest(): void {
  atlasDispatchCwd = undefined
}

/** Env for the Atlas child: parent process.env only (never invent ATLAS_HOME). */
function atlasChildEnv(): Record<string, string | undefined> {
  return { ...process.env }
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

export type SessionDirectoryRead =
  | { kind: "ok"; directory: string }
  | { kind: "missing" }
  | { kind: "contradictory" }
  | { kind: "unresolvable" }

export type MembershipReason =
  | "missing_instance_directory"
  | "missing_session_directory"
  | "foreign_session_directory"
  | "contradictory_session_directory"
  | "unresolvable_session_directory"
  | "session_lookup_failed"

export type Membership =
  | { ok: true; directory: string }
  | { ok: false; reason: MembershipReason }

function directoryString(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined
  const trimmed = value.trim()
  if (!trimmed || trimmed.includes("\0")) return undefined
  return trimmed
}

/**
 * Confirmed OpenCode 2.0.20 directory fields only:
 * location.directory (Location.Ref / event envelope) and info.directory (SessionV1).
 * Nested under data, properties, or payload. A bare string `directory` is not identity.
 */
function collectConfirmedSessionDirectories(value: unknown, out: Set<string>, depth: number): void {
  if (!value || typeof value !== "object" || depth > 5) return
  const v = value as Record<string, unknown>
  if (v.location && typeof v.location === "object") {
    const dir = directoryString((v.location as Record<string, unknown>).directory)
    if (dir) out.add(dir)
  }
  if (v.info && typeof v.info === "object") {
    const dir = directoryString((v.info as Record<string, unknown>).directory)
    if (dir) out.add(dir)
  }
  for (const nest of ["data", "properties", "payload"]) {
    const nested = v[nest]
    if (nested && typeof nested === "object") {
      collectConfirmedSessionDirectories(nested, out, depth + 1)
    }
  }
}

function unifyExistingDirectories(dirs: Set<string>): SessionDirectoryRead {
  if (dirs.size === 0) return { kind: "missing" }
  const canonical = new Set<string>()
  for (const dir of dirs) {
    try {
      canonical.add(realpathSync(dir))
    } catch {
      return { kind: "unresolvable" }
    }
  }
  if (canonical.size !== 1) return { kind: "contradictory" }
  const directory = canonical.values().next().value
  if (!directory) return { kind: "missing" }
  return { kind: "ok", directory }
}

/** Read the session directory from a confirmed OpenCode event shape. */
export function readSessionDirectory(event: unknown): SessionDirectoryRead {
  const found = new Set<string>()
  collectConfirmedSessionDirectories(event, found, 0)
  return unifyExistingDirectories(found)
}

/**
 * session.get return used by OpenCode 2.0.20: { location: { directory } }.
 * Other shapes are not treated as identity.
 */
export function directoryFromSessionRecord(session: unknown): string | undefined {
  if (!session || typeof session !== "object") return undefined
  const location = (session as Record<string, unknown>).location
  if (!location || typeof location !== "object") return undefined
  return directoryString((location as Record<string, unknown>).directory)
}

function instanceRealpath(instanceDirectory: string | undefined): string | undefined {
  if (typeof instanceDirectory !== "string") return undefined
  const trimmed = instanceDirectory.trim()
  if (!trimmed || trimmed.includes("\0")) return undefined
  try {
    return realpathSync(trimmed)
  } catch {
    return undefined
  }
}

/** Bind a confirmed session directory to this plugin instance's project root. */
export function membershipForDirectory(
  sessionDirectory: string | undefined,
  instanceDirectory: string | undefined,
  read: SessionDirectoryRead,
): Membership {
  const instance = instanceRealpath(instanceDirectory)
  if (!instance) return { ok: false, reason: "missing_instance_directory" }
  if (read.kind === "contradictory") return { ok: false, reason: "contradictory_session_directory" }
  if (read.kind === "unresolvable") return { ok: false, reason: "unresolvable_session_directory" }
  if (read.kind !== "ok" || !sessionDirectory) return { ok: false, reason: "missing_session_directory" }
  if (read.directory !== instance) return { ok: false, reason: "foreign_session_directory" }
  return { ok: true, directory: read.directory }
}

export function membershipFor(event: unknown, instanceDirectory: string | undefined): Membership {
  const read = readSessionDirectory(event)
  return membershipForDirectory(read.kind === "ok" ? read.directory : undefined, instanceDirectory, read)
}

/**
 * Tool and session hooks do not carry a directory. When the event has none,
 * resolve ctx.session.get({ sessionID }).location.directory.
 */
export async function membershipForHook(
  event: unknown,
  instanceDirectory: string | undefined,
  lookup?: (sessionID: string) => Promise<unknown>,
): Promise<Membership> {
  const fromEvent = readSessionDirectory(event)
  if (fromEvent.kind === "ok") {
    return membershipForDirectory(fromEvent.directory, instanceDirectory, fromEvent)
  }
  if (fromEvent.kind === "contradictory" || fromEvent.kind === "unresolvable") {
    return membershipForDirectory(undefined, instanceDirectory, fromEvent)
  }
  const sid = sessionIDFrom(event)
  if (!sid || !lookup) {
    return membershipForDirectory(undefined, instanceDirectory, { kind: "missing" })
  }
  let session: unknown
  try {
    session = await lookup(sid)
  } catch {
    return { ok: false, reason: "session_lookup_failed" }
  }
  const dir = directoryFromSessionRecord(session)
  if (!dir) return membershipForDirectory(undefined, instanceDirectory, { kind: "missing" })
  return membershipForDirectory(dir, instanceDirectory, unifyExistingDirectories(new Set([dir])))
}

export type SessionCreatedHandle =
  | { kind: "dispatch"; session_id: string; session_directory: string }
  | {
      kind: "reject"
      reason:
        | "not_session_created"
        | "missing_session_id"
        | "duplicate"
        | MembershipReason
    }

/**
 * Normalize a subscribe-loop event into a session.created dispatch decision.
 * Membership is decided before dedupe. A foreign session is not remembered.
 */
export function handleSessionCreatedEvent(
  event: unknown,
  seenSessionIDs?: Set<string>,
  instanceDirectory?: string,
): SessionCreatedHandle {
  if (!isSessionCreatedEvent(event)) {
    return { kind: "reject", reason: "not_session_created" }
  }
  const sid = sessionIDFrom(event)
  if (!sid) {
    return { kind: "reject", reason: "missing_session_id" }
  }
  const member = membershipFor(event, instanceDirectory)
  if (!member.ok) {
    return { kind: "reject", reason: member.reason }
  }
  if (seenSessionIDs) {
    if (seenSessionIDs.has(sid)) {
      return { kind: "reject", reason: "duplicate" }
    }
    seenSessionIDs.add(sid)
  }
  return { kind: "dispatch", session_id: sid, session_directory: member.directory }
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
 * Resolve an explicit ATLAS_HOME for QA diagnostics.
 * Never falls back to ~/.atlas — missing/invalid home means no writes.
 */
export function resolveTraceHome(): string | undefined {
  const raw = process.env.ATLAS_HOME
  if (typeof raw !== "string") return undefined
  const home = raw.trim()
  if (!home || home.includes("\0")) return undefined
  try {
    if (!statSync(home).isDirectory()) return undefined
  } catch {
    // Absent path is acceptable only when the caller explicitly set ATLAS_HOME;
    // mkdir under that exact prefix happens in traceHooks (still no ~/.atlas).
  }
  return home
}

/**
 * Minimal QA diagnostics. Disabled unless ATLAS_OPENCODE_HOOKS_TRACE=1.
 * Writes only under an explicit, validated ATLAS_HOME — never ~/.atlas fallback.
 * Never records prompts, tool args, transcripts, paths as secrets, or code.
 */
export function traceHooks(fields: TraceFields): void {
  if (process.env[TRACE_ENV] !== "1") return
  try {
    const home = resolveTraceHome()
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
    mkdirSync(dir, { recursive: true, mode: 0o700 })
    try {
      chmodSync(dir, 0o700)
    } catch {
      // Best-effort; umask / platform may already match.
    }
    const logPath = join(dir, "opencode-hooks.jsonl")
    appendFileSync(logPath, line + "\n", { encoding: "utf8", mode: 0o600 })
    try {
      chmodSync(logPath, 0o600)
    } catch {
      // Best-effort restrictive mode.
    }
  } catch {
    // Diagnostics must never affect the agent session (fail-open).
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

  // OpenCode's server process cwd may be $HOME; Atlas resolves the project from Getwd.
  // Always spawn with the plugin location directory captured at setup.
  const cwd = resolveAtlasDispatchCwd(atlasDispatchCwd)
  if (!cwd) {
    traceHooks({ phase: "dispatch_failed", reason: "invalid_cwd", event_type: hookName })
    return allowDecision()
  }

  let proc: ReturnType<typeof Bun.spawn> | undefined
  let timer: ReturnType<typeof setTimeout> | undefined
  try {
    proc = Bun.spawn(["atlas", "hooks", "opencode", "--json"], {
      stdin: "pipe",
      stdout: "pipe",
      stderr: "pipe",
      cwd,
      env: atlasChildEnv(),
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
function noteRejected(eventType: string, reason: string, hasSessionID: boolean): void {
  traceHooks({
    phase: "event_rejected",
    event_type: eventType,
    has_session_id: hasSessionID,
    reason,
  })
  if (eventType === "session.created" && reason !== "not_session_created" && reason !== "duplicate") {
    try {
      console.error(`atlas-hooks: skip session.created ${reason}`)
    } catch {
      // Diagnostics must not affect the session.
    }
  }
}

export async function processSubscribeEvent(
  event: unknown,
  seenSessionIDs: Set<string>,
  dispatch: (payload: Record<string, string>) => Promise<AtlasDecision> = dispatchAtlas,
  instanceDirectory: string | undefined = atlasDispatchCwd,
): Promise<SessionCreatedHandle> {
  const eventType = eventTypeOf(event)
  if (eventType) {
    traceHooks({ phase: "event_received", event_type: eventType })
  }

  const handled = handleSessionCreatedEvent(event, seenSessionIDs, instanceDirectory)
  if (handled.kind === "reject") {
    if (handled.reason !== "not_session_created") {
      noteRejected(eventType || "session.created", handled.reason, handled.reason !== "missing_session_id")
    }
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
    session_directory: handled.session_directory,
  })
  return handled
}

/**
 * Dispatch a tool or session hook only when the session directory matches
 * this instance. A failed check returns allow and does not spawn atlas.
 */
export async function dispatchOwnedHook(
  payload: Record<string, string>,
  event: unknown,
  instanceDirectory: string | undefined,
  lookup: ((sessionID: string) => Promise<unknown>) | undefined,
  dispatch: (body: Record<string, string>) => Promise<AtlasDecision> = dispatchAtlas,
): Promise<AtlasDecision> {
  const member = await membershipForHook(event, instanceDirectory, lookup)
  const eventType = payload.hook_event_name ?? ""
  if (!member.ok) {
    noteRejected(eventType, member.reason, sessionIDFrom(event) !== "")
    return allowDecision()
  }
  return dispatch({
    ...payload,
    session_directory: member.directory,
  })
}

export default Plugin.define({
  id: "atlas.hooks",
  async setup(ctx) {
    traceHooks({ phase: "setup_enter" })
    if (!configureAtlasDispatchCwd(ctx.location?.directory)) {
      // Fail-open: hooks still register, but dispatch refuses to spawn without a project root.
      traceHooks({ phase: "dispatch_cwd_invalid", reason: "missing_or_invalid_location" })
    }
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

    const lookupSession = (sessionID: string) => {
      if (typeof ctx.session.get !== "function") {
        return Promise.reject(new Error("session.get unavailable"))
      }
      return ctx.session.get({ sessionID })
    }

    await ctx.session.hook("prompt", async (event) => {
      const sid = sessionIDFrom(event)
      await dispatchOwnedHook(
        {
          hook_event_name: "session.prompt",
          session_id: sid,
        },
        event,
        atlasDispatchCwd,
        lookupSession,
      )
      // Observational only — OpenCode V2 prompt hooks have no typed rejection API.
      // Does NOT trigger session.start maintenance (session.created only).
    })

    await ctx.tool.hook("execute.before", async (event) => {
      const ev = event as { tool?: unknown; callID?: unknown; id?: unknown }
      const tool = typeof ev.tool === "string" ? ev.tool : ""
      const sid = sessionIDFrom(event)
      const callID =
        typeof ev.callID === "string" ? ev.callID : typeof ev.id === "string" ? ev.id : ""
      const res = await dispatchOwnedHook(
        {
          hook_event_name: "tool.execute.before",
          session_id: sid,
          tool_name: tool,
          tool_call_id: callID,
        },
        event,
        atlasDispatchCwd,
        lookupSession,
      )
      if (shouldDenyToolExecution(res)) {
        throw new Error(DENY_MESSAGE)
      }
    })

    await ctx.tool.hook("execute.after", async (event) => {
      const ev = event as { tool?: unknown; callID?: unknown; id?: unknown; status?: unknown }
      const tool = typeof ev.tool === "string" ? ev.tool : ""
      const sid = sessionIDFrom(event)
      const callID =
        typeof ev.callID === "string" ? ev.callID : typeof ev.id === "string" ? ev.id : ""
      const status = typeof ev.status === "string" ? ev.status : "success"
      await dispatchOwnedHook(
        {
          hook_event_name: "tool.execute.after",
          session_id: sid,
          tool_name: tool,
          tool_call_id: callID,
          status: status === "error" || status === "failed" ? "error" : "success",
        },
        event,
        atlasDispatchCwd,
        lookupSession,
      )
    })

    await ctx.session.hook("compaction", async (event) => {
      const sid = sessionIDFrom(event)
      await dispatchOwnedHook(
        {
          hook_event_name: "session.compaction",
          session_id: sid,
          reason: "compaction",
        },
        event,
        atlasDispatchCwd,
        lookupSession,
      )
    })

    traceHooks({ phase: "setup_complete" })
    return () => {
      controller.abort()
      traceHooks({ phase: "cleanup" })
    }
  },
})
