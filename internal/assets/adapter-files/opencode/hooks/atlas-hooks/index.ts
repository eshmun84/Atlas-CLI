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
 */

export const DENY_MESSAGE = "blocked by atlas hooks"

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

export function sessionIDFrom(value: unknown): string {
  if (!value || typeof value !== "object") return ""
  const v = value as Record<string, unknown>
  for (const key of ["sessionID", "sessionId", "session_id", "id"]) {
    const cur = v[key]
    if (typeof cur === "string" && cur.trim()) return cur.trim()
  }
  const properties = v.properties
  if (properties && typeof properties === "object") {
    return sessionIDFrom(properties)
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
  let body: string
  try {
    body = JSON.stringify(payload)
  } catch {
    return allowDecision()
  }
  if (utf8ByteLength(body) > MAX_BYTES) {
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
      return allowDecision()
    }

    if (outcome.kind === "fail") {
      return allowDecision()
    }

    if (outcome.stdout.truncated || outcome.stderr.truncated) {
      killProc(proc)
      return allowDecision()
    }

    // Never trust stdout when the process failed.
    if (outcome.code !== 0) {
      return allowDecision()
    }

    const text = decodeUTF8(outcome.stdout.bytes)
    if (text === null) {
      return allowDecision()
    }

    const parsed = parseAtlasResponse(text)
    if (!parsed) {
      return allowDecision()
    }
    return parsed
  } catch {
    killProc(proc)
    return allowDecision()
  } finally {
    if (timer !== undefined) clearTimeout(timer)
  }
}

export default Plugin.define({
  id: "atlas.hooks",
  async setup(ctx) {
    const controller = new AbortController()

    void (async () => {
      try {
        for await (const event of ctx.event.subscribe({ signal: controller.signal })) {
          if (!event || typeof event !== "object") continue
          const typed = event as { type?: string; properties?: unknown }
          if (typed.type !== "session.created") continue
          const sid = sessionIDFrom(typed.properties) || sessionIDFrom(typed)
          if (!sid) continue
          await dispatchAtlas({
            hook_event_name: "session.created",
            session_id: sid,
          })
        }
      } catch {
        // Subscription aborted on unload — fail open.
      }
    })()

    await ctx.session.hook("prompt", async (event) => {
      const sid = sessionIDFrom(event)
      await dispatchAtlas({
        hook_event_name: "session.prompt",
        session_id: sid,
      })
      // Observational only — OpenCode V2 prompt hooks have no typed rejection API.
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

    return () => controller.abort()
  },
})
