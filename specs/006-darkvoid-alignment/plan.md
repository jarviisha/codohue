# Implementation Plan: Align Codohue with the DarkVoid integration

**Branch**: `feat/recommend-darkvoid-alignment` | **Date**: 2026-08-03 | **Design**: [design.md](design.md)
**Input**: Accepted design from `/specs/006-darkvoid-alignment/design.md` (rev 4 — all open
questions decided; there is no separate spec.md, the design document is the authoritative artifact)

## Summary

Make `dense_source="catalog"` the core mode of the system and align `Rank` with what
DarkVoid's `specs/006-materialized-ranked-feed` needs from it. The Codohue work, in the
decided order: (1) revise `Rank` — dense blend shared with `Recommend` via an extracted
helper, batch-independent sparse normalization (`x/(x+k)`, global constant `k`), the
seen-items + `exclude_authored` filters applied unconditionally, a `scored` per-item flag
and a `no_subject_vector` source on the wire — one contract revision, one SDK release;
(2) give the core catalog path a durable transport — a client-facing catalog Redis stream
consumed by the existing `ingest` worker, batch HTTP ingest, and a data-plane
reconciliation read; (3) provision the core mode in one request —
`dense_source="catalog"` accepted on the namespace upsert when strategy fields accompany
it, bearer admin auth with a failed-attempts rate limiter, `sdk/go/admin`; (4) hardening —
silent-downgrade warning (log + admin overview alert), SDK `PutObject`, readiness read.
DarkVoid's own flip to catalog (`CODOHUE_DENSE_SOURCE=catalog` + reindex) is a consumer-side
prerequisite, not a task in this repo.

## Technical Context

**Language/Version**: Go 1.x (go.work multi-module); TypeScript/React 19 for `web/admin` (alert surface only)  
**Primary Dependencies**: pgx (PostgreSQL), Qdrant gRPC, go-redis (Streams + pub/sub), Prometheus  
**Storage**: No schema change (design Non-goal). New Redis stream (`codohue:catalog` or per-ns variant, named in phase 2). Qdrant collections unchanged  
**Testing**: `go test` across modules (`make test`, `make test-pkg`), `make test-e2e`; golden snapshots in `pkg/codohuetypes/testdata/` regenerated once for the P2 contract change  
**Target Platform**: Linux server (four binaries: api, cron, admin, embedder)  
**Project Type**: Web service; server domains + two SDK modules (`sdk/go`, `sdk/go/redistream`)  
**Performance Goals**: `Rank` stays one Qdrant round trip per side (sparse + dense) per request; candidate cap stays 500 until the `HasID` filter cost at 500/1000/2000 is measured (D4)  
**Constraints**: `/recommendations` contract and ordering semantics unchanged — score *values* shift once (shared blend helper + new normalization), called out in release notes; wire-contract changes limited to additive fields so existing decoders keep working; all P1–P4 + constants ship in one SDK bump  
**Scale/Scope**: ~4 server domains touched (`recommend`, `ingest`, `catalog`, `admin`) + `pkg/codohuetypes` + both SDK modules; no new binary; one new client-facing transport

## Constitution Check

*GATE: Must pass before task generation. Re-check during implementation.*

| Gate | Status | Notes |
|------|--------|-------|
| **I. Code Quality** — domain in `internal/<domain>/`, `docs.go` present, import boundaries respected, English-only comments | ☑ | No new domains. Catalog-stream consumption lands in the `ingest` worker calling `internal/catalog` machinery **via `cmd/api` wiring** (D5) — peer import stays forbidden; same adapter pattern as `cmd/admin/nsconfig_adapter.go`. |
| **II. Testing Standards** — `_test.go` for every `service.go`/`repository.go`/`worker.go` touched | ☑ | Extend `internal/recommend/service_test.go` (blend helper, filters, scored flag, fallback source), `internal/ingest` worker tests (catalog stream), `internal/admin` (bearer auth, upsert validation, overview alert). E2e: rank path in `make test-e2e-api`, catalog stream in `make test-e2e-heavy`. |
| **III. API Consistency** — `/v1/<resource>`, two-tier auth, REST API table in CLAUDE.md updated | ☑ | New rows: batch catalog ingest, reconciliation read, readiness read (phases 2/4). Admin-plane changes (bearer, catalog-on-upsert) update existing rows. `RankedItem.scored` + `no_subject_vector` documented on the rankings row. |
| **IV. Performance** — Redis cache plan, batch phases non-blocking, cold-start fallback | ☑ | `Rank` is uncached by design (background caller, computed response). Cold start untouched — zero-event subjects keep `rankFallback`, now visibly via `no_subject_vector`. Cron phases unchanged. |

## Project Structure

### Documentation (this feature)

```text
specs/006-darkvoid-alignment/
├── design.md            # Authoritative: problems P1–P9, decisions D1–D6 + k + warning tier
├── plan.md              # This file
└── tasks.md             # /speckit.tasks output (next step)
```

### Source (repo areas touched, by phase)

```text
Phase 1 — Rank revision (unblocks DarkVoid spec 006):
  internal/recommend/service.go        # blend helper extraction, dense path in Rank,
                                       # x/(x+k) sparse normalization, filters, fallback source
  internal/recommend/types.go          # SourceNoSubjectVector
  pkg/codohuetypes/recommend.go        # RankedItem.Scored; golden regen
  pkg/codohuetypes/                    # dense_source constants (used by internal/ too)
  sdk/go/rank.go                       # surface Scored + new source

Phase 2 — catalog-path durability:
  internal/ingest/                     # catalog stream consumer (wired to catalog svc in cmd/api)
  internal/catalog/                    # batch ingest handler, reconciliation read
  sdk/go/redistream/                   # catalog publisher, symmetric with events
  cmd/api/                             # wiring: stream consumer → catalog service adapter

Phase 3 — provisioning the core mode:
  internal/admin/                      # catalog-on-upsert validation, bearer auth + rate limit
  sdk/go/admin/                        # new module or package: ProvisionCatalogNamespace

Phase 4 — hardening:
  internal/recommend/service.go        # downgrade warning log
  internal/admin/                      # overview alert (empty subjects_dense vs dense config)
  sdk/go/embedding.go                  # PutObject
  internal/recommend/ + cmd/api        # readiness read (only if DarkVoid gates on it)
```

## Phase ordering and gates

1. **Phase 1** ships as one unit: blend helper + normalization + filters + contract fields
   + constants, one golden regen, one SDK release. Nothing in it is independently useful to
   DarkVoid, and splitting it means two SDK bumps.
2. **Phase 2** starts with the stream (removes the loss class), then batch ingest (repair
   cost), then reconciliation (repair precision). Independently shippable pieces.
3. **Phase 3** and **4** have no ordering dependency on each other; phase 4's warning is
   cheap and may be pulled forward if convenient.
4. **Measurement gate (D4)**: the candidate cap changes only after benchmarking `Rank`
   `HasID` filters at 500/1000/2000 points against a real Qdrant.
5. **Out of scope** (design Non-goals): deprecating other modes, `VIEW` events, a new
   embedding strategy, any migration.
