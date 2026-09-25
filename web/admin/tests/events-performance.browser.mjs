import assert from 'node:assert/strict'
import { chromium } from 'playwright'
import { preview } from 'vite'

/**
 * Live tail bounding regression (issue #69).
 *
 * The defect was that a populated tail rendered every retained event and kept
 * one flash timer per row, so a busy namespace blocked the main thread and
 * made the rest of the console feel slow. API fixtures cannot exercise that —
 * the cost was in what arrived over SSE and what the component did with it —
 * so this suite drives a fake EventSource and asserts on structure: how many
 * rows reach the DOM, and how many timers get scheduled.
 *
 * Structure, not milliseconds. Wall-clock numbers are reported for the record
 * but not asserted on: they track whatever machine CI happens to run.
 */
const BURST = 1_000
/** Mirrors WINDOW_SIZE in EventsPage.tsx. */
const WINDOW_SIZE = 60

const server = process.env.ADMIN_TEST_URL
  ? null
  : await preview({ preview: { host: '127.0.0.1', port: 4178, strictPort: true } })
const browser = await chromium.launch({
  executablePath: process.env.BROWSER_EXECUTABLE || undefined,
  headless: true,
})
const context = await browser.newContext({
  viewport: { width: 1440, height: 1000 },
  reducedMotion: 'reduce',
})

/**
 * Replaces EventSource with a hand-driven stub and counts timer scheduling.
 * Installed before any app script so the component sees only these.
 */
await context.addInitScript(() => {
  window.__timers = { timeout: 0, interval: 0 }
  // Intervals that were scheduled and not yet cleared. A flash timer per row
  // would show up here as growth that never comes back down.
  window.__liveIntervals = new Set()
  const realSetTimeout = window.setTimeout
  const realSetInterval = window.setInterval
  const realClearInterval = window.clearInterval
  window.setTimeout = function (...args) {
    window.__timers.timeout++
    return realSetTimeout.apply(this, args)
  }
  window.setInterval = function (...args) {
    window.__timers.interval++
    const handle = realSetInterval.apply(this, args)
    window.__liveIntervals.add(handle)
    return handle
  }
  window.clearInterval = function (handle) {
    window.__liveIntervals.delete(handle)
    return realClearInterval.call(this, handle)
  }

  window.__longTasks = []
  try {
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) window.__longTasks.push(entry.duration)
    }).observe({ entryTypes: ['longtask'] })
  } catch {
    // Long-task timing is reported, never asserted on; skip where unsupported.
  }

  window.__streams = []
  class FakeEventSource {
    constructor(url) {
      this.url = url
      this.listeners = {}
      this.closed = false
      window.__streams.push(this)
      realSetTimeout(() => this.onopen && this.onopen(), 0)
    }
    addEventListener(name, fn) {
      ;(this.listeners[name] ||= []).push(fn)
    }
    removeEventListener(name, fn) {
      this.listeners[name] = (this.listeners[name] || []).filter((f) => f !== fn)
    }
    close() {
      this.closed = true
    }
    emit(name, data) {
      for (const fn of this.listeners[name] || []) fn({ data: JSON.stringify(data) })
    }
  }
  window.EventSource = FakeEventSource

  window.__nextId = 1
  // The shell keeps its own subscriptions (ops toasts, re-embed progress) open
  // across navigations, so every assertion about the tail has to select the
  // tail's own stream rather than "the latest one".
  window.__tails = () => window.__streams.filter((s) => s.url.includes('/events/stream'))
  window.__live = () => window.__tails().filter((s) => !s.closed).at(-1)
  window.__emit = (count) => {
    const stream = window.__live()
    for (let i = 0; i < count; i++) {
      const id = window.__nextId++
      stream.emit('event', {
        id,
        namespace: 'a',
        subject_id: `user-${id}`,
        object_id: `item-${id}`,
        action: 'VIEW',
        weight: 1,
        occurred_at: new Date(1_800_000_000_000 + id * 1_000).toISOString(),
      })
    }
  }
})

const page = await context.newPage()
page.setDefaultTimeout(15_000)
const origin = process.env.ADMIN_TEST_URL || 'http://127.0.0.1:4178'
const pageErrors = []
page.on('pageerror', (error) => pageErrors.push(error.message))

