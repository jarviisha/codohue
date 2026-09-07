# ADR-0001: Retain forward-compatible boundary adapters

- Status: Accepted
- Date: 2026-09-07
- Issue: [#22](https://github.com/jarviisha/codohue/issues/22)

## Context

Two boundaries currently have only one production implementation and can look
like speculative abstraction when judged only by adapter count.

`internal/core/embedstrategy` owns the strategy interface and registry used to
select, validate, list, and build catalog embedding strategies. The shipped
implementation is `hashing-ngrams`, with multiple dimension variants. The
boundary also prevents `internal/catalog`, `internal/embedder`, and
`internal/admin` from importing one another.

`cmd/admin/nsconfig_adapter.go` translates admin request/response types and
errors to `internal/nsconfig`. Much of the request mapping is mechanical, but
the adapter keeps peer domains independent and gives validation-error
translation one composition-layer home.

Collapsing either boundary would reduce code today, but would move strategy
selection or peer-domain translation into callers and weaken import rules that
are enforced by `internal/architecture/imports_test.go`.

## Decision

Retain both boundaries.

- Keep `embedstrategy.Registry`, including `Register`, `RegisterVariants`,
  `Build`, `Has`, and `List`, as the supported extension point for additional
  embedding strategies.
- Keep namespace-config translation in the `cmd/admin` composition root.
- Do not make `internal/nsconfig` accept an admin-domain DTO. A neutral shared
  type is justified only when it represents a stable domain contract rather
  than identical field spelling at one call site.
- Treat field-copy boilerplate as an acceptable cost of preserving ownership;
  keep the error mapping explicit and tested.

Adapter count alone is not sufficient reason to remove a boundary when that
boundary enforces dependency direction or represents an accepted product
extension point.

## Consequences

- Adding a strategy does not require catalog, admin, or embedder callers to
  change their selection protocol.
- Admin and namespace-config packages remain independently testable and do not
  acquire a peer-domain import.
- Some request/response fields remain mechanically copied at the composition
  root.
- Registry behavior for factories that cannot build with empty parameters
  remains part of the extension contract and must be covered when such a
  strategy is introduced.

## Deletion criteria

Reconsider the strategy registry only if the product explicitly abandons
runtime strategy selection and the catalog/admin/embedder consumers no longer
need a shared build/list/validation protocol.

Reconsider the namespace-config adapter only if the architecture permits a
direct dependency, or a neutral core-owned configuration contract emerges
with semantics shared by multiple consumers. A second DTO having the same
fields is not, by itself, that contract.

