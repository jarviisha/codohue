import { Button } from '../../components/ui'

export default function EventsPagination({
  offset,
  pageEnd,
  total,
  onPrevious,
  onNext,
}: {
  offset: number
  pageEnd: number
  total: number
  onPrevious: () => void
  onNext: () => void
}) {
  return (
    <div className="flex items-center justify-between mt-4">
      <Button onClick={onPrevious} disabled={offset === 0}>
        Prev
      </Button>
      <Button onClick={onNext} disabled={pageEnd >= total}>
        Next
      </Button>
    </div>
  )
}
