import { useEffect, useState } from 'react'
import Icon from './Icon'

export default function ThemeToggle() {
  const [dark, setDark] = useState(() => {
    const savedTheme = localStorage.getItem('theme')
    if (savedTheme === 'dark') return true
    if (savedTheme === 'light') return false
    return document.documentElement.classList.contains('dark')
  })

  useEffect(() => {
    if (dark) {
      document.documentElement.classList.add('dark')
      localStorage.setItem('theme', 'dark')
    } else {
      document.documentElement.classList.remove('dark')
      localStorage.setItem('theme', 'light')
    }
  }, [dark])

  return (
    <button
      type="button"
      role="switch"
      aria-checked={dark}
      aria-label={dark ? 'Switch to light mode' : 'Switch to dark mode'}
      onClick={() => setDark(value => !value)}
      className="fixed bottom-5 right-5 z-50 inline-flex h-8 w-15 items-center rounded border border-default bg-surface p-1 text-secondary shadow-floating transition-colors duration-200 hover:border-strong hover:text-primary focus-visible:outline-none focus-visible:shadow-focus"
    >
      <span
        aria-hidden="true"
        className={`flex size-6 items-center justify-center rounded bg-surface-raised text-primary shadow-raised transition-transform duration-200 ${dark ? 'translate-x-7' : 'translate-x-0'}`}
      >
        {dark ? <Icon name="moon" size={14} /> : <Icon name="sun" size={14} />}
      </span>
    </button>
  )
}
