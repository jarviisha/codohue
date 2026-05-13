import Button from './Button'

interface PaginationProps {
  offset: number
  limit: number
  total?: number // when missing, only prev/next is enabled — assumes there might be more
  onOffsetChange: (offset: number) => void
}

// Offset/limit pagination footer for data tables. Renders the showing-range
// on the left and prev/next buttons on the right. Text labels only — no
// arrow glyphs (per icon rule).
export default function Pagination({
  offset,
  limit,
  total,
  onOffsetChange,
}: PaginationProps) {
  const hasPrev = offset > 0
  const hasNext = total === undefined ? true : offset + limit < total

  const startIdx = offset + 1
  const endIdx = total === undefined ? offset + limit : Math.min(offset + limit, total)
  const showingPrefix = `Showing ${startIdx}–${endIdx}`
  const showing = total === undefined ? showingPrefix : `${showingPrefix} of ${total}`

  return (
    <div className="flex items-center justify-between text-sm">
      <span className="font-mono tabular-nums text-muted">{showing}</span>
      <div className="flex items-center gap-1">
        <Button
          variant="ghost"
          size="sm"
          disabled={!hasPrev}
          onClick={() => onOffsetChange(Math.max(0, offset - limit))}
        >
          prev
        </Button>
        <Button
          variant="ghost"
          size="sm"
          disabled={!hasNext}
          onClick={() => onOffsetChange(offset + limit)}
        >
          next
        </Button>
      </div>
    </div>
  )
}
