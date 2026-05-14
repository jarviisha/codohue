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

const routesSrc = read('src/routes/index.tsx')
const pathSrc = read('src/routes/path.ts')

// Each entry is the literal Route declaration string expected in routes/index.tsx.
// Updated alongside any route change so the URL contract stays grep-verifiable.
const ROUTE_PATHS = [
  'path="/login"',
  'path="namespaces"',
  'path="namespaces/new"',
  'path="ns/:name"',
  'path="config"',
  'path="catalog"',
  'path="catalog/items"',
  'path=":id"',
  'path="events"',
  'path="trending"',
  'path="batch-runs"',
  'path="debug"',
  'path="demo-data"',
]

test('routes/index.tsx declares every BUILD_PLAN route', () => {
  for (const route of ROUTE_PATHS) {
    assert.ok(
      routesSrc.includes(route),
      `routes/index.tsx is missing ${route}`,
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

const COMMAND_PAGE_MODULES = [
  'src/pages/health/HealthPage.tsx',
  'src/pages/namespaces/ListPage.tsx',
  'src/pages/namespaces/CreatePage.tsx',
  'src/pages/ns/OverviewPage.tsx',
  'src/pages/ns/ConfigPage.tsx',
  'src/pages/ns/catalog/ConfigPage.tsx',
  'src/pages/ns/catalog/items/ListPage.tsx',
  'src/pages/ns/catalog/items/DetailModal.tsx',
]

test('implemented shell pages register at least one command', () => {
  for (const rel of COMMAND_PAGE_MODULES) {
    const src = read(rel)
    assert.ok(
      src.includes('useRegisterCommand('),
      `${rel} does not register a command palette entry`,
    )
  }
})
