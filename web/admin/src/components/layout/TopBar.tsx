import { Button } from '@/components/ui'
import Ps1Prompt from './Ps1Prompt'
import ThemeToggle from './ThemeToggle'
import UserMenu from './UserMenu'

// Top bar: PS1 prompt on the left, command-palette hint + theme toggle + user
// menu on the right. Pinned above the scrolling content.
export default function TopBar() {
  return (
    <header className="fixed top-0 left-60 right-0 h-12 bg-base border-b border-default flex items-center justify-between px-6 z-30">
      <Ps1Prompt />
      <div className="flex items-center gap-2">
        <Button
          type="button"
          size='xs'
          aria-label="Open command palette"
          title="Open command palette (Cmd+K / Ctrl+K)"
        >
          <span>Cmd+K</span>
        </Button>
        <ThemeToggle />
        <UserMenu />
      </div>
    </header>
  )
}
