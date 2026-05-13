import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const adminApi = readFileSync(new URL('../src/services/adminApi.ts', import.meta.url), 'utf8')
const api = readFileSync(new URL('../src/services/api.ts', import.meta.url), 'utf8')
const sources = `${adminApi}\n${api}`

test('admin SPA uses only canonical spec 003 URLs', () => {
  const required = [
    '/api/v1/auth/sessions',
    '/api/v1/auth/sessions/current',
    '/api/admin/v1/namespaces?include=overview',
    '/api/admin/v1/demo-data',
    '/api/admin/v1/batch-runs',
    '/qdrant',
    '/recommendations?${params}',
  ]

  for (const url of required) {
    assert.match(sources, new RegExp(escapeRegExp(url)))
  }

  const removed = [
    /\/api\/auth\/login/,
    /\/api\/auth\/logout/,
    /\/api\/admin\/v1\/namespaces\/overview/,
    /\/api\/admin\/v1\/recommend\/debug/,
    /\/api\/admin\/v1\/trending\//,
    /\/api\/admin\/v1\/subjects\//,
    /\/qdrant-stats/,
    /\/batch-runs\/trigger/,
    /\/api\/admin\/v1\/demo(?!-data)/,
    /\/v1\/recommendations/,
    /\/v1\/rank(?!ings)/,
    /\/v1\/trending\//,
  ]

  for (const pattern of removed) {
    assert.doesNotMatch(sources, pattern)
  }
})

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')
}
