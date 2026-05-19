// PS1 prompt helpers — pure JS so `tests/urls.test.mjs` can import them
// without a TypeScript runtime. Re-exported from routes/nav.ts so component
// code keeps a single import path (`@/routes/nav`).

/**
 * @typedef {{ ns: string; segments: string[] }} Ps1
 */

/**
 * Parse a URL pathname into PS1 prompt segments.
 * Examples:
 *   /                       -> { ns: '~', segments: [] }
 *   /namespaces             -> { ns: '~', segments: ['namespaces'] }
 *   /namespaces/new         -> { ns: '~', segments: ['namespaces', 'new'] }
 *   /ns/prod                -> { ns: 'prod', segments: [] }
 *   /ns/prod/events         -> { ns: 'prod', segments: ['events'] }
 *   /ns/prod/catalog/items  -> { ns: 'prod', segments: ['catalog', 'items'] }
 *
 * @param {string} pathname
 * @returns {Ps1}
 */
export function parsePs1(pathname) {
  const parts = pathname.split('/').filter(Boolean)
  if (parts[0] === 'ns' && parts[1]) {
    return { ns: parts[1], segments: parts.slice(2) }
  }
  return { ns: '~', segments: parts }
}

/**
 * Render the PS1 prompt as a single string. Mirrors what Ps1Prompt.tsx
 * renders via JSX so the rendered shell-style line matches the design
 * contract (DESIGN.md §3.1.1).
 *
 * Examples:
 *   formatPs1('~', [])                      -> 'codohue@~:~ $'
 *   formatPs1('~', ['namespaces'])          -> 'codohue@~:~/namespaces $'
 *   formatPs1('prod', [])                   -> 'codohue@prod:~ $'
 *   formatPs1('prod', ['catalog', 'items']) -> 'codohue@prod:~/catalog/items $'
 *
 * @param {string} ns
 * @param {string[]} segments
 * @returns {string}
 */
export function formatPs1(ns, segments) {
  const tail = segments.length === 0 ? '' : '/' + segments.join('/')
  return `codohue@${ns}:~${tail} $`
}

/**
 * Build the URL for the i-th segment of a PS1 path. Used to make PS1
 * segments clickable in the layout.
 *
 * @param {string} ns
 * @param {string[]} segments
 * @param {number} idx
 * @returns {string}
 */
export function segmentTo(ns, segments, idx) {
  const sub = segments.slice(0, idx + 1).join('/')
  return ns === '~' ? `/${sub}` : `/ns/${ns}/${sub}`
}
