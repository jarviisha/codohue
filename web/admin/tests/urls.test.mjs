import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'

const __dirname = dirname(fileURLToPath(import.meta.url))

function read(rel) {
  return readFileSync(resolve(__dirname, '..', rel), 'utf8')
}

function walk(relDir) {
  const absDir = resolve(__dirname, '..', relDir)
  const out = []
  for (const entry of readdirSync(absDir, { withFileTypes: true })) {
    const rel = `${relDir}/${entry.name}`
    if (entry.isDirectory()) {
      out.push(...walk(rel))
    } else if (/\.(ts|tsx)$/.test(entry.name)) {
      out.push(rel)
    }
  }
  return out
}

// ---------------------------------------------------------------------------
// Phase 0 smoke gate. Real route / page / command coverage tests land per
// phase as those artefacts arrive. The single invariant we want enforced from
// day one is the "all HTTP goes through services/http.ts" rule — every other
// service depends on it for the auth-expired dispatch and shared error shape.
// ---------------------------------------------------------------------------

test('HTTP calls go through services/http.ts (no raw fetch in pages or services)', () => {
  const sources = [
    ...walk('src/services').filter((rel) => rel !== 'src/services/http.ts'),
    ...walk('src/pages'),
    ...walk('src/components'),
  ]
  for (const rel of sources) {
    const src = read(rel)
    const rawFetch = /(?<![A-Za-z_])fetch\s*\(/.test(src)
    assert.ok(
      !rawFetch,
      `${rel} contains a raw fetch() call — route HTTP through services/http.ts instead`,
    )
  }
})

test('services/http.ts is the only file that calls fetch()', () => {
  const src = read('src/services/http.ts')
  assert.ok(
    /(?<![A-Za-z_])fetch\s*\(/.test(src),
    'services/http.ts must contain the canonical fetch( call',
  )
})

test('SSE connections go through services/stream.ts (no raw EventSource in pages or other services)', () => {
  const sources = [
    ...walk('src/services').filter((rel) => rel !== 'src/services/stream.ts'),
    ...walk('src/pages'),
    ...walk('src/components'),
  ]
  for (const rel of sources) {
    const src = read(rel)
    const raw = /(?<![A-Za-z_])new\s+EventSource\s*\(/.test(src)
    assert.ok(
      !raw,
      `${rel} constructs an EventSource directly — use useServerStream from services/stream.ts`,
    )
  }
})

// ---------------------------------------------------------------------------
// The shell (TopNav switcher, SideNav, command palette) renders inside the
// `path: '/'` layout route, above where `ns/:ns` is matched. React Router
// builds each route element's context from `matches.slice(0, index + 1)`, so
// useParams() there returns {} on every route — including /ns/foo. It fails
// silently: the switcher just shows no namespace and the sidebar's Namespace
// section never renders. useNamespaceParam() reads the pathname instead.
// ---------------------------------------------------------------------------

test('shell components read the namespace from the URL, not useParams', () => {
  for (const rel of walk('src/components/shell')) {
    // Comments stripped so useNamespaceParam's own doc comment, which names
    // useParams to explain why it exists, doesn't trip its own rule.
    const src = read(rel)
      .replace(/\/\*[\s\S]*?\*\//g, ' ')
      .replace(/^[ \t]*\/\/.*$/gm, ' ')
    assert.ok(
      !/(?<![A-Za-z_])useParams\s*\(/.test(src),
      `${rel} calls useParams() — the shell sits above the ns/:ns match, so it ` +
        `always resolves to {}. Use useNamespaceParam() instead.`,
    )
  }
})
