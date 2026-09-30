import { useState } from 'react'
import { Button } from '@astryxdesign/core'

/**
 * CopyButton copies `value` and reports the result on its own label rather
 * than a toast, so feedback stays attached to the value; a clipboard failure
 * (insecure origin, denied permission) says so instead of silently no-opping.
 */
export default function CopyButton({ value, label }: { value: string; label?: string }) {
  const [state, setState] = useState<'idle' | 'copied' | 'failed'>('idle')

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setState('copied')
      window.setTimeout(() => setState('idle'), 2000)
    } catch {
      setState('failed')
    }
  }

  return (
    <Button
      size="sm"
      variant="secondary"
      onClick={copy}
      label={state === 'copied' ? 'Copied' : state === 'failed' ? 'Copy failed' : 'Copy'}
      tooltip={label ? `Copy ${label}` : 'Copy to clipboard'}
    />
  )
}
