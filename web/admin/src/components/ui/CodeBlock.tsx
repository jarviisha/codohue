import { useState } from 'react'

interface CodeBlockProps {
  /** Short identifier (e.g. "json", "yaml") rendered as a mono-uppercase label in the header. Purely informational. */
  language?: string
  /** Show a copy-to-clipboard text button. */
  copyable?: boolean
  /** Maximum visible height in CSS units — content scrolls past it. */
  maxHeight?: string
  /** Pre-formatted source string. Whitespace is preserved. */
  children: string
}

// Read-only code/JSON viewer. No syntax highlighting (deliberate — keeps
// bundle small and matches the mono-text-only direction). The header strip
// renders only when a language label or copy action is present.
export default function CodeBlock({
  language,
  copyable,
  maxHeight,
  children,
}: CodeBlockProps) {
  const [copied, setCopied] = useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(children)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard API unavailable (e.g. non-secure context) — silently no-op;
      // the user can still select the text manually.
    }
  }

  const showHeader = Boolean(language) || Boolean(copyable)

  return (
    <div className="border border-default rounded-sm overflow-hidden">
      {showHeader ? (
        <div className="flex items-center justify-between px-3 py-1.5 border-b border-default bg-surface-raised">
          {language ? (
            <span className="font-mono text-xs uppercase tracking-[0.04em] text-secondary">
              {language}
            </span>
          ) : (
            <span />
          )}
          {copyable ? (
            <button
              type="button"
              onClick={copy}
              className="font-mono text-xs uppercase tracking-[0.04em] text-secondary hover:text-primary"
              aria-label="Copy to clipboard"
            >
              {copied ? 'copied' : 'copy'}
            </button>
          ) : null}
        </div>
      ) : null}
      <pre
        className="overflow-auto p-4 m-0"
        style={maxHeight ? { maxHeight } : undefined}
      >
        <code className="font-mono text-sm text-primary whitespace-pre">
          {children}
        </code>
      </pre>
    </div>
  )
}
