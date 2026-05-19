import { test } from 'node:test'
import assert from 'node:assert/strict'
import { readdirSync, readFileSync } from 'node:fs'
import { fileURLToPath } from 'node:url'
import { dirname, resolve } from 'node:path'
import { parsePs1, formatPs1, segmentTo } from '../src/routes/ps1.mjs'

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

const routesSrc = read('src/routes/index.tsx')
const pathSrc = read('src/routes/path.ts')

// ────────────────────────────────────────────────────────────────────────────
// Route parser
//
// Auto-derives ROUTE_PATHS and route-element → module-file mappings from
// src/routes/index.tsx so adding or removing a route does not silently age
// the test. Static and lazy-imported pages are both recognised; layout-only
// files (pure <Outlet />) and the not-found fallback opt out of the
// command-registration check via COMMAND_OPTOUT.
// ────────────────────────────────────────────────────────────────────────────

function parseRouteFile(src) {
  const imports = new Map() // ComponentName -> src/pages/... .tsx

  for (const m of src.matchAll(/^import\s+(\w+)\s+from\s+'@\/pages\/([^']+)'/gm)) {
    imports.set(m[1], `src/pages/${m[2]}.tsx`)
  }
  // Match `const X = ...lazy(() => import('@/pages/...'))...`. The lazy `[\s\S]*?`
  // accepts intervening ternaries (e.g. `import.meta.env.DEV ? lazy(...) : null`)
  // and multi-line wrapping while still binding to the nearest `const X =`.
  for (const m of src.matchAll(/\b(?:const|let|var)\s+(\w+)\s*=[\s\S]*?lazy\(\s*\(\s*\)\s*=>\s*import\(\s*'@\/pages\/([^']+)'\s*\)\s*\)/g)) {
    imports.set(m[1], `src/pages/${m[2]}.tsx`)
  }

  // Every "path=..." declaration. Used for the human-readable contract test;
  // also lets us cross-check that paths.ts knows the builder for each one.
  const paths = []
  for (const m of src.matchAll(/path="([^"]+)"/g)) {
    paths.push(m[1])
  }

  // JSX self-closing component usages whose name resolves to one of the
  // imports above. This is what `element={<XxxPage />}` (and `element={...
  // <XxxPage /> ...}` for Suspense wrappers) reduces to.
  const usedComponents = new Set()
  for (const m of src.matchAll(/<(\w+)\s*\/>/g)) {
    if (imports.has(m[1])) usedComponents.add(m[1])
  }

  const routeModules = new Set()
  for (const name of usedComponents) {
    routeModules.add(imports.get(name))
  }

  return { paths, routeModules }
}

const parsed = parseRouteFile(routesSrc)

// Layout-only and fallback pages that have no business-action surface of
// their own — palette commands belong on their parents or children instead.
// LoginPage opts out too: CommandPalette only mounts inside AppShell, so
// the palette is unreachable from /login by design.
const COMMAND_OPTOUT = new Set([
  'src/pages/login/LoginPage.tsx',
  'src/pages/ns/NamespaceLayout.tsx',
  'src/pages/ns/batch-runs/BatchRunsLayout.tsx',
  'src/pages/not-found/Page.tsx',
  // Dev-only design surface; tree-shaken out of production.
  'src/pages/_kitchen-sink/Page.tsx',
])

test('routes/index.tsx still declares every documented route path', () => {
  // Anchor a minimum set so a stray rename or path drop fails the test.
  // We don't need the full enumeration here — the heavier coverage comes
  // from path.ts builder + command-registration tests below.
  const MUST_HAVE = [
    '/login',
    'namespaces',
    'namespaces/new',
    'ns/:name',
    'catalog',
    'items',
    ':id',
    'events',
    'trending',
    'batch-runs',
    're-embeds',
    'debug',
    'demo-data',
    '*',
  ]
  const declared = new Set(parsed.paths)
  for (const expected of MUST_HAVE) {
    assert.ok(
      declared.has(expected),
      `routes/index.tsx is missing path="${expected}"`,
    )
  }
})

const EXPECTED_BUILDERS = [
  "login: '/login'",
  "health: '/'",
  "namespaces: '/namespaces'",
  "namespaceCreate: '/namespaces/new'",
  '`/ns/${name}`',
  '`/ns/${name}/config`',
  '`/ns/${name}/catalog`',
  '`/ns/${name}/catalog/items`',
  '`/ns/${name}/catalog/items/${id}`',
  '`/ns/${name}/events`',
  '`/ns/${name}/trending`',
  '`/ns/${name}/batch-runs`',
  '`/ns/${name}/debug`',
  '`/ns/${name}/demo-data`',
]

test('path.ts exposes every URL builder', () => {
  for (const b of EXPECTED_BUILDERS) {
    assert.ok(pathSrc.includes(b), `path.ts is missing builder: ${b}`)
  }
})

