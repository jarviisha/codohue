import test from 'node:test'
import assert from 'node:assert/strict'
import {
  safeNamespaceSection,
  validateActionWeights,
  readPage,
} from '../src/services/operatorUx.ts'

test('namespace switching never carries entity IDs, including encoded string IDs', () => {
  for (const id of ['42', 'user-42', 'user%2F42', 'config']) {
    assert.equal(safeNamespaceSection(`/ns/store/subjects/${id}`), '/subjects')
    assert.equal(safeNamespaceSection(`/ns/store/catalog/items/${id}`), '/catalog/items')
    assert.equal(safeNamespaceSection(`/ns/store/batch-runs/${id}`), '/batch-runs')
  }
  assert.equal(safeNamespaceSection('/ns/a%20b/events'), '/events')
  assert.equal(safeNamespaceSection('/ns/store/config'), '/config')
  assert.equal(safeNamespaceSection('/ns/store/unknown'), '')
  assert.equal(safeNamespaceSection('/namespaces'), '')
})

test('action validation reports every duplicate and does not normalize away invalid rows', () => {
  const errors = validateActionWeights([
    { id: 'a', name: 'VIEW', value: 1 },
    { id: 'b', name: ' VIEW ', value: 2 },
    { id: 'c', name: ' ', value: NaN },
  ])
  assert.deepEqual(
    errors.map((error) => error.id),
    ['action-a', 'action-b', 'action-c', 'weight-c'],
  )
  assert.deepEqual(
    validateActionWeights([
      { id: 'a', name: 'VIEW', value: 0 },
      { id: 'b', name: 'view', value: -1 },
    ]),
    [],
  )
})

test('pagination rejects malformed, unsafe and out-of-range query values', () => {
  for (const value of [
    null,
    '',
    '0',
    '-1',
    '1.2',
    'Infinity',
    '1e3',
    '1000001',
    '9007199254740993',
  ]) {
    assert.equal(readPage(value), 0)
  }
  assert.equal(readPage('1'), 0)
  assert.equal(readPage('12'), 11)
})
