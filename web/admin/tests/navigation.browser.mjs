import assert from 'node:assert/strict'
import { chromium } from 'playwright'
import { preview } from 'vite'

/**
 * Navigation feedback regression (issue #68).
 *
 * The defect was invisible to API fixtures: every admin request was already
 * fast, and the gap the operator saw was the destination's *JavaScript chunk*
 * arriving late. So this suite holds a real lazy-loaded module response and
 * asserts on what the shell does while it is in flight.
 *
 * The 2-second hold is injected, not a measured production download time. It
 * stands in for any slow first load — cold cache, bad network, a large page —
 * and makes the pending window long enough to assert on deterministically.
 */
const HOLD_MS = 2_000

const server = process.env.ADMIN_TEST_URL
  ? null
  : await preview({ preview: { host: '127.0.0.1', port: 4176, strictPort: true } })
const browser = await chromium.launch({
  executablePath: process.env.BROWSER_EXECUTABLE || undefined,
  headless: true,
})
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
  reducedMotion: 'reduce',
})
const page = await context.newPage()
page.setDefaultTimeout(15_000)
page.setDefaultNavigationTimeout(15_000)
const origin = process.env.ADMIN_TEST_URL || 'http://127.0.0.1:4176'
const pageErrors = []
page.on('pageerror', (error) => pageErrors.push(error.message))

const OBSERVED_AT = '2026-09-18T00:00:00Z'

await context.route('**/api/**', async (route) => {
  const path = new URL(route.request().url()).pathname
  const json = (body, status = 200) => route.fulfill({ status, json: body })
  if (path.endsWith('/stream'))
    return route.fulfill({ contentType: 'text/event-stream', body: ': connected\n\n' })
  if (path.endsWith('/sessions/current')) return json({ actor: { name: 'Test operator' } })
  if (path === '/api/admin/v1/namespaces') return json({ items: [], total: 0 })
  if (path === '/api/admin/v1/runtime')
    return json({ observed_at: OBSERVED_AT, expiry_seconds: 120, processes: [] })
  if (path.endsWith('/health'))
    return json({ status: 'ok', postgres: 'ok', redis: 'ok', qdrant: 'ok' })
  return json({ error: { message: 'Fixture intentionally unavailable' } }, 404)
})

/**
 * Chunk interception. `mode` is re-read on every request so a single route
 * registration can hold, fail, or pass through depending on the scenario.
 * Requests are recorded so the preload assertions can prove a module was
 * fetched without a navigation.
 */
let chunkMode = 'pass'
/** Which page module `chunkMode` applies to. */
let chunkTarget = 'RuntimePage-'
const chunkRequests = []
await context.route('**/assets/*.js', async (route) => {
  const file = new URL(route.request().url()).pathname.split('/').pop()
  chunkRequests.push(file)
  if (chunkMode === 'hold' && file.startsWith(chunkTarget)) {
    await new Promise((resolve) => setTimeout(resolve, HOLD_MS))
    return route.continue()
  }
  if (chunkMode === 'fail' && file.startsWith(chunkTarget)) {
    return route.abort('failed')
  }
  return route.continue()
})

const goto = (path) => page.goto(`${origin}${path}`)
const sidebar = () => page.getByRole('navigation', { name: 'Main navigation', exact: true })
const runtimeLink = () => sidebar().getByRole('link', { name: 'System runtime', exact: true })

