import { Stack } from '@astryxdesign/core'
import CopyButton from '@/components/CopyButton'

/**
 * SecretValue renders a credential the server hands out exactly once (a
 * namespace API key) together with a copy affordance.
 *
 * Printing the value is the whole point: a "copy it from the response" hint
 * without the value leaves the operator with a key they can only recover by
 * rotating it.
 */
export default function SecretValue({ value, label }: { value: string; label?: string }) {
  return (
    <Stack
      direction="horizontal"
      gap={2}
      align="center"
      justify="between"
      className="border-border w-full border p-2"
    >
      <code className="text-primary font-mono text-sm break-all">{value}</code>
      <CopyButton value={value} label={label} />
    </Stack>
  )
}