test('HTTP calls go through services/http.ts (no raw fetch in pages or services)', () => {
  const sources = [
    ...walk('src/services').filter((rel) => rel !== 'src/services/http.ts'),
    ...walk('src/pages'),
  ]
  for (const rel of sources) {
    const src = read(rel)
    // Allow the word "fetch" inside comments / strings but flag actual call sites.
    const rawFetch = /(?<![A-Za-z_])fetch\s*\(/.test(src)
    assert.ok(
      !rawFetch,
      `${rel} contains a raw fetch() call — route HTTP through services/http.ts instead`,
    )
  }
})

test('every route-element page registers at least one command', () => {
  const checked = []
  for (const rel of parsed.routeModules) {
    if (COMMAND_OPTOUT.has(rel)) continue
    const src = read(rel)
    assert.ok(
      src.includes('useRegisterCommand('),
      `${rel} does not register a command palette entry`,
    )
    checked.push(rel)
  }
  // Sanity: catch regressions where the parser silently empties the set
  // (e.g. someone refactors routes/index.tsx into a format we cannot read).
  assert.ok(
    checked.length >= 10,
    `route parser yielded only ${checked.length} module(s); the parser likely regressed`,
  )
})

// ────────────────────────────────────────────────────────────────────────────
// PS1 prompt smoke tests
//
// BUILD_PLAN.md §4.8 calls for a smoke that asserts
// `(namespace, pathSegments) -> "codohue@{ns}:~/{path} $"`. The renderer in
// Ps1Prompt.tsx splits the line across spans + Links for clickable segments,
// so we test the parallel formatPs1 helper that produces the same string —
// any drift in either direction surfaces here.
// ────────────────────────────────────────────────────────────────────────────

test('parsePs1 splits pathname into ns + segments', () => {
  assert.deepEqual(parsePs1('/'), { ns: '~', segments: [] })
  assert.deepEqual(parsePs1('/namespaces'), { ns: '~', segments: ['namespaces'] })
  assert.deepEqual(parsePs1('/namespaces/new'), { ns: '~', segments: ['namespaces', 'new'] })
  assert.deepEqual(parsePs1('/ns/prod'), { ns: 'prod', segments: [] })
  assert.deepEqual(parsePs1('/ns/prod/events'), { ns: 'prod', segments: ['events'] })
  assert.deepEqual(parsePs1('/ns/prod/catalog/items'), {
    ns: 'prod',
    segments: ['catalog', 'items'],
  })
  assert.deepEqual(parsePs1('/ns/prod/catalog/items/sku_42'), {
    ns: 'prod',
    segments: ['catalog', 'items', 'sku_42'],
  })
  // Trailing slash should not produce an empty segment.
  assert.deepEqual(parsePs1('/ns/prod/'), { ns: 'prod', segments: [] })
  // A bare `/ns` without a name is global, not namespace-scoped.
  assert.deepEqual(parsePs1('/ns'), { ns: '~', segments: ['ns'] })
})

test('formatPs1 renders the canonical shell prompt line', () => {
  // Empty path — global root vs namespace root.
  assert.equal(formatPs1('~', []), 'codohue@~:~ $')
  assert.equal(formatPs1('prod', []), 'codohue@prod:~ $')
  // Single segment.
  assert.equal(formatPs1('~', ['namespaces']), 'codohue@~:~/namespaces $')
  assert.equal(formatPs1('prod', ['events']), 'codohue@prod:~/events $')
  // Deep path.
  assert.equal(
    formatPs1('prod', ['catalog', 'items']),
    'codohue@prod:~/catalog/items $',
  )
  assert.equal(
    formatPs1('prod', ['catalog', 'items', 'sku_42']),
    'codohue@prod:~/catalog/items/sku_42 $',
  )
})

test('formatPs1 is consistent with parsePs1 for real route paths', () => {
  const cases = [
    '/',
    '/namespaces',
    '/namespaces/new',
    '/ns/prod',
    '/ns/prod/events',
    '/ns/prod/catalog',
    '/ns/prod/catalog/items',
    '/ns/prod/catalog/items/sku_42',
    '/ns/prod/batch-runs/re-embeds',
  ]
  for (const pathname of cases) {
    const { ns, segments } = parsePs1(pathname)
    // The PS1 line is purely cosmetic — no hard assertion on which fragment
    // ends up where. The contract is: ns matches what parsePs1 picked, and
    // every segment shows up verbatim in the rendered line.
    const line = formatPs1(ns, segments)
    assert.match(line, /^codohue@/)
    assert.match(line, / \$$/)
    for (const seg of segments) {
      assert.ok(line.includes(`/${seg}`), `segment ${seg} missing from ${line}`)
    }
  }
})

test('segmentTo builds clickable URLs for each PS1 segment', () => {
  // Global block: segments live at top-level paths.
  assert.equal(segmentTo('~', ['namespaces'], 0), '/namespaces')
  assert.equal(segmentTo('~', ['namespaces', 'new'], 1), '/namespaces/new')
  // Namespace block: segments live under /ns/{name}/.
  assert.equal(segmentTo('prod', ['events'], 0), '/ns/prod/events')
  assert.equal(segmentTo('prod', ['catalog', 'items'], 0), '/ns/prod/catalog')
  assert.equal(segmentTo('prod', ['catalog', 'items'], 1), '/ns/prod/catalog/items')
})