try {
  // ---------------------------------------------------------------------
  // A cold visit paints the shell first. Route-level `lazy` resolves the
  // entered route's module before that route renders, which is what makes the
  // download part of the navigation — and is also what will blank the whole
  // page if the hydrate fallback sits on the root route instead of the child.
  // ---------------------------------------------------------------------
  chunkMode = 'hold'
  chunkTarget = 'HealthPage-'
  const coldStartedAt = Date.now()
  const coldLoad = goto('/health')
  // The shell must be up well before the held page module lands.
  await sidebar().getByRole('link', { name: 'System runtime', exact: true }).waitFor()
  const shellAt = Date.now() - coldStartedAt
  assert.equal(
    await page.getByRole('heading', { name: 'Service health', exact: true }).count(),
    0,
    'the entered page must still be waiting while the shell is already painted',
  )
  await coldLoad
  await page.getByRole('heading', { name: 'Service health', exact: true }).waitFor()
  const coldReadyAt = Date.now() - coldStartedAt
  assert.ok(
    shellAt < coldReadyAt,
    `shell painted at ${shellAt}ms, page at ${coldReadyAt}ms — the shell must not wait on the page module`,
  )
  console.log(
    `PASS a cold visit paints the shell at ${shellAt}ms while the entered page module is held until ${coldReadyAt}ms`,
  )
  chunkMode = 'pass'
  chunkTarget = 'RuntimePage-'

  // ---------------------------------------------------------------------
  // A held destination chunk must produce prompt, accessible pending state.
  // ---------------------------------------------------------------------
  await goto('/health')
  await page.getByRole('heading', { name: 'Service health', exact: true }).waitFor()

  chunkMode = 'hold'
  const startedAt = Date.now()
  // dispatchEvent rather than click(): a real click moves the pointer over the
  // link first, which would preload the chunk and dissolve the very wait this
  // scenario exists to observe.
  await runtimeLink().dispatchEvent('click')

  // The selected destination moves on the click, not on arrival — the original
  // report was that the previous entry stayed highlighted and the click looked
  // dropped.
  await page.waitForFunction(
    () => {
      const nav = document.querySelector('nav[aria-label="Main navigation"]')
      const link = [...(nav?.querySelectorAll('a') ?? [])].find(
        (a) => a.textContent?.trim() === 'System runtime',
      )
      return link?.getAttribute('aria-current') === 'page' || link?.dataset.selected === 'true'
    },
    null,
    { timeout: 1_000 },
  )
  const selectionMovedAfter = Date.now() - startedAt

  // aria-busy marks the region whose content is still being fetched.
  await page.waitForSelector('[aria-busy="true"]', { timeout: 1_000 })

  // And the bar itself, which carries the accessible name of the destination.
  const bar = page.getByRole('progressbar', { name: 'Loading runtime', exact: true })
  await bar.waitFor({ timeout: 2_000 })
  const feedbackAfter = Date.now() - startedAt

  // Still pending: the old page is what is rendered, but nothing claims it is
  // the current selection any more.
  assert.equal(
    await page.getByRole('heading', { name: 'System runtime', exact: true }).count(),
    0,
    'destination must not have rendered while its chunk is held',
  )

  await page.getByRole('heading', { name: 'System runtime', exact: true }).waitFor()
  const arrivedAfter = Date.now() - startedAt

  // The pending signal must land far closer to the click than to the arrival.
  assert.ok(
    feedbackAfter < 1_000,
    `pending feedback took ${feedbackAfter}ms, expected well under the ${HOLD_MS}ms hold`,
  )
  assert.ok(
    arrivedAfter >= HOLD_MS,
    `destination arrived after ${arrivedAfter}ms, expected at least the ${HOLD_MS}ms hold`,
  )
  // Feedback cleared on arrival rather than sticking around.
  await bar.waitFor({ state: 'detached' })
  assert.equal(await page.locator('[aria-busy="true"]').count(), 0)
  console.log(
    `PASS held chunk shows pending selection at ${selectionMovedAfter}ms and a labelled progressbar at ${feedbackAfter}ms, destination at ${arrivedAfter}ms`,
  )

  // ---------------------------------------------------------------------
  // Hover preloads the destination module — without navigating or calling the
  // API.
  // ---------------------------------------------------------------------
  chunkMode = 'pass'
  await goto('/health')
  await page.getByRole('heading', { name: 'Service health', exact: true }).waitFor()

  const apiCalls = []
  page.on('request', (r) => {
    if (r.url().includes('/api/')) apiCalls.push(r.url())
  })
  chunkRequests.length = 0
  await sidebar().getByRole('link', { name: 'Demo data', exact: true }).hover()
  await page.waitForFunction(
    () => performance.getEntriesByType('resource').some((e) => e.name.includes('DemoDataPage-')),
    null,
    { timeout: 5_000 },
  )
  assert.ok(
    chunkRequests.some((f) => f.startsWith('DemoDataPage-')),
    `hover should fetch the destination module, saw ${JSON.stringify(chunkRequests)}`,
  )
  assert.ok(page.url().endsWith('/health'), `hover must not navigate, at ${page.url()}`)
  assert.deepEqual(apiCalls, [], 'hover must not issue API requests')

  // Keyboard focus is the same signal of intent as hover.
  chunkRequests.length = 0
  await sidebar().getByRole('link', { name: 'Danger zone', exact: true }).focus()
  await page.waitForFunction(
    () => performance.getEntriesByType('resource').some((e) => e.name.includes('DangerZonePage-')),
    null,
    { timeout: 5_000 },
  )
  assert.ok(page.url().endsWith('/health'), 'focus must not navigate')
  console.log('PASS hover and focus preload the destination module without navigating or fetching data')

  // A preloaded destination still navigates, by keyboard, and lands.
  await sidebar().getByRole('link', { name: 'Demo data', exact: true }).focus()
  await page.keyboard.press('Enter')
  await page.waitForURL('**/demo-data')
  console.log('PASS keyboard activation navigates to a preloaded destination')

  // ---------------------------------------------------------------------
  // A chunk that never arrives gets an actionable recovery, inside the shell.
  // ---------------------------------------------------------------------
  await goto('/health')
  await page.getByRole('heading', { name: 'Service health', exact: true }).waitFor()
  chunkMode = 'fail'
  await runtimeLink().dispatchEvent('click')
  await page.getByText('Could not load this page', { exact: true }).waitFor()
  await page.getByRole('button', { name: 'Reload page', exact: true }).waitFor()
  await page.getByRole('button', { name: 'Back to fleet', exact: true }).waitFor()
  // The shell survives: navigation is still there to escape with.
  assert.equal(await runtimeLink().count(), 1, 'sidebar must survive a failed chunk load')
  console.log('PASS a failed chunk renders an actionable recovery without losing the shell')

  // Leaving the failed route recovers without a reload.
  chunkMode = 'pass'
  await sidebar().getByRole('link', { name: 'Health', exact: true }).click()
  await page.getByRole('heading', { name: 'Service health', exact: true }).waitFor()

  // ---------------------------------------------------------------------
  // History still behaves.
  // ---------------------------------------------------------------------
  await sidebar().getByRole('link', { name: 'Demo data', exact: true }).click()
  await page.waitForURL('**/demo-data')
  await page.goBack()
  await page.waitForURL('**/health')
  await page.getByRole('heading', { name: 'Service health', exact: true }).waitFor()
  await page.goForward()
  await page.waitForURL('**/demo-data')
  console.log('PASS back and forward navigate through lazily-loaded routes')

  assert.deepEqual(pageErrors, [])
  console.log('PASS no uncaught browser errors')
} finally {
  await browser.close()
  await new Promise((resolve) => (server ? server.httpServer.close(resolve) : resolve()))
}