await context.route('**/api/**', async (route) => {
  const path = new URL(route.request().url()).pathname
  const json = (body, status = 200) => route.fulfill({ status, json: body })
  if (path.endsWith('/sessions/current')) return json({ actor: { name: 'Test operator' } })
  if (path === '/api/admin/v1/namespaces') return json({ items: [], total: 0 })
  if (path.endsWith('/events/summary'))
    return json({
      total: 0,
      rate_per_second: 0,
      window_seconds: 60,
      by_action: [],
      series: [],
    })
  if (path.endsWith('/health'))
    return json({ status: 'ok', postgres: 'ok', redis: 'ok', qdrant: 'ok' })
  return json({ error: { message: 'Fixture intentionally unavailable' } }, 404)
})

const rowCount = () => page.locator('table[aria-label="Live events"] tbody tr').count()
const timers = () => page.evaluate(() => ({ ...window.__timers }))
const emit = async (n) => page.evaluate((count) => window.__emit(count), n)
/** Lets the component's 100ms flush timer run and the DOM settle. */
const settle = () => page.waitForTimeout(400)

try {
  await page.goto(`${origin}/ns/a/events`)
  await page.getByRole('heading', { name: 'Events', exact: true }).waitFor()
  await page.getByText('Waiting for events', { exact: true }).waitFor()

  // Baseline before any event arrives: the tail is mounted but idle, so it
  // holds no sweep. Leaving the page later must return to this.
  const baselineIntervals = await page.evaluate(() => window.__liveIntervals.size)

  // ---------------------------------------------------------------------
  // A 1000-event burst must stay bounded in both rows and timers.
  // ---------------------------------------------------------------------
  const before = await timers()
  const startedAt = Date.now()
  await emit(BURST)
  await page.locator('table[aria-label="Live events"]').waitFor()
  await settle()
  const populatedAfter = Date.now() - startedAt
  const after = await timers()

  const rows = await rowCount()
  assert.ok(
    rows > 0 && rows <= WINDOW_SIZE,
    `rendered ${rows} rows for ${BURST} events, expected at most ${WINDOW_SIZE}`,
  )

  const scheduled = after.timeout - before.timeout + (after.interval - before.interval)
  assert.ok(
    scheduled < BURST / 10,
    `scheduled ${scheduled} timers for ${BURST} events; a per-row timer would schedule at least ${BURST}`,
  )

  // The whole history is retained and reported, not just what is drawn.
  const table = page.locator('table[aria-label="Live events"]')
  assert.equal(await table.getAttribute('aria-rowcount'), String(BURST + 1))
  await page
    .getByText(`Showing the newest ${WINDOW_SIZE} — live, newest first`, { exact: true })
    .waitFor()

  // Newest first: the top row is the last event emitted, and its ARIA row
  // index is the first body row.
  const firstRow = page.locator('table[aria-label="Live events"] tbody tr').first()
  assert.equal(await firstRow.getAttribute('aria-rowindex'), '2')
  await firstRow.getByText(`user-${BURST}`, { exact: true }).waitFor()

  const longest = Math.max(0, ...(await page.evaluate(() => window.__longTasks)))
  console.log(
    `PASS ${BURST}-event burst renders ${rows} rows (cap ${WINDOW_SIZE}) and schedules ${scheduled} timers — populated in ${populatedAfter}ms, longest long task ${Math.round(longest)}ms`,
  )

  // ---------------------------------------------------------------------
  // Sustained arrivals across many ticks stay bounded too.
  // ---------------------------------------------------------------------
  const beforeSustained = await timers()
  for (let i = 0; i < 10; i++) {
    await emit(50)
    await page.waitForTimeout(120)
  }
  await settle()
  const sustained = await timers()
  const sustainedRows = await rowCount()
  assert.ok(
    sustainedRows > 0 && sustainedRows <= WINDOW_SIZE,
    `sustained traffic rendered ${sustainedRows} rows`,
  )
  const sustainedTimers =
    sustained.timeout - beforeSustained.timeout + (sustained.interval - beforeSustained.interval)
  assert.ok(
    sustainedTimers < 500,
    `sustained traffic scheduled ${sustainedTimers} timers for 500 events`,
  )
  // History is capped, not unbounded.
  assert.equal(await table.getAttribute('aria-rowcount'), String(BURST + 1))
  console.log(
    `PASS 500 events over 10 ticks render ${sustainedRows} rows and schedule ${sustainedTimers} timers, history capped at ${BURST}`,
  )

  // ---------------------------------------------------------------------
  // Pause wins the race against the flush timer: arrivals collected in the
  // ≤100ms before the click buffer instead of landing in the paused table.
  // Emitting and clicking in one synchronous task guarantees the flush timer
  // has not fired in between.
  // ---------------------------------------------------------------------
  const topBeforeRace = await page
    .locator('table[aria-label="Live events"] tbody tr')
    .first()
    .innerText()
  await page.evaluate(() => {
    window.__emit(30)
    const pause = [...document.querySelectorAll('button')].find(
      (b) => b.textContent.trim() === 'Pause',
    )
    if (!pause) throw new Error('Pause button not found')
    pause.click()
  })
  await settle()
  await page.getByRole('button', { name: 'Resume (30)', exact: true }).waitFor()
  const topAfterRace = await page
    .locator('table[aria-label="Live events"] tbody tr')
    .first()
    .innerText()
  assert.equal(topAfterRace, topBeforeRace, 'in-flight arrivals must not move a paused table')
  await page.getByRole('button', { name: /^Resume/ }).click()
  await settle()
  console.log('PASS pausing mid-tick buffers in-flight arrivals instead of shifting the paused table')

  // ---------------------------------------------------------------------
  // Pause buffers, resume flushes — without a timer per buffered row.
  // ---------------------------------------------------------------------
  await page.getByRole('button', { name: 'Pause', exact: true }).click()
  await emit(300)
  await settle()
  await page.getByRole('button', { name: 'Resume (300)', exact: true }).waitFor()
  // Paused means paused: buffered arrivals are not in the table yet.
  const topWhilePaused = await page
    .locator('table[aria-label="Live events"] tbody tr')
    .first()
    .innerText()
  assert.ok(!topWhilePaused.includes(`user-${await page.evaluate(() => window.__nextId - 1)}`))

  const beforeResume = await timers()
  await page.getByRole('button', { name: /^Resume/ }).click()
  await settle()
  const afterResume = await timers()
  const resumeTimers =
    afterResume.timeout - beforeResume.timeout + (afterResume.interval - beforeResume.interval)
  assert.ok(
    resumeTimers < 50,
    `resuming 300 buffered events scheduled ${resumeTimers} timers; one per row would be 300`,
  )
  const resumedRows = await rowCount()
  assert.ok(resumedRows > 0 && resumedRows <= WINDOW_SIZE, `resume rendered ${resumedRows} rows`)
  const newestId = await page.evaluate(() => window.__nextId - 1)
  await page
    .locator('table[aria-label="Live events"] tbody tr')
    .first()
    .getByText(`user-${newestId}`, { exact: true })
    .waitFor()
  console.log(
    `PASS pause buffers 300 events and resume flushes them in ${resumeTimers} timers, rendering ${resumedRows} rows newest-first`,
  )

  // ---------------------------------------------------------------------
  // Scrollback reaches retained history, pauses the tail, and keeps ARIA row
  // indices anchored to the full history.
  // ---------------------------------------------------------------------
  await page.getByRole('button', { name: 'Older (pauses tail)', exact: true }).click()
  await page.getByRole('button', { name: /^Resume/ }).waitFor()
  await page
    .getByText(`Showing ${WINDOW_SIZE + 1}–${WINDOW_SIZE * 2} of ${BURST} retained`, {
      exact: true,
    })
    .waitFor()
  assert.equal(
    await page.locator('table[aria-label="Live events"] tbody tr').first().getAttribute('aria-rowindex'),
    String(WINDOW_SIZE + 2),
    'scrollback row index must count from the full history, not the window',
  )
  assert.ok((await rowCount()) <= WINDOW_SIZE, 'scrollback stays within the window')
  await page.getByRole('button', { name: 'Newer', exact: true }).click()
  await page.getByText(`Showing 1–${WINDOW_SIZE} of ${BURST} retained`, { exact: true }).waitFor()
  console.log('PASS scrollback pauses the tail, stays windowed, and reports position in the full history')

  // ---------------------------------------------------------------------
  // Live ingest must not talk over a screen reader. The position line is a
  // polite live region, so anything in it that changes per flush queues an
  // announcement roughly ten times a second.
  //
  // This needs a *filling* buffer, not a full one: once the history is capped
  // the retained count stops moving and even a bad string goes quiet. So
  // reload for a fresh tail and stay under TAIL_CAP throughout.
  // ---------------------------------------------------------------------
  await page.goto(`${origin}/ns/a/events`)
  await page.getByText('Waiting for events', { exact: true }).waitFor()
  await emit(100)
  await settle()
  await page.evaluate(() => {
    window.__announced = []
    for (const node of document.querySelectorAll('[aria-live]')) {
      new MutationObserver(() => window.__announced.push(node.textContent.trim())).observe(node, {
        childList: true,
        characterData: true,
        subtree: true,
      })
    }
  })
  // 100 → 500 retained: well inside the cap, so the count is still climbing on
  // every flush and a count in the live region would announce on each one.
  for (let i = 0; i < 10; i++) {
    await emit(40)
    await page.waitForTimeout(120)
  }
  await settle()
  const retained = await page.evaluate(() => window.__nextId - 1)
  assert.ok(retained < BURST, `this check must stay under the ${BURST} cap, reached ${retained}`)
  const announced = await page.evaluate(() => window.__announced)
  assert.deepEqual(
    announced,
    [],
    `live ingest queued ${announced.length} screen-reader announcements while the buffer filled: ${JSON.stringify(announced.slice(0, 3))}`,
  )

  // Paused, the same region does announce — position changes are the
  // operator's own doing and are worth hearing.
  await page.getByRole('button', { name: 'Pause', exact: true }).click()
  await page.getByRole('button', { name: 'Older', exact: true }).click()
  await page.waitForFunction(() => window.__announced.length > 0, null, { timeout: 5_000 })
  console.log(
    `PASS a filling tail (${retained} retained) announces nothing while pause and scrollback still announce position changes`,
  )

  // ---------------------------------------------------------------------
  // Row links, filters, and cleanup.
  // ---------------------------------------------------------------------
  const someRow = page.locator('table[aria-label="Live events"] tbody tr').first()
  const href = await someRow.getByRole('link').first().getAttribute('href')
  assert.ok(href?.startsWith('/ns/a/subjects/'), `row link should target a subject, got ${href}`)

  // A filter change re-keys LiveTail: the old subscription closes and a new
  // one opens, and no timer from the old one survives.
  const tailsBefore = await page.evaluate(() => window.__tails().length)
  await page.getByRole('textbox', { name: 'Filter tail by action', exact: true }).fill('VIEW')
  await page.getByRole('textbox', { name: 'Filter tail by action', exact: true }).press('Enter')
  await page.waitForFunction((n) => window.__tails().length > n, tailsBefore, { timeout: 5_000 })
  assert.equal(
    await page.evaluate((n) => window.__tails().slice(0, n).every((s) => s.closed), tailsBefore),
    true,
    'the previous filter subscription must be closed',
  )

  // Sidebar interaction with the tail populated and still receiving — the
  // symptom that opened the issue. The duration is reported rather than
  // bounded: the structural assertions above (rows, timers) are what make it
  // fast, and a millisecond threshold here would only track CI's hardware.
  await emit(20)
  const navigateStartedAt = Date.now()
  await page
    .getByRole('navigation', { name: 'Main navigation', exact: true })
    .getByRole('link', { name: 'Overview', exact: true })
    .click()
  await page.waitForURL('**/ns/a')
  const navigatedInMs = Date.now() - navigateStartedAt
  // Unmount cleanup runs after paint, so wait for it rather than sampling the
  // instant the URL changes. A timeout here is the failure.
  await page.waitForFunction(() => window.__tails().every((s) => s.closed), null, {
    timeout: 5_000,
  })
  await page.waitForTimeout(1_000)
  const liveIntervals = await page.evaluate(() => window.__liveIntervals.size)
  assert.ok(
    liveIntervals <= baselineIntervals,
    `leaving the tail left ${liveIntervals - baselineIntervals} interval(s) running (baseline ${baselineIntervals})`,
  )
  console.log(
    `PASS row links, filter re-subscription, and cleanup on navigation away — sidebar navigation with a populated, receiving tail took ${navigatedInMs}ms`,
  )

  assert.deepEqual(pageErrors, [])
  console.log('PASS no uncaught browser errors')
} finally {
  await browser.close()
  await new Promise((resolve) => (server ? server.httpServer.close(resolve) : resolve()))
}
