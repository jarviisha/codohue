# Configuration UX: first implementation

Scope: P0/P1 namespace configuration and read-only System Runtime on the existing
`fix/admin-console-operator-ux` branch. Preserve Astryx and semantic tokens.

## Implemented behavior

- Namespace-qualified heading, anchor navigation, sticky save/discard controls,
  per-field before/after review and application guidance.
- Save reads the canonical configuration back from the server. A failed readback
  is explicitly a successful save with reload required, not a failed save.
- Existing draft preservation and external-change guards remain in place.
- Dense dimension and distance controls explain the migration requirement when
  dense collections already exist; no implicit collection recreation.
- Trending window changes require batch rebuild. TTL expiry never triggers a
  batch; warn when TTL is no longer than a recently reported cron interval.
- Action weights explain sparse and trending effects and negative signals.
- Catalog settings have one editing entry point in Configuration. Their existing
  separate endpoint and save operation remain explicit. A pending namespace
  draft prevents starting a catalog edit. Catalog keeps operational actions.
- Blank catalog override fields preserve existing values; do not claim that
  omission clears an override or restores inheritance.
- Runtime reports originate independently from all four processes. Redis-backed
  reports are ephemeral, allowlisted and read-only; missing/failed reports are
  distinguished from healthy services and effective settings are not inferred.

## Deliberately deferred

- Config revisions, atomic optimistic concurrency, audit history and rollback.
- Proof that a specific config revision reached a worker or batch.
- Dynamic system settings, automatic restart, and config-dependent cache keys.
- Resetting catalog overrides to inheritance requires an explicit backend
  contract; the current omission semantics are preserved and explained.
- Runtime reports cover allowlisted operational settings, not every environment
  variable or deployment desired state. Runtime reporting is not health probing.

## Validation

Frontend lint, unit tests, production build, and browser workflows; Go package
and race suites; E2E coverage of authenticated runtime reporting from actual API
and admin processes with isolated PostgreSQL, Redis and Qdrant.
