import test from 'node:test'
import assert from 'node:assert/strict'
import { breakGaps } from '../src/components/charts/timeSeries.ts'

const at = (min, pending) => ({ ts: new Date(Date.UTC(2026, 8, 30, 0, min)).toISOString(), pending })
const MIN = 60_000

test('breakGaps leaves evenly sampled data alone', () => {
  const points = [at(0, 1), at(5, 2), at(10, 3)]
  assert.deepEqual(breakGaps(points, 10 * MIN, ['pending']), points)
})

test('breakGaps nulls every series right after a gap longer than maxGapMs', () => {
  const out = breakGaps([at(0, 1), at(30, 2)], 10 * MIN, ['pending'])
  assert.deepEqual(out, [at(0, 1), { ts: at(10).ts, pending: null }, at(30, 2)])
})

test('breakGaps handles empty and single-point series', () => {
  assert.deepEqual(breakGaps([], MIN, ['pending']), [])
  assert.deepEqual(breakGaps([at(0, 1)], MIN, ['pending']), [at(0, 1)])
})
