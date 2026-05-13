import { Outlet } from 'react-router-dom'
import Sidebar from './Sidebar'
import TopBar from './TopBar'
import CommandPalette from '../ui/CommandPalette'

// Top-level shell: fixed Sidebar (240px) + fixed TopBar (48px) + scrolling
// content. Login route renders outside this shell.
//
// CommandPalette is mounted here so the Cmd+K listener is active on every
// shell page; the palette modal itself only renders when open.
export default function AppShell() {
  return (
    <div className="min-h-screen bg-base text-primary">
      <Sidebar />
      <div className="ml-60">
        <TopBar />
        <main className="pt-12">
          <div className="px-6 py-6">
            <Outlet />
          </div>
        </main>
      </div>
      <CommandPalette />
    </div>
  )
}
