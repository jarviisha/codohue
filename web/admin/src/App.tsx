import { createBrowserRouter, RouterProvider } from 'react-router-dom'
import AppShellLayout from '@/components/shell/AppShellLayout'
import { AuthGuard } from '@/components/shell/AuthGuard'
import HomePage from '@/pages/home/HomePage'
import LoginPage from '@/pages/login/LoginPage'

const router = createBrowserRouter([
  {
    path: '/login',
    element: <LoginPage />,
  },
  {
    path: '/',
    element: (
      <AuthGuard>
        <AppShellLayout />
      </AuthGuard>
    ),
    children: [{ index: true, element: <HomePage /> }],
  },
])

export default function App() {
  return <RouterProvider router={router} />
}
