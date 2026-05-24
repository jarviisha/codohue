import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ThemeProvider } from '@jarviisha/davinci-react-theme-provider'
import App from '@/App'
import '@/index.css'

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Default retry off for 4xx — auth failures should bubble immediately.
      // Real services override retry on a per-query basis when appropriate.
      retry: false,
      refetchOnWindowFocus: false,
    },
  },
})

const root = document.getElementById('root')
if (!root) throw new Error('missing #root in index.html')

createRoot(root).render(
  <StrictMode>
    <ThemeProvider defaultTheme="system" storageKey="codohue-admin-theme">
      <QueryClientProvider client={queryClient}>
        <App />
      </QueryClientProvider>
    </ThemeProvider>
  </StrictMode>,
)
