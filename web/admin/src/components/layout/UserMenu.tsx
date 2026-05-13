// Placeholder. Real menu (logout + session info) lands when auth wires up in
// Phase 1.4.
export default function UserMenu() {
  return (
    <button
      type="button"
      className="h-7 w-7 flex items-center justify-center rounded-sm text-muted hover:text-primary hover:bg-surface-raised"
      aria-label="User menu"
      title="User menu"
    >
      <span className="font-mono text-sm">◯</span>
    </button>
  )
}
