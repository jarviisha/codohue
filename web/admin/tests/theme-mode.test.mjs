// Store-surface tests for src/services/themeMode.ts, run with `node --test`.
//
// The module reads localStorage at import time and caches state at module
// scope, so each test installs stubs first and then pulls a fresh copy via a
// cache-busting query string on the import specifier (Node strips the .ts
// types natively). The React half (useSyncExternalStore client path) is out
// of scope; the one render test uses react-dom/server, which needs no DOM.
import assert from 'node:assert/strict'
import test from 'node:test'

const KEY = 'codohue-admin-theme'

function stubStorage(overrides = {}) {
  const stub = {
    setItemCalls: [],
    getItem: () => null,
    setItem(key, value) {
      stub.setItemCalls.push([key, value])
    },
    ...overrides,
  }
  Object.defineProperty(globalThis, 'localStorage', { value: stub, configurable: true })
  return stub
}

let importCount = 0
function freshStore() {
  importCount += 1
  return import(`../src/services/themeMode.ts?v=${importCount}`)
}

test('readStored falls back to system for missing, garbage, and throwing storage', async () => {
  stubStorage()
  assert.equal((await freshStore()).getThemeMode(), 'system')

  stubStorage({ getItem: () => 'banana' })
  assert.equal((await freshStore()).getThemeMode(), 'system')

  stubStorage({
    getItem: () => {
      throw new Error('storage blocked')
    },
  })
  assert.equal((await freshStore()).getThemeMode(), 'system')

  // Sanity: a valid stored value is honoured, so the fallbacks above are real.
  stubStorage({ getItem: () => 'dark' })
  assert.equal((await freshStore()).getThemeMode(), 'dark')
})

test('setThemeMode is a no-op when the mode is unchanged', async () => {
  const storage = stubStorage()
  const store = await freshStore()
  let fired = 0
  store.subscribe(() => {
    fired += 1
  })

  store.setThemeMode('system')
  assert.equal(fired, 0)
  assert.equal(storage.setItemCalls.length, 0)

  store.setThemeMode('dark')
  assert.equal(fired, 1)
  assert.deepEqual(storage.setItemCalls, [[KEY, 'dark']])

  store.setThemeMode('dark')
  assert.equal(fired, 1)
  assert.equal(storage.setItemCalls.length, 1)
})

test('subscribers fire on change and unsubscribe cleanly', async () => {
  stubStorage()
  const store = await freshStore()
  let a = 0
  let b = 0
  const unsubscribeA = store.subscribe(() => {
    a += 1
  })
  store.subscribe(() => {
    b += 1
  })

  store.setThemeMode('dark')
  assert.equal(a, 1)
  assert.equal(b, 1)

  unsubscribeA()
  store.setThemeMode('light')
  assert.equal(a, 1)
  assert.equal(b, 2)
})

test('resolveThemeMode folds system against the OS preference and passes explicit modes through', async () => {
  stubStorage()
  const { resolveThemeMode } = await freshStore()

  assert.equal(resolveThemeMode('system', true), 'dark')
  assert.equal(resolveThemeMode('system', false), 'light')
  assert.equal(resolveThemeMode('dark', false), 'dark')
  assert.equal(resolveThemeMode('light', true), 'light')
})

test('useThemeMode resolves system from matchMedia in a server render', async () => {
  const { createElement } = await import('react')
  const { renderToString } = await import('react-dom/server')

  for (const matches of [true, false]) {
    stubStorage()
    Object.defineProperty(globalThis, 'window', {
      value: {
        matchMedia: () => ({ matches, addEventListener() {}, removeEventListener() {} }),
      },
      configurable: true,
    })
    const store = await freshStore()
    const Probe = () => createElement('span', null, store.useThemeMode().resolvedMode)
    assert.equal(
      renderToString(createElement(Probe)),
      `<span>${matches ? 'dark' : 'light'}</span>`,
    )
  }
})

test('setThemeMode keeps the in-memory mode when setItem throws', async () => {
  stubStorage({
    setItem: () => {
      throw new Error('storage blocked')
    },
  })
  const store = await freshStore()
  let fired = 0
  store.subscribe(() => {
    fired += 1
  })

  store.setThemeMode('dark')
  assert.equal(store.getThemeMode(), 'dark')
  assert.equal(fired, 1)
})
