import { useState } from 'react'
import { Button, Stack } from '@astryxdesign/core'

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
    <Stack
      direction="horizontal"
      gap={2}
      align="center"
      justify="between"
      className="border-border w-full border p-2"
    >
      <code className="text-primary font-mono text-sm break-all">{value}</code>
      <Button
        size="sm"
        variant="secondary"
        onClick={copy}
        label={state === 'copied' ? 'Copied' : state === 'failed' ? 'Copy failed' : 'Copy'}
        tooltip={label ? `Copy ${label}` : 'Copy to clipboard'}
      />
    </Stack>
  )
}
