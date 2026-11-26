// CopilotKit A2UI React renderer bundled for ChatGo.
//
// It mounts @copilotkit/a2ui-renderer's A2UIProvider and A2UIRenderer, which
// ship the Basic Catalog as React components. The host bridge is shared with
// the official @a2ui/react bundle.
import { createRoot } from 'react-dom/client'
import { useEffect, useState } from 'react'
import {
  A2UIProvider,
  A2UIRenderer,
  useA2UIActions,
} from '@copilotkit/a2ui-renderer'
import {
  installBridge,
  postAction,
  reportError,
  setApplyHandler,
  type Envelope,
} from './bridge'

let getSurface: ((surfaceId: string) => unknown) | null = null

function readDataModel(surfaceId: string): unknown {
  if (!getSurface) return undefined
  const surface = getSurface(surfaceId) as
    | { dataModel?: { get(path: string): unknown } }
    | undefined
  if (!surface?.dataModel) return undefined
  try {
    return surface.dataModel.get('/')
  } catch {
    return undefined
  }
}

function handleAction(message: unknown): void {
  const envelope = message as {
    userAction?: Record<string, unknown>
    action?: Record<string, unknown>
  }
  const action = envelope?.userAction ?? envelope?.action ?? (message as Record<string, unknown>)
  if (!action || typeof action.name !== 'string') return
  postAction(
    {
      version: '1.0',
      name: action.name,
      surfaceId: action.surfaceId ?? '',
      sourceComponentId: action.sourceComponentId ?? '',
      timestamp: action.timestamp,
      context: action.context ?? {},
      ...(action.userMessage ? { userMessage: action.userMessage } : {}),
      ...(action.metadata ? { metadata: action.metadata } : {}),
    },
    readDataModel(String(action.surfaceId ?? '')),
  )
}

function withSurfaces(previous: string[], messages: Envelope[]): string[] {
  const next = new Set(previous)
  for (const message of messages) {
    for (const key of [
      'createSurface',
      'updateComponents',
      'updateDataModel',
      'deleteSurface',
    ]) {
      const body = (message as Record<string, unknown>)[key] as
        | { surfaceId?: unknown }
        | undefined
      const id = body?.surfaceId
      if (typeof id !== 'string' || id === '') continue
      if (key === 'deleteSurface') next.delete(id)
      else next.add(id)
    }
  }
  if (next.size === previous.length && previous.every((id) => next.has(id))) {
    return previous
  }
  return Array.from(next)
}

function Surfaces() {
  const actions = useA2UIActions()
  const [ids, setIds] = useState<string[]>([])

  getSurface = actions.getSurface

  useEffect(() => {
    setApplyHandler((messages) => {
      setIds((previous) => withSurfaces(previous, messages))
      try {
        actions.processMessages(messages as Array<Record<string, unknown>>)
      } catch (err) {
        reportError(err)
      }
    })
  }, [actions])

  return (
    <>
      {ids.map((id) => (
        <A2UIRenderer key={id} surfaceId={id} />
      ))}
    </>
  )
}

function App() {
  return (
    <A2UIProvider onAction={handleAction}>
      <Surfaces />
    </A2UIProvider>
  )
}

// Start polling before React mounts; the bridge queues envelopes until the
// provider registers its handler.
installBridge()
createRoot(document.getElementById('root')!).render(<App />)