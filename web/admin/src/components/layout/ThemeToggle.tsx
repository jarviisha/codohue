import { useEffect, useState } from 'react'
import { Button } from '../ui'

type Theme = 'light' | 'dark'

function readInitialTheme(): Theme {
  const saved = localStorage.getItem('theme')
  if (saved === 'light' || saved === 'dark') return saved
  return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
}

function applyTheme(theme: Theme) {
  document.documentElement.classList.toggle('dark', theme === 'dark')
}

// Icons deferred — the button label shows the current theme as plain text.
export default function ThemeToggle() {
  const [theme, setTheme] = useState<Theme>(() => readInitialTheme())

  useEffect(() => {
    applyTheme(theme)
    localStorage.setItem('theme', theme)
  }, [theme])

  const next: Theme = theme === 'dark' ? 'light' : 'dark'

  return (
    <Button
      type="button"
      size='xs'
      onClick={() => setTheme(next)}
      aria-label={`Switch to ${next} theme`}
      title={`Switch to ${next} theme (currently ${theme})`}
    >
      {theme}
    </Button>
  )
}
