import test from 'node:test'
import assert from 'node:assert/strict'
import { toDraft, draftChanges, parseChanges } from '../src/services/configurationDraft.ts'

test('draft changes distinguish omission, explicit inheritance and numeric text', () => {
  const baseline = toDraft({ catalog_max_attempts: 5, alpha: 0.5, exclude_authored: false })
  assert.deepEqual(parseChanges(baseline, { ...baseline, catalog_max_attempts: null }), {
    changes: { catalog_max_attempts: null },
    errors: {},
  })
  assert.deepEqual(parseChanges(baseline, { ...baseline, alpha: '0.6' }), {
    changes: { alpha: 0.6 },
    errors: {},
  })
  for (const invalid of ['', 'NaN', '1e999', 'one'])
    assert.ok(parseChanges(baseline, { ...baseline, alpha: invalid }).errors.alpha)
  assert.ok(
    parseChanges(baseline, { ...baseline, catalog_max_attempts: '1.5' }).errors
      .catalog_max_attempts,
  )
  assert.deepEqual(draftChanges(baseline, baseline), [])
})
test('action map replacement preserves removals and rejects invalid raw drafts', () => {
  const baseline = toDraft({ action_weights: { VIEW: 1, LIKE: 3 } })
  assert.deepEqual(parseChanges(baseline, { action_weights: '{"VIEW":1}' }).changes, {
    action_weights: { VIEW: 1 },
  })
  for (const bad of ['[]', 'null', 'Invalid action weights: []'])
    assert.ok(parseChanges(baseline, { action_weights: bad }).errors.action_weights)
})

// The error summary focuses `#field-<name>`. TextInput takes the id directly;
// Selector and Switch do not, so those need an explicit anchor element.
test('every editable configuration field has an error-summary anchor', async () => {
  const { readFileSync } = await import('node:fs')
  const page = readFileSync(
    new URL('../src/pages/ns/config/NamespaceConfigPage.tsx', import.meta.url),
    'utf8',
  )
  for (const field of ['dense_source', 'dense_distance', 'exclude_authored', 'action_weights']) {
    assert.ok(page.includes(`id="field-${field}"`), `${field} has no #field-${field} target`)
  }
})
