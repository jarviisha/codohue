import { Outlet } from 'react-router-dom'
import Sidebar from './Sidebar'
import TopBar from './TopBar'

// Top-level shell: fixed Sidebar (240px) + fixed TopBar (48px) + scrolling
// content. Login route renders outside this shell.
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
    </div>
  )
}
