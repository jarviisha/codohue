// Shared placeholder for Phase 1.2 routing skeleton.
// Real pages replace these stubs in Phase 1.5 (Login, Health) and Phase 2.
interface PageStubProps {
  title: string
}

export default function PageStub({ title }: PageStubProps) {
  return (
    <section>
      <h1 className="text-xl font-semibold text-primary leading-tight mb-4">
        {title}
      </h1>
      <p className="text-sm text-muted">
        <span className="font-mono text-muted">[PEND]</span> implementation pending.
      </p>
    </section>
  )
}
