// Placeholder. Real menu (logout + session info) lands when auth wires up in
// Phase 1.4 follow-up. Icons deferred — uses a plain text label.
export default function UserMenu() {
  return (
    <button
      type="button"
      className="h-8 px-2.5 flex items-center justify-center rounded-sm border border-default text-secondary hover:text-primary hover:border-strong font-mono text-xs uppercase tracking-[0.04em]"
      aria-label="User menu"
      title="User menu"
    >
      user
    </button>
  )
}
