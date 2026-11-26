// Official Google A2UI React renderer bundled for ChatGo.
//
// It uses @a2ui/web_core's MessageProcessor and @a2ui/react's A2uiSurface, so
// the surface tree, bindings and Basic Catalog come straight from the upstream
// packages. The host bridge is shared with the CopilotKit bundle.
import { createRoot } from 'react-dom/client'
import { useSyncExternalStore } from 'react'
import { MessageProcessor } from '@a2ui/web_core'
import {
  basicCatalog,
  injectBasicCatalogStyles,
  setMarkdownRenderer,
} from '@a2ui/web_core/catalogs/basic/v1'
import { renderMarkdown } from '@a2ui/markdown-it'
import { A2uiSurface } from '@a2ui/react'
import { injectStyles } from '@a2ui/react/styles'
import {
  installBridge,
  postAction,
  postRendererMessage,
  reportError,
  type Envelope,
} from './bridge'

injectBasicCatalogStyles()
injectStyles()
setMarkdownRenderer(renderMarkdown)

let revision = 0
const listeners = new Set<() => void>()

function emit(): void {
  revision += 1
  for (const listener of listeners) listener()
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
  }
}

function readDataModel(surfaceId: string): unknown {
  const surface = processor.getSurface(surfaceId)
  if (!surface) return undefined
  try {
    return surface.dataModel.get('/')
  } catch {
    return undefined
  }
}

const processor = new MessageProcessor(
  [basicCatalog],
  (action: Record<string, unknown>) => {
    postAction(
      {
        version: '1.0',
        name: action.name,
        surfaceId: action.surfaceId,
        sourceComponentId: action.sourceComponentId,
        timestamp: action.timestamp,
        context: action.context ?? {},
        ...(action.userMessage ? { userMessage: action.userMessage } : {}),
        ...(action.metadata ? { metadata: action.metadata } : {}),
      },
      readDataModel(String(action.surfaceId ?? '')),
    )
  },
  {
    outboundListener: (message) => postRendererMessage(message),
    errorHandler: (message) => {
      const error = (message as { error?: { message?: string } }).error
      reportError(error?.message ?? message)
    },
    reportErrorsOnly: true,
  },
)

processor.onSurfaceCreated(() => emit())
processor.onSurfaceDeleted(() => emit())

function App() {
  useSyncExternalStore(subscribe, () => revision)
  const surfaces = Array.from(processor.model.surfacesMap.values())
  return (
    <>
      {surfaces.map((surface) => (
        <A2uiSurface key={surface.id} surface={surface} />
      ))}
    </>
  )
}

function apply(messages: Envelope[]): void {
  processor.processMessages(messages)
  emit()
}

installBridge(apply)
createRoot(document.getElementById('root')!).render(<App />)