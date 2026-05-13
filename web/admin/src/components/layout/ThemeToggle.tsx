import { useEffect, useState } from 'react'

type Theme = 'light' | 'dark'

function readInitialTheme(): Theme {
  const saved = localStorage.getItem('theme')
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(theme: Theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
}

export default function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(() => readInitialTheme())

  useEffect(() => {
    applyTheme(theme)
    localStorage.setItem('theme', theme)
  }, [theme])

  return (
    <button
      type="button"
      onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
      className="h-7 w-7 flex items-center justify-center rounded-sm text-muted hover:text-primary hover:bg-surface-raised font-mono text-sm"
      aria-label={`Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`}
      title={`Switch to ${theme === 'dark' ? 'light' : 'dark'} theme`}
    >
      {theme === 'dark' ? '☼' : '☾'}
    </button>
  )
}
