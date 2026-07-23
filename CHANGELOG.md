# Changelog

Notable changes to the public wire contract (`pkg/codohuetypes`) and the Go SDK
(`sdk/go`). Both modules are versioned together; tag them at the same version.

The format is loosely based on [Keep a Changelog](https://keepachangelog.com).

## Unreleased — next tag: `v0.4.0`

Breaking wire-contract and SDK changes accumulated on `main` since `v0.3.0`.
Bump both `pkg/codohuetypes` and `sdk/go` to `v0.4.0` when tagging.

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
