// Ambient types for ps1.mjs — the runtime impl is plain JS so it can run
// directly under `node --test` without a TypeScript runtime. This .d.mts
// sibling provides the type signatures TS callers expect.

export interface Ps1 {
  ns: string
  segments: string[]
}

export function parsePs1(pathname: string): Ps1

export function formatPs1(ns: string, segments: string[]): string

export function segmentTo(ns: string, segments: string[], idx: number): string
