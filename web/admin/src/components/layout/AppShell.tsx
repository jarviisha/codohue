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
export default function AppShell() {
  return (
    <div className="min-h-screen bg-surface text-primary">
      <Sidebar />
      <div className="ml-60">
        <TopBar />
        <main className="fixed left-60 right-0 top-12 bottom-0 overflow-y-auto bg-base">
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
