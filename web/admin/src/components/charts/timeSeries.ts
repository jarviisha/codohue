/**
 * breakGaps inserts an all-null point `maxGapMs` after any sample whose
 * successor arrives later than that, so the chart breaks the line instead of
 * drawing straight (or flat, for step curves) across a stretch with no data.
 */
export function breakGaps<T extends { ts: string }>(
  points: T[],
  maxGapMs: number,
  keys: string[],
): Array<T | Record<string, string | null>> {
  const out: Array<T | Record<string, string | null>> = []
  points.forEach((point, i) => {
    out.push(point)
    const next = points[i + 1]
    if (!next) return
    const breakAt = Date.parse(point.ts) + maxGapMs
    if (Date.parse(next.ts) > breakAt) {
      out.push({ ts: new Date(breakAt).toISOString(), ...Object.fromEntries(keys.map((k) => [k, null])) })
    }
  })
  return out
}
