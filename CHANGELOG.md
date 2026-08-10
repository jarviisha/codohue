# Changelog

Notable changes to the public wire contract (`pkg/codohuetypes`) and the Go SDK
(`sdk/go`). Both modules are versioned together; tag them at the same version.

The format is loosely based on [Keep a Changelog](https://keepachangelog.com).

## Unreleased

Nothing yet.

## v0.5.1 — 2026-08-10

Audit remediation maintenance release. Server tag: `v0.9.0`.

### Breaking

- **Event ingest now rejects unknown namespaces.** Events are no longer stored
  with default action weights when the namespace is missing. Redis Stream
  entries targeting a missing namespace are treated as permanently invalid,
  acknowledged, and dropped.
- **Catalog item deletion now reports Qdrant cleanup failures.** The endpoint
  returns a server error and retains the PostgreSQL row so callers can retry,
  instead of returning `204` after a best-effort external cleanup.
- **JSON request bodies have an 8 MiB hard limit.** Oversized requests are
  rejected before business validation. Admin request decoders also reject
  unknown fields and trailing JSON data consistently.
- **Dense phase failures now fail the aggregate batch run.** The independent
  trending phase still runs, but `batch_run_logs.success` is `false` when the
  dense phase fails.

### Fixed

- Prevent cross-namespace recommendation cache collisions by encoding each
  cache-key component independently and validating cached response identity.
- Prevent same-version catalog re-embed runs from completing before reset
  items are processed. The watcher now uses the target strategy version frozen
  on the batch run rather than mutable namespace configuration.
- Make namespace cleanup retryable after partial PostgreSQL, Redis, or Qdrant
  failure, including when the namespace configuration row is already gone.
- Coordinate compute, namespace deletion, and app reset with shared/exclusive
  PostgreSQL maintenance advisory locks. App reset waits for active runs to
  drain and blocks new runs until cleanup completes.
- Propagate namespace configuration lookup failures during ingest so transient
  infrastructure errors are retried instead of silently changing event weight.
- Delete catalog vectors before deleting their durable PostgreSQL rows, keeping
  failed external cleanup retryable.
- Allow valid admin credentials to bypass an exhausted failed-attempt bucket,
  preventing failed logins from locking out operators behind a shared proxy IP.

### Changed

- Recommendation cache keys now use the `rec:v2:*` format. Existing entries
  are ignored and expire naturally, causing a temporary increase in cache misses
  after deployment.
- Deploy `api`, `admin`, `cron`, and `embedder` together for this release so all
  processes use the same maintenance-lock and re-embed completion protocols.
- Updated README, offline evaluation tooling, and Redis Streams SDK examples to
  use `dense_source`, `occurred_at`, and the current catalog configuration API.
- No public Go API or `pkg/codohuetypes` wire type changed. The Go module
  `v0.5.1` tags are maintenance releases for corrected published documentation.

## v0.5.0 — 2026-08-03

Additive wire-contract and SDK changes. Server tag: `v0.8.0`.

### Added

- **Wire: catalog stream transport.** `CatalogStreamName`
  (`codohue:catalog`) and `CatalogStreamItem` — the durable fire-and-forget
  twin of `POST /v1/namespaces/{ns}/catalog`. Carries `namespace` in the
  payload because a stream entry has no URL path; neither stream is
  producer-trimmed, so content published during an outage is ingested on
  recovery.
- **Wire: batch catalog ingest.** `CatalogBatchIngestRequest` /
  `CatalogBatchIngestResponse` / `CatalogBatchItemResult` and the
  `CatalogBatchMaxItems` cap (100). Items are validated independently — one
  bad item does not fail the batch.
- **Wire: catalog reconciliation read.** `CatalogObjectsResponse` /
  `CatalogObjectSummary` for `GET /v1/namespaces/{ns}/catalog/objects`
  (`?changed_since=&limit=&offset=`), ordered by `updated_at` ascending so a
  repair pass re-sends only the gap.
- **Wire (`RankedItem`): `scored` (bool).** Distinguishes "evaluated, low
  relevance" from "returned unscored" (never indexed, or excluded by the
  seen-items / `exclude_authored` filters). `RankResponse.source` is
  `"no_subject_vector"` when the subject has no vector at all — the whole
  list comes back unscored in request order. Golden snapshots for
  `rank_response` / `ranked_item` were regenerated.
- **Wire: `DenseSource` constants.** `DenseSourceDisabled` / `Item2Vec` /
  `SVD` / `BYOE` / `Catalog`, so clients stop hand-copying string literals.
  Namespace configuration, not a marshaled type — no golden snapshot.
- **SDK: `Namespace.IngestCatalogBatch`** (≤100 items, per-item results) and
  **`Namespace.ListCatalogObjects`** (reconciliation read).
- **SDK: `Namespace.PutObject`.** Per-object metadata independent of
  embedding — currently `author_subject_id`, which feeds `exclude_authored`.
  Works under every `dense_source`; an empty value clears the attribution.
- **SDK: `sdk/go/admin` package.** Bearer-token admin client (no session
  cookies) with `ProvisionCatalogNamespace` — one-call provisioning of the
  core catalog mode, returning the namespace data-plane key on first create.
- **SDK: `redistream.CatalogProducer`** (`NewCatalogProducer`, `Publish`,
  `PublishBatch`, `WithCatalogStream`) for the catalog stream.

### Changed

- **`Rank` scores are comparable across calls.** The hybrid blend now
  normalizes batch-independently, so ranking a candidate set in chunks yields
  the same relative ordering as one call over the union. `Rank` also applies
  the same eligibility filters as recommendations.
- `sdk/go` and `sdk/go/redistream` require `pkg/codohuetypes v0.5.0`.

## v0.4.0 — 2026-07-23

Breaking wire-contract and SDK changes accumulated since `v0.3.0`.

### Breaking

- **Wire (`EventPayload`): removed the `metadata` field.** The `events` table
  never had a column for it, so it was accepted on the wire and silently
  discarded — a contract advertising a capability the server does not have. The
  HTTP ingest path now rejects an unknown `metadata` key with `400` (via
  `DecodeStrict`); the Redis Streams path ignores unknown fields, so existing
  Streams producers keep working without a rebuild. Categorical signals belong
  on the catalog item, not the event.
- **SDK: removed `WithWindowHours` (and the `windowHours` list option).** The
  trending look-back window is namespace configuration — there is one trending
  ZSET per namespace — so a per-request `window_hours` param was ignored by the
  server. The SDK no longer sends it. Trending look-back is set via the admin
  namespace config, not per call.

### Added

- **Wire (`EmbeddingRequest`): `object_created_at` (optional).** Feeds the
  γ-based object-freshness rerank for BYOE object vectors.
- **SDK: `WithObjectCreatedAt(time.Time)` option on `StoreObjectEmbedding`.**
  Sends the new `object_created_at`. No-op for subject embeddings.

## v0.3.0 and earlier

See git history / tags `pkg/codohuetypes/v0.3.0`, `sdk/go/v0.3.0`.
