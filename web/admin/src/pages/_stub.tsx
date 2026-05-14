// Shared placeholder for Phase 1.2 routing skeleton.
// Real pages replace these stubs in Phase 1.5 (Login, Health) and Phase 2.
import { PageHeader, PageShell } from '../components/ui'

interface PageStubProps {
  title: string
}

export default function PageStub({ title }: PageStubProps) {
  return (
    <PageShell>
      <PageHeader title={title} />
      <p className="text-sm text-muted">
        <span className="font-mono text-xs uppercase tracking-[0.04em] text-muted">PEND</span>{' '}
        implementation pending.
      </p>
    </PageShell>
  )
}
