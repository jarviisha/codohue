import assert from 'node:assert/strict'
import { chromium } from 'playwright'
import { preview } from 'vite'

// The preview serves the built SPA. Every API response is a local fixture.
const server = process.env.ADMIN_TEST_URL
  ? null
  : await preview({ preview: { host: '127.0.0.1', port: 4175, strictPort: true } })
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
const origin = process.env.ADMIN_TEST_URL || 'http://127.0.0.1:4175'
const pageErrors = []
page.on('pageerror', (error) => pageErrors.push(error.message))
let config = {
  namespace: 'a',
  action_weights: { VIEW: 1 },
  lambda: 0.01,
  gamma: 0,
  alpha: 0.5,
  max_results: 20,
  seen_items_days: 30,
  exclude_authored: false,
  dense_source: 'disabled',
  embedding_dim: 128,
  dense_distance: 'cosine',
  trending_window: 24,
  trending_ttl: 60,
  lambda_trending: 0.01,
  has_api_key: true,
  updated_at: '2026-09-18T00:00:00Z',
}
let savedBodies = []
let summaryFailure = true
let trendingFailure = true
let trendingRows = []
let healthFailure = false
let healthDetails = false
let catalogRequests = 0
let configReadFailure = false
let saveFailure = false
let denseLocked = false
const revisions = { recommendations: 1, signals: 1, trending: 1, embeddings: 1 }
const groupFields = {
  recommendations: ['alpha', 'gamma', 'max_results', 'seen_items_days', 'exclude_authored'],
  signals: ['lambda', 'action_weights'],
  trending: ['trending_window', 'trending_ttl', 'lambda_trending'],
  embeddings: [
    'dense_source',
    'embedding_dim',
    'dense_distance',
    'catalog_strategy_id',
    'catalog_strategy_version',
    'catalog_strategy_params',
    'catalog_max_attempts',
    'catalog_max_content_bytes',
  ],
}
const configuration = () => ({
  namespace: 'a',
  generation: 1,
  defaults: {
    catalog_max_attempts: {
      state: 'observed',
      process: 'embedder',
      value: 5,
      reports: [{ instance: 'worker', reported_at: config.updated_at, value: 5 }],
    },
    catalog_max_content_bytes: { state: 'unknown', process: 'api', reports: [] },
  },
  groups: Object.fromEntries(
    Object.entries(groupFields).map(([g, fields]) => [
      g,
      {
        revision: revisions[g],
        values: Object.fromEntries(fields.map((f) => [f, config[f] ?? null])),
        locks:
          g === 'embeddings' && denseLocked
            ? {
                embedding_dim: 'Dense collections exist',
                dense_distance: 'Dense collections exist',
              }
            : {},
        guidance: 'Saved values are used by subsequent work. Saving does not start a job.',
      },
    ]),
  ),
})
await context.route('**/api/**', async (route) => {
  const url = new URL(route.request().url())
  const path = url.pathname
  const json = (body, status = 200) => route.fulfill({ status, json: body })
  if (path.endsWith('/stream'))
    return route.fulfill({ contentType: 'text/event-stream', body: ': connected\n\n' })
  if (path.endsWith('/sessions/current')) return json({ name: 'Test operator', role: 'admin' })
  if (path === '/api/admin/v1/namespaces')
    return json({ items: [config, { ...config, namespace: 'b', alpha: 0.9 }], total: 2 })
  if (/\/namespaces\/[^/]+$/.test(path) && route.request().method() === 'GET')
    return configReadFailure
      ? json({ error: { message: 'Readback unavailable' } }, 503)
      : json(config)
  if (path.endsWith('/catalog/strategies'))
    return json({ strategies: [{ id: 'test', version: 'v1', dim: 128 }] })
  if (path.endsWith('/configuration')) {
    if (route.request().method() === 'PATCH') {
      const body = route.request().postDataJSON()
      if (saveFailure)
        return json(
          { error: { code: 'unavailable', message: 'Save temporarily unavailable' } },
          503,
        )
      if (body.base_revision !== revisions[body.group])
        return json(
          {
            error: {
              code: 'configuration_conflict',
              message: 'Changed elsewhere',
              current: configuration(),
            },
          },
          409,
        )
      savedBodies.push(body)
      config = { ...config, ...body.changes }
      revisions[body.group]++
    }
    return json(configuration())
  }
  if (path.endsWith('/catalog'))
    return json({
      catalog: { namespace: 'a', enabled: true, strategy_id: 'test', strategy_version: 'v1' },
      available_strategies: [{ id: 'test', version: 'v1', dim: 128 }],
    })
  if (path.endsWith('/dashboard'))
    return json({
      config: {
        ...config,
        namespace: path.includes('/b/') ? 'b' : 'a',
        alpha: path.includes('/b/') ? 0.9 : config.alpha,
      },
      author_coverage: { total: 0, attributed: 0 },
    })
  if (route.request().method() === 'PUT') {
    const body = route.request().postDataJSON()
    savedBodies.push(body)
    config = { ...config, ...body, updated_at: '2026-09-18T00:01:00Z' }
    return json({ namespace: 'a', updated_at: config.updated_at })
  }
  if (path.endsWith('/events/summary')) {
    return summaryFailure
      ? json({ error: { message: 'Telemetry unavailable' } }, 503)
      : json({
          total: 10,
          rate_per_second: 2,
          window_seconds: 60,
          by_action: [{ action: 'VIEW', count: 10 }],
          series: [{ ts: config.updated_at, count: 10 }],
        })
  }
  if (path === '/api/admin/v1/runtime')
    return json({
      observed_at: config.updated_at,
      expiry_seconds: 120,
      processes: [
        {
          process: 'admin',
          instance: 'test-admin',
          started_at: config.updated_at,
          reported_at: config.updated_at,
          settings: [{ name: 'secure_cookies', value: true }],
        },
      ],
    })
  if (path.endsWith('/trending')) {
    assert.equal(new URL(route.request().url()).searchParams.has('window_hours'), false)
    return trendingFailure
      ? json({ error: { message: 'Unavailable' } }, 503)
      : json({
          namespace: 'a',
          items: trendingRows,
          window_hours: 72,
          total: trendingRows.length,
          limit: 50,
          offset: 0,
          cache_ttl_sec: 0,
          generated_at: config.updated_at,
        })
  }
  if (path.endsWith('/health'))
    return healthFailure
      ? json({ error: { message: 'Unavailable' } }, 503)
      : json(
          healthDetails
            ? { status: 'ok', postgres: 'ok', redis: 'ok', qdrant: 'ok' }
            : { status: 'ok', postgres: '', redis: '', qdrant: '' },
        )
  if (path.endsWith('/subjects'))
    return json({
      total: 51,
      items: [{ subject_id: 'user-42', interaction_count: 20, last_seen: config.updated_at }],
    })
  if (path.endsWith('/catalog/items')) {
    catalogRequests++
    return json({
      total: 51,
      items: [
        {
          id: 42,
          object_id: 'item-42',
          state: 'failed',
          attempt_count: 1,
          last_error: 'An embedding failure with enough detail to help an operator recover.',
          updated_at: config.updated_at,
        },
      ],
    })
  }
  return json({ error: { message: 'Fixture intentionally unavailable' } }, 404)
})
const goto = (path) => page.goto(`${origin}${path}`)
try {
  await page.clock.install()
  await goto('/ns/a/config')
  const alpha = page.getByRole('textbox', { name: 'Alpha (sparse weight)', exact: true })
  await alpha.fill('0.4')
  await page.getByRole('tab', { name: 'Trending', exact: true }).click()
  await page.getByRole('textbox', { name: 'Trending TTL (seconds)', exact: true }).fill('3600')
  await page.getByRole('button', { name: 'Save trending', exact: true }).click()
  await page.getByText(/Trending saved/).waitFor()
  assert.equal(savedBodies.length, 1)
  assert.deepEqual(savedBodies[0].changes, { trending_ttl: 3600 })
  await page.getByRole('tab', { name: 'Recommendations · Unsaved', exact: true }).click()
  assert.equal(await alpha.inputValue(), '0.4')
  await page.getByRole('button', { name: 'Save recommendations', exact: true }).click()
  await page.getByText(/Recommendations saved/).waitFor()
  assert.deepEqual(savedBodies[1].changes, { alpha: 0.4 })
  await alpha.fill('0.6')
  config = { ...config, alpha: 0.8 }
  revisions.recommendations++
  await page.clock.fastForward(31_000)
  assert.equal(await alpha.inputValue(), '0.6')
  await page.getByRole('button', { name: 'Save recommendations', exact: true }).click()
  await page.getByRole('heading', { name: 'Review concurrent changes', exact: true }).waitFor()
  assert.equal(await alpha.inputValue(), '0.6')
  await page.getByRole('button', { name: 'Keep my changed fields for review', exact: true }).click()
  await page.getByRole('button', { name: 'Save recommendations', exact: true }).click()
  await page.getByText(/Recommendations saved/).waitFor()
  await alpha.fill('broken')
  await page.getByRole('button', { name: 'Save recommendations', exact: true }).click()
  await page.getByRole('heading', { name: 'Could not save this section' }).waitFor()
  assert.equal(await alpha.inputValue(), 'broken')
  await page.getByRole('button', { name: 'Discard', exact: true }).click()
  await page.getByRole('tab', { name: 'Event signals', exact: true }).click()
  await page.getByRole('button', { name: 'Add action', exact: true }).click()
  await page.getByRole('tab', { name: 'Trending', exact: true }).click()
  await page.getByRole('tab', { name: 'Event signals · Unsaved', exact: true }).click()
  await page.getByRole('textbox', { name: 'Action name, row 2' }).fill('VIEW')
  await page.getByRole('button', { name: 'Save event signals', exact: true }).click()
  await page.getByRole('heading', { name: 'Could not save this section' }).waitFor()
  await page.getByRole('textbox', { name: 'Action name, row 2' }).fill('LIKE')
  saveFailure = true
  await page.getByRole('button', { name: 'Save event signals', exact: true }).click()
  await page.getByText('Save temporarily unavailable', { exact: true }).waitFor()
  assert.equal(await page.getByRole('textbox', { name: 'Action name, row 2' }).inputValue(), 'LIKE')
  saveFailure = false
  await page.getByRole('button', { name: 'Save event signals', exact: true }).click()
  await page.getByText(/Event signals saved/).waitFor()
  assert.deepEqual(savedBodies.at(-1).changes.action_weights, { VIEW: 1, LIKE: 1 })
  console.log(
    'PASS independent saves, multiple drafts, conflict reconciliation, raw numeric validation and failure retry',
  )

  config = {
    ...config,
    dense_source: 'catalog',
    catalog_strategy_id: 'test',
    catalog_strategy_version: 'v1',
    catalog_strategy_params: {},
    catalog_max_attempts: 7,
    catalog_max_content_bytes: null,
  }
  denseLocked = true
  await goto('/ns/a/config?tab=embeddings')
  await page.getByRole('textbox', { name: 'Embedding dimension', exact: true }).waitFor()
  assert.equal(
    await page.getByRole('textbox', { name: 'Embedding dimension', exact: true }).isDisabled(),
    true,
  )
  await page
    .getByRole('switch', { name: 'Use system default: embedding retry limit', exact: true })
    .click()
  await page.getByRole('button', { name: 'Save embeddings', exact: true }).click()
  await page.getByText(/Embeddings saved/).waitFor()
  assert.equal(savedBodies.at(-1).changes.catalog_max_attempts, null)
  assert.equal(
    await page.getByRole('textbox', { name: 'Embedding dimension', exact: true }).isDisabled(),
    true,
  )
  await page.getByRole('tab', { name: 'Trending', exact: true }).click()
  await page.goBack()
  await page.getByRole('textbox', { name: 'Embedding dimension', exact: true }).waitFor()
  for (const mode of ['light', 'dark']) {
    await page.emulateMedia({ colorScheme: mode })
    for (const width of [375, 768, 1440]) {
      await page.setViewportSize({ width, height: 1000 })
      assert.equal(
        await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
        false,
      )
      await page.screenshot({ path: `/tmp/settings-${mode}-${width}.png`, fullPage: true })
    }
  }
  await page.evaluate(() => (document.documentElement.style.zoom = '2'))
  assert.equal(
    await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    false,
  )
  await page.evaluate(() => (document.documentElement.style.zoom = ''))
  console.log(
    'PASS inline catalog inheritance, persistent locks, tab history and responsive light/dark settings',
  )

  await goto('/ns/a/subjects?q=user&page=2&sort=interactions')
  await page.getByRole('link', { name: 'user-42', exact: true }).click()
  await page.waitForURL('**/subjects/user-42')
  await page.goBack()
  await page.getByText('page 2', { exact: true }).waitFor()
  assert.equal(
    await page.getByRole('textbox', { name: 'Search subjects', exact: true }).inputValue(),
    'user',
  )
  await page.reload()
  await page.getByText('page 2', { exact: true }).waitFor()
  console.log('PASS list filters and pagination survive detail/back/reload')

  await page.getByRole('button', { name: 'Open command palette' }).click()
  const paletteInput = page.getByRole('dialog').getByRole('combobox')
  await paletteInput.fill('#42')
  await page.getByText('Open batch run #42', { exact: true }).waitFor()
  await paletteInput.press('ArrowDown')
  await paletteInput.press('Enter')
  await page.waitForURL('**/batch-runs/42')
  console.log('PASS dynamic command-palette keyboard selection')

  await goto('/ns/a/subjects/user-42')
  await page.getByRole('button', { name: 'Namespace', exact: true }).click()
  await page.getByRole('option', { name: 'b', exact: true }).click()
  await page.waitForURL('**/ns/b/subjects')
  console.log('PASS namespace switch drops string subject IDs')
  const sidebar = page.getByRole('navigation', { name: 'Main navigation', exact: true })
  assert.equal(await sidebar.getByRole('link', { name: 'Batch runs', exact: true }).count(), 1)
  assert.equal(await sidebar.getByRole('link', { name: 'Health', exact: true }).count(), 0)
  await sidebar.locator('a[href="/ns/b/config"]').waitFor()
  assert.equal(
    await sidebar.getByRole('link', { name: 'Configuration', exact: true }).getAttribute('href'),
    '/ns/b/config',
  )
  await sidebar.getByRole('link', { name: '← All namespaces', exact: true }).click()
  await page.waitForURL('**/namespaces')
  await sidebar.getByRole('link', { name: 'Health', exact: true }).waitFor()
  assert.equal(await sidebar.getByRole('link', { name: 'Configuration', exact: true }).count(), 0)
  console.log('PASS sidebar switches between namespace and global destinations')

  await goto('/ns/a/events')
  await page.getByText('Event summary could not refresh', { exact: true }).waitFor()
  assert.equal(await page.getByText('Rate / s', { exact: true }).count(), 0)
  summaryFailure = false
  await page.getByRole('button', { name: 'Retry', exact: true }).click()
  await page.getByText('Rate / s', { exact: true }).waitFor()
  await page.getByRole('radio', { name: '5m', exact: true }).click()
  assert.equal(
    await page.getByRole('radio', { name: '5m', exact: true }).getAttribute('aria-checked'),
    'true',
  )
  await page.getByText('View chart data', { exact: true }).click()
  await page.getByRole('columnheader', { name: 'Time', exact: true }).waitFor()
  summaryFailure = true
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await page
    .getByText('Showing the last successful snapshot. Values may be out of date.', { exact: true })
    .waitFor()
  assert.equal(await page.getByText('Rate / s', { exact: true }).count(), 1)
  console.log('PASS summary failure/recovery, selected window and accessible chart data')

  await goto('/system/runtime')
  await page.getByText('Managed by deployment', { exact: true }).waitFor()
  await page.getByText('secure cookies', { exact: true }).waitFor()
  assert.equal(await page.getByText('No recent report', { exact: true }).count(), 3)
  console.log('PASS runtime shows self-reported values and missing reports explicitly')

  await goto('/ns/a/trending')
  await page.getByText('Trending could not refresh', { exact: true }).waitFor()
  assert.equal(await page.getByText('No trending items available', { exact: true }).count(), 0)
  trendingFailure = false
  await page.getByRole('button', { name: 'Retry', exact: true }).click()
  await page.getByText('No trending items available', { exact: true }).waitFor()
  await page.getByText('Configured event window: 72 hours', { exact: true }).waitFor()
  assert.equal(await page.getByRole('button', { name: '7d', exact: true }).count(), 0)
  assert.equal(await page.getByText('expires in 0s', { exact: true }).count(), 0)
  trendingRows = [{ object_id: 'trending-item', score: 2.5, cache_ttl_sec: 0 }]
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await page.getByText('trending-item', { exact: true }).waitFor()
  trendingFailure = true
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await page.getByText('Trending could not refresh', { exact: true }).waitFor()
  assert.equal(await page.getByText('trending-item', { exact: true }).count(), 1)
  console.log(
    'PASS trending separates errors, empty results, configured window and stale snapshots',
  )

  await goto('/health')
  await page.getByText('PostgreSQL', { exact: true }).waitFor()
  assert.equal(await page.getByText('unknown', { exact: true }).count(), 3)
  assert.equal(await page.getByText('ok', { exact: true }).count(), 1)
  healthDetails = true
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await page.getByText('unknown', { exact: true }).first().waitFor({ state: 'detached' })
  assert.equal(await page.getByText('ok', { exact: true }).count(), 4)
  healthFailure = true
  await page.getByRole('button', { name: 'Refresh', exact: true }).click()
  await page.getByText('Service health could not refresh', { exact: true }).waitFor()
  assert.equal(await page.getByText('PostgreSQL', { exact: true }).count(), 1)
  console.log('PASS health keeps last good snapshot after refresh failure')

  await goto('/ns/a/catalog/items')
  await page.getByRole('link', { name: 'item-42' }).waitFor()
  const beforeTyping = catalogRequests
  await page.getByRole('textbox', { name: 'Search catalog items', exact: true }).fill('item')
  assert.equal(catalogRequests, beforeTyping)
  await page.getByRole('button', { name: 'Apply filter', exact: true }).click()
  await page.waitForURL('**/catalog/items?q=item')
  await page.getByRole('button', { name: 'Delete item-42', exact: true }).waitFor()
  await page.getByText('View error', { exact: true }).click()
  await page
    .getByText('An embedding failure with enough detail to help an operator recover.', {
      exact: true,
    })
    .waitFor()
  console.log('PASS explicit list search and accessible row actions/errors')

  for (const width of [375, 768, 1440]) {
    await page.setViewportSize({ width, height: 1000 })
    await page.screenshot({ path: `/tmp/admin-ux-catalog-${width}.png`, fullPage: true })
    const tableWidth = await page
      .getByRole('table', { name: 'Catalog items', exact: true })
      .evaluate((node) => node.getBoundingClientRect().width)
    assert.ok(tableWidth >= 800, 'Catalog columns retain readable widths')
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    )
    assert.equal(overflow, false, `Page overflows at ${width}px`)
  }
  await page.emulateMedia({ colorScheme: 'dark', reducedMotion: 'reduce' })
  await page.screenshot({ path: '/tmp/admin-ux-catalog-dark.png', fullPage: true })
  await goto('/ns/a/config')
  await page.getByRole('heading', { name: 'Settings · a', exact: true }).waitFor()
  await page.setViewportSize({ width: 375, height: 1000 })
  await page.screenshot({ path: '/tmp/admin-ux-config-mobile.png', fullPage: true })
  assert.equal(
    await page.evaluate(() => document.documentElement.scrollWidth > window.innerWidth),
    false,
  )
  await page.getByRole('tab', { name: 'Event signals', exact: true }).click()
  // A blank new row is still an unsaved edit and must trigger the leave guard.
  await page.getByRole('button', { name: 'Add action', exact: true }).click()
  await page.getByRole('link', { name: 'fleet', exact: true }).click()
  await page.getByRole('heading', { name: 'Discard unsaved changes?', exact: true }).waitFor()
  await page.getByRole('button', { name: 'Stay on page', exact: true }).click()
  assert.ok(page.url().includes('/ns/a/config'))
  assert.deepEqual(pageErrors, [])
  console.log('PASS responsive page containment and no uncaught browser errors')
} finally {
  await browser.close()
  await new Promise((resolve) => (server ? server.httpServer.close(resolve) : resolve()))
}
