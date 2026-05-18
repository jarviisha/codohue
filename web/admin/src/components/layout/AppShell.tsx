import { Outlet } from 'react-router-dom'
import Sidebar from './Sidebar'
import TopBar from './TopBar'
import ErrorBoundary from './ErrorBoundary'
import CommandPalette from '@/components/ui/CommandPalette'

// Top-level shell: fixed Sidebar (240px) + fixed TopBar (48px) + scrolling
// content. Login route renders outside this shell.
//
// CommandPalette is mounted here so the Cmd+K listener is active on every
// shell page; the palette modal itself only renders when open.
//
// ErrorBoundary wraps the Outlet so a crashed page leaves the sidebar and
// top bar interactive — operators always have an escape hatch.
//
// The skip-to-content link is the first tab stop on every page so keyboard
// operators can bypass the sidebar nav. <main> carries tabIndex=-1 so the
// browser can focus it when the link's hash target is followed.
export default function AppShell() {
  return (
    <div className="min-h-screen bg-surface text-primary">
      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:left-2 focus:top-2 focus:z-100 focus:rounded-sm focus:bg-accent-emphasis focus:px-3 focus:py-2 focus:font-mono focus:text-xs focus:uppercase focus:tracking-[0.06em] focus:text-accent-text focus:shadow-focus focus:outline-none"
      >
        Skip to content
      </a>
      <Sidebar />
      <div className="ml-60">
        <TopBar />
        <main
          id="main-content"
          tabIndex={-1}
          className="fixed left-60 right-0 top-12 bottom-0 overflow-y-auto bg-base focus:outline-none"
        >
          <div className="min-h-full px-6 pb-6">
            <ErrorBoundary>
              <Outlet />
            </ErrorBoundary>
          </div>
        </main>
      </div>
      <CommandPalette />
    </div>
  )
}
