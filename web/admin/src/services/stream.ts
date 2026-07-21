import { useEffect, useRef, useState } from 'react'
import { apiBaseUrl } from './http'

type StreamHandler = (data: unknown) => void
export type StreamHandlers = Record<string, StreamHandler>

export type UseServerStreamOptions = {
  enabled?: boolean
  onError?: (event: Event) => void
}

export type UseServerStreamResult = {
  connected: boolean
  lastPingAt: number | null
}

/**
 * useServerStream subscribes to a Server-Sent Events endpoint and routes named
 * events to handler callbacks. `ping` events are tracked separately as a
 * heartbeat watchdog — UI can render staleness if `lastPingAt` falls behind.
 *
 * Handlers are pinned via a ref so callers can pass fresh closures every
 * render without thrashing the EventSource connection.
 *
 * The hook listens for the global `codohue:auth-expired` event to drop the
 * connection cleanly when the session is invalidated.
 */
export function useServerStream(
  url: string | null,
  handlers: StreamHandlers,
  options: UseServerStreamOptions = {},
): UseServerStreamResult {
  const { enabled = true, onError } = options
  const [connected, setConnected] = useState(false)
  const [lastPingAt, setLastPingAt] = useState<number | null>(null)

  const handlersRef = useRef(handlers)
  const onErrorRef = useRef(onError)

  // React 19 forbids ref mutation during render. Pin latest closures in an
  // effect that runs after every render — the EventSource listeners read
  // *Ref.current at fire time so they always see the current callbacks.
  useEffect(() => {
    handlersRef.current = handlers
    onErrorRef.current = onError
  })

  // Snapshot the set of event names that need addEventListener wiring. We
  // intentionally fix this at mount; callers that need a different event set
  // should re-key the parent component to remount the hook.
  const eventNames = Object.keys(handlers)
  const eventNamesKey = eventNames.join('|')

  useEffect(() => {
    if (!url || !enabled) return

    const es = new EventSource(`${apiBaseUrl}${url}`, { withCredentials: true })

    es.onopen = () => setConnected(true)
    es.onerror = (event) => {
      setConnected(false)
      onErrorRef.current?.(event)
    }

    const listeners: Array<[string, (event: MessageEvent) => void]> = []
    for (const name of eventNamesKey.split('|').filter(Boolean)) {
      const listener = (event: MessageEvent) => {
        let payload: unknown
        try {
          payload = JSON.parse(event.data as string)
        } catch {
          payload = event.data
        }
        handlersRef.current[name]?.(payload)
      }
      es.addEventListener(name, listener)
      listeners.push([name, listener])
    }

    const onPing = () => setLastPingAt(Date.now())
    es.addEventListener('ping', onPing)

    const onAuthExpired = () => es.close()
    window.addEventListener('codohue:auth-expired', onAuthExpired)

    return () => {
      for (const [name, fn] of listeners) es.removeEventListener(name, fn)
      es.removeEventListener('ping', onPing)
      window.removeEventListener('codohue:auth-expired', onAuthExpired)
      es.close()
      setConnected(false)
    }
  }, [url, enabled, eventNamesKey])

  return { connected, lastPingAt }
}
