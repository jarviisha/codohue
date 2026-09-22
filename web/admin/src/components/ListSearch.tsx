import { useState } from 'react'
import { Button, Stack, TextInput } from '@astryxdesign/core'

/** Keep typing local; only an explicit submission changes the applied query. */
export default function ListSearch({
  value,
  label,
  description,
  onApply,
  onOpen,
}: {
  value: string
  label: string
  description: string
  onApply: (value: string) => void
  onOpen?: (value: string) => void
}) {
  const [draft, setDraft] = useState(value)
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        onApply(draft.trim())
      }}
    >
      <Stack direction="horizontal" gap={2} align="end" wrap="wrap">
        <TextInput
          label={label}
          description={description}
          value={draft}
          onChange={setDraft}
          hasClear
          size="sm"
        />
        <Button type="submit" label="Apply filter" size="sm" />
        <Button
          type="button"
          label="Clear filter"
          variant="ghost"
          size="sm"
          isDisabled={!draft && !value}
          onClick={() => {
            setDraft('')
            onApply('')
          }}
        />
        {onOpen && (
          <Button
            type="button"
            label="Open exact ID"
            variant="secondary"
            size="sm"
            isDisabled={!draft.trim()}
            onClick={() => onOpen(draft.trim())}
          />
        )}
      </Stack>
    </form>
  )
}
