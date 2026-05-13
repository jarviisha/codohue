import { useParams } from 'react-router-dom'

// Stub modal. Real modal lands in Phase 2 entry #12.
export default function CatalogItemDetailModal() {
  const { id } = useParams<{ id: string }>()
  return (
    <div className="mt-6 border border-default rounded-sm bg-surface p-4">
      <p className="text-sm text-muted">
        <span className="font-mono text-muted">[PEND]</span> catalog item modal · id={' '}
        <span className="font-mono text-primary">{id}</span>
      </p>
    </div>
  )
}
