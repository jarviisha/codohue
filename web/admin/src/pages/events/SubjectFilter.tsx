import { Button, FormControl, TextInput, Toolbar } from '../../components/ui'

interface SubjectFilterProps {
  value: string
  applied: boolean
  onChange: (value: string) => void
  onApply: () => void
  onClear: () => void
}

export default function SubjectFilter({ value, applied, onChange, onApply, onClear }: SubjectFilterProps) {
  return (
    <Toolbar className="mb-4">
      <FormControl
        label="Filter by Subject ID"
        htmlFor="event-subject-filter"
        labelClassName="text-[11px] font-semibold uppercase tracking-[0.06em] text-muted"
      >
        <TextInput
          id="event-subject-filter"
          value={value}
          onChange={e => onChange(e.target.value)}
          onKeyDown={e => e.key === 'Enter' && onApply()}
          placeholder="user-1"
          className="w-48"
        />
      </FormControl>
      <Button onClick={onApply} variant="primary" size="sm">
        Apply
      </Button>
      {applied && (
        <Button onClick={onClear} variant="ghost" size="sm">
          Clear
        </Button>
      )}
    </Toolbar>
  )
}
