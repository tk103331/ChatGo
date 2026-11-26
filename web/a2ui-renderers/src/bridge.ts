// Shared host bridge for the optional React A2UI bundles.
//
// The host injects one envelope at a time through GET /pending and receives
// actions, size changes and renderer-to-agent messages through POST /rpc.
// Both bundles feed the envelopes to their own renderer and reuse this file.

export type Envelope = Record<string, unknown>

// Some renderers clone incoming messages. Older embedded WebViews may not
// expose structuredClone, so fall back to JSON, which is enough for A2UI
// envelopes (they are already JSON).
const runtime = globalThis as unknown as {
  structuredClone?: (value: unknown) => unknown
}
if (typeof runtime.structuredClone !== 'function') {
  runtime.structuredClone = (value: unknown) =>
    value === undefined ? value : JSON.parse(JSON.stringify(value))
}

type ApplyHandler = (messages: Envelope[]) => void

const queued: Envelope[] = []
let applyHandler: ApplyHandler | null = null
let installed = false

function post(body: unknown): Promise<unknown> {
  return fetch('rpc', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify(body),
  }).catch(() => undefined)
}

export function reportError(message: unknown): void {
  const text = message instanceof Error ? message.message : String(message)
  void post({ type: 'error', message: text })
}

export function postAction(action: Record<string, unknown>, dataModel?: unknown): void {
  const body: Record<string, unknown> = { type: 'action', action }
  if (dataModel !== undefined) body.dataModel = dataModel
  void post(body)
}

export function postRendererMessage(message: unknown): void {
  void post({ type: 'rendererMessage', rendererMessage: message })
}

let lastHeight = -1
function postResize(): void {
  requestAnimationFrame(() => {
    const height = Math.ceil(document.documentElement.scrollHeight)
    if (height === lastHeight) return
    lastHeight = height
    void post({ type: 'resize', width: window.innerWidth, height })
  })
}

function apply(messages: Envelope[]): void {
  if (messages.length === 0) return
  if (!applyHandler) {
    queued.push(...messages)
    return
  }
  try {
    applyHandler(messages)
  } catch (err) {
    reportError(err)
  }
  postResize()
}

function poll(): void {
  if (document.hidden) return
  fetch('pending')
    .then((res) => {
      if (res.status === 204) return null
      if (!res.ok) throw new Error(`pending ${res.status}`)
      return res.json() as Promise<Envelope>
    })
    .then((msg) => {
      if (msg) apply([msg])
      if (!document.hidden) setTimeout(poll, 0)
    })
    .catch((err: unknown) => {
      reportError(err)
      if (!document.hidden) setTimeout(poll, 1000)
    })
}

/** Registers the renderer that consumes envelopes and flushes any backlog. */
export function setApplyHandler(handler: ApplyHandler): void {
  applyHandler = handler
  if (queued.length > 0) {
    const pending = queued.splice(0)
    apply(pending)
  }
}

/** Installs the polling bridge and publishes window.a2ui.apply. */
export function installBridge(handler?: ApplyHandler): void {
  if (handler) setApplyHandler(handler)

  const host = (window as unknown as { a2ui?: Record<string, unknown> }).a2ui ?? {}
  host.apply = (messages: Envelope | Envelope[]) => {
    apply(Array.isArray(messages) ? messages : [messages])
  }
  ;(window as unknown as { a2ui?: Record<string, unknown> }).a2ui = host

  if (installed) return
  installed = true

  new ResizeObserver(postResize).observe(document.documentElement)
  document.addEventListener('visibilitychange', () => {
    if (!document.hidden) poll()
  })

  const start = () => {
    poll()
    postResize()
  }
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', start, { once: true })
  } else {
    start()
  }
}