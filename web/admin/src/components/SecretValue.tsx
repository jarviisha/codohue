import { useState } from 'react'
import { Button, Inline } from '@jarviisha/davinci-react-ui'

/**
 * SecretValue renders a credential the server hands out exactly once (a
 * namespace API key) together with a copy affordance.
 *
 * Printing the value is the whole point: a "copy it from the response" hint
 * without the value leaves the operator with a key they can only recover by
 * rotating it.
 *
 * The copy result is reflected in the button label rather than a toast so the
 * feedback stays attached to the value being copied, and a clipboard failure
 * (insecure origin, denied permission) says so instead of silently no-opping.
 */
export default function SecretValue({ value, label }: { value: string; label?: string }) {
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
    <Inline align="center" justify="between" className="border-border w-full gap-2 border p-2">
      <code className="text-foreground font-mono text-sm break-all">{value}</code>
      <Button
        size="sm"
        variant="outline"
        tone="neutral"
        onClick={copy}
        aria-label={label ? `Copy ${label}` : 'Copy to clipboard'}
      >
        {state === 'copied' ? 'Copied' : state === 'failed' ? 'Copy failed' : 'Copy'}
      </Button>
    </Inline>
  )
}
