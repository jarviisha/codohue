import { useEffect, useRef, useState } from 'react'

export interface Command {
  id: string
  label: string
  hint?: string
  action: () => void
}

const REGISTRY = new Map<string, Command>()
const SUBSCRIBERS = new Set<() => void>()

function notify() {
  for (const cb of SUBSCRIBERS) cb()
}

export function useRegisterCommand(
  id: string,
  label: string,
  action: () => void,
  hint?: string,
) {
  const actionRef = useRef(action)

  useEffect(() => {
    actionRef.current = action
  }, [action])

  useEffect(() => {
    REGISTRY.set(id, {
      id,
      label,
      hint,
      action: () => actionRef.current(),
    })
    notify()
    return () => {
      REGISTRY.delete(id)
      notify()
    }
  }, [id, label, hint])
}

export function useCommandList(): Command[] {
  const [list, setList] = useState<Command[]>(() => Array.from(REGISTRY.values()))

  useEffect(() => {
    const update = () => setList(Array.from(REGISTRY.values()))
    SUBSCRIBERS.add(update)
    return () => {
      SUBSCRIBERS.delete(update)
    }
  }, [])

  return list
}
