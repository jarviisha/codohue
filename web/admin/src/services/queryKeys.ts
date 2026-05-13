// Re-export barrel for every domain's TanStack Query key factory.
// Convention: keys are hierarchical tuples `[domain, ns?, ...params]` so
// invalidation patterns stay obvious (e.g. invalidate all namespace data
// with `['ns', name]`).
//
// Each domain service file owns its keys; this file is the single import
// surface for consumers that need to invalidate cross-domain.
export { authKeys } from './auth'
