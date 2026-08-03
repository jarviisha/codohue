# Tasks: Align Codohue with the DarkVoid integration

**Input**: Design documents from `/specs/006-darkvoid-alignment/`
**Prerequisites**: plan.md, spec.md, design.md (authoritative rationale; decisions D1–D6 resolved)

**Tests**: Included — the project constitution requires a `_test.go` for every touched
`service.go` / `repository.go` / `worker.go` (CLAUDE.md "Test files" convention), so test
tasks are mandatory here, not optional.

**Organization**: Grouped by user story from spec.md. US1+US2+US3 form one SDK release
(single contract revision); US4–US6 are independently shippable.

**Consumer-side prerequisite (not a task in this repo)**: DarkVoid flips
`CODOHUE_DENSE_SOURCE=catalog` and runs `darkvoidctl codohue reindex`. Its integration is
already fully mode-aware; nothing below depends on when this happens, but US1 is only
*observable end-to-end* for DarkVoid after the flip.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: US1–US6 from spec.md
- Line references are as of branch point `9a06d3e` and will drift as edits land

---

## Phase 1: Setup (pre-change safety net)

**Purpose**: Freeze current behaviour so the refactor is provably ordering-neutral (SC-009)

- [x] T001 Add an ordering-fixture test to internal/recommend/service_test.go: fixed
      sparse+dense inputs through `hybridRecommend`, asserting the exact result ordering
      (not score values). Commit it green against current code *before* any refactor —
      T004/T005 must keep it green.

---

## Phase 2: Foundational (blocking prerequisites)

**Purpose**: Shared constants every later story references

- [x] T002 [P] Export `DenseSource` string constants (`disabled`, `item2vec`, `svd`,
      `byoe`, `catalog`) from pkg/codohuetypes (new file pkg/codohuetypes/densesource.go
      with doc comment; additive, no golden snapshot — not a marshaled wire type) (FR-008)
- [x] T003 Replace bare dense-source string literals across internal/ with the
      codohuetypes constants: internal/recommend/service.go (:188, :359),
      internal/compute/job.go (`phase2Runs` :512), internal/compute/cleanup.go,
      internal/nsconfig (validation), internal/admin (catalog-source checks). No behaviour
      change; existing tests stay green.

**Checkpoint**: Constants in place — user story phases can begin

---

## Phase 3: User Story 1 — Background re-rank gets meaningful scores (P1) 🎯 MVP

**Goal**: `Rank` blends dense + sparse exactly like `Recommend` (D1: namespace `alpha`),
with batch-independent normalization so chunked calls stay comparable (FR-001–FR-003)

**Independent Test**: Seed a catalog-mode namespace with embedded content and a subject
with 2–3 likes; rank a mixed candidate list → liked-similar items outscore unrelated ones;
two 500-item chunks merge into the same ordering as one union call (SC-001, SC-002)

- [x] T004 [US1] Extract the blend body of `hybridRecommend`
      (internal/recommend/service.go:436-479 — normalize → union candidates → α-blend →
      γ-decay) into a shared helper taking sparse/dense score maps + cfg; `hybridRecommend`
      delegates to it. T001 fixture must stay green.
- [x] T005 [US1] Replace min-max sparse normalization with the fixed saturating map
      `x/(x+k)` (`k` = package-level constant, decided global; document the tuning basis
      in a comment) inside the shared helper; dense cosine scores pass through unscaled
      (FR-003). Update `normalizeScores` call sites; T001 ordering fixture stays green.
- [x] T006 [US1] Wire the dense side into `Rank` (internal/recommend/service.go:1163-1255):
      fetch subject dense vector via `fetchSubjectDenseVecFn`, search `{ns}_objects_dense`
      with the same `HasID` candidate filter via `searchObjectsDenseFn`, feed both result
      sets through the shared helper, keep `rerankScored`'s γ handling consistent with it.
- [x] T007 [US1] Define the one-sided degradations in `Rank` (FR-002): no dense vector →
      sparse-only (as today); no sparse vector but dense present → dense-only scoring
      instead of `rankFallback`'s whole-list zero return.
- [x] T008 [US1] Unit tests in internal/recommend/service_test.go: hybrid blend in Rank
      (both sides), dense-only path, sparse-only path, chunk-comparability (rank 1000
      candidates as 2×500 vs 1×1000 → same merged ordering), α respected from cfg.
- [ ] T009 [US1] Extend the rank e2e flow (e2e/, `make test-e2e-api` subset) with a hybrid
      case: catalog-embedded objects + events → rank returns differentiated non-zero
      scores.

**Checkpoint**: Rank returns meaningful hybrid scores — server-side MVP shippable

---

## Phase 4: User Story 2 — Caller can tell "unscored" from "irrelevant" (P1)

**Goal**: Distinct whole-response source for the no-vector fallback + per-item `scored`
flag; every candidate still returned (D2, FR-004, FR-006)

**Independent Test**: Rank for an unknown subject → source `no_subject_vector`; rank with
one never-indexed candidate → that item `scored:false`, others `scored:true`; old decoder
still parses the response

- [x] T010 [P] [US2] Add `Scored bool \`json:"scored"\`` to `RankedItem` in
      pkg/codohuetypes/recommend.go and update the doc comment at :36-37 that currently
      collapses the three zero-score cases into one.
- [x] T011 [P] [US2] Add `SourceNoSubjectVector = "no_subject_vector"` beside the existing
      source constants in internal/recommend/types.go:46-50.
- [x] T012 [US2] Set the new semantics in internal/recommend/service.go: `rankFallback`
      reports `SourceNoSubjectVector` with all items `Scored:false`; the scored/appended
      split in `Rank` (:1230-1245) marks scored items true, zero-fill appends false.
- [x] T013 [US2] Regenerate golden snapshots
      (`go test ./pkg/codohuetypes/... -run Golden -update`) and commit the
      pkg/codohuetypes/testdata/ diff for review (FR-006).
- [x] T014 [P] [US2] Surface `Scored` and document both source values in sdk/go/rank.go +
      sdk/go/rank_test.go.
- [x] T015 [US2] Tests distinguishing the three former-ambiguous outcomes (no subject
      vector / not indexed / zero overlap) in internal/recommend/service_test.go and
      handler_test.go.

**Checkpoint**: Contract revision complete — with US1 this is the SDK release DarkVoid needs

---

## Phase 5: User Story 3 — One notion of eligibility across read surfaces (P2)

**Goal**: `Rank` applies the same seen-items + `exclude_authored` exclusions as
`Recommend`, unconditionally (D3, FR-005); excluded candidates return `scored:false`

**Independent Test**: Enable `exclude_authored`, attribute an object to the subject, send
it as a rankings candidate → present in response, `scored:false` (SC-004)

- [x] T016 [US3] Build the exclusion set in `Rank` via the existing `excludedObjectIDs`
      (internal/recommend/service.go:974) and merge its `MustNot` conditions into the
      candidate `HasID` filter (:1210-1214) for both sparse and dense searches.
- [x] T017 [US3] Return excluded candidates as `scored:false` in request order (never
      dropped); on exclusion-set lookup failure, degrade to unfiltered scoring — same
      posture `Recommend` takes today.
- [x] T018 [US3] Tests in internal/recommend/service_test.go: seen-item excluded,
      authored-object excluded (flag on), flag off → scored normally, eligibility parity
      with `Recommend` for identical config, lookup-failure degradation.

**Checkpoint**: Both read surfaces share one eligibility definition

---

## Phase 6: User Story 4 — Catalog content survives an outage (P2)

**Goal**: Durable stream transport for catalog content (D5: consumed by the `ingest`
worker), batch HTTP ingest, reconciliation read (FR-009–FR-013)

**Independent Test**: Publish catalog content to the stream while `cmd/api` is down; start
it; content is embedded with zero producer retries (SC-005); 10k-item repair ≤ 100
requests (SC-006)

- [x] T019 [P] [US4] Define the stream contract: stream name (`codohue:catalog`, namespace
      inside the payload — same reasoning as events) and a `CatalogStreamItem` wire type in
      pkg/codohuetypes/catalog.go; add the golden-test case + snapshot. Trim policy is
      explicit and symmetric with `codohue:events`: no producer-side MAXLEN (durability is
      the point — entries live until consumed and acked; the internal `catalog:embed:{ns}`
      stream keeps its existing approximate cap of 100k downstream, see
      internal/catalog/service.go:179).
- [x] T020 [P] [US4] Catalog publisher in sdk/go/redistream mirroring the event
      publisher's ergonomics (+ tests) (FR-010).
- [x] T021 [US4] Consume the catalog stream in the ingest worker (internal/ingest):
      XREADGROUP with the existing consumer-name convention, validation identical to the
      HTTP path (namespace in catalog mode, size caps, required fields), invalid items
      observably rejected, redelivery idempotent by object id (FR-013). "Observably
      rejected" is concrete: invalid items are acked off the stream and recorded through
      the existing catalog failure machinery (`last_error` + dead-letter state where a
      `catalog_items` row exists; warning log + ingest failure metric where none can — e.g.
      unknown namespace), so they surface in the admin failures-summary, never silently
      dropped. The worker calls a narrow `CatalogIngestor` interface — internal/ingest
      must NOT import internal/catalog.
- [x] T022 [US4] Wire the adapter in cmd/api: inject the internal/catalog service into the
      ingest worker behind the T021 interface (same pattern as
      cmd/admin/nsconfig_adapter.go); update internal/ingest/docs.go and worker tests.
- [x] T023 [US4] Batch HTTP ingest on internal/catalog: accept an item array on the
      catalog path (new wire request/response types in pkg/codohuetypes/catalog.go +
      golden cases, `DecodeStrict`, per-item results so one bad item doesn't fail the
      batch), capped at 100 items per request (100 × the 32KB per-item content cap ≈
      3.2MB request bound; satisfies SC-006's ≥100 batch size) (FR-011). Add the
      endpoint's REST API table row to CLAUDE.md in the same change (constitution III:
      the row ships with the endpoint's PR, not in polish).
- [x] T024 [US4] Data-plane reconciliation read in internal/catalog: list held object ids
      / changed-since-timestamp for the namespace, paginated; wire type + golden case
      (FR-012). Add the endpoint's CLAUDE.md REST API table row in the same change.
- [x] T025 [US4] Batch + reconciliation SDK wrappers in sdk/go/catalog_http.go (+ tests).
- [x] T026 [US4] Tests: internal/ingest worker (stream consume, validation parity,
      idempotent redelivery), internal/catalog service/repository (batch, reconciliation);
      e2e outage scenario in the `make test-e2e-heavy` subset (publish during downtime →
      recover → embedded).

**Checkpoint**: Core-mode ingest has no fire-and-forget loss class

---

## Phase 7: User Story 5 — Provisioning the core mode is one supported call (P3)

**Goal**: Bearer admin auth (D6: existing key + failed-attempt rate limit),
`dense_source="catalog"` accepted on the namespace upsert with strategy fields, supported
SDK provisioning call (FR-014–FR-016)

**Independent Test**: One bearer-authenticated upsert with strategy fields provisions a
catalog namespace (SC-007); dim mismatch rejected identically to the catalog endpoint;
repeated bad bearers throttled, correct key never

- [ ] T027 [P] [US5] Bearer auth on `/api/admin/v1/*` in internal/admin: accept
      `Authorization: Bearer <CODOHUE_ADMIN_API_KEY>` beside the session cookie
      (constant-time compare; documented precedence: bearer wins when both present), with
      a per-IP failed-attempts rate limiter reusing the login limiter's shape (FR-014).
- [ ] T028 [US5] Accept `dense_source="catalog"` on `PUT /api/admin/v1/namespaces/{ns}`
      when catalog strategy id/version accompany it, running the same dim-vs-embedding_dim
      validation as the catalog endpoint; absent fields → reject naming them (replaces the
      blanket `ErrCatalogSourceViaUpsert` at internal/admin/types.go:195,
      handler.go:228-230, service.go:275) (FR-015).
- [ ] T029 [US5] New sdk/go/admin package: bearer-authenticated admin client with
      `ProvisionCatalogNamespace` (one call: upsert with strategy fields) (FR-016).
- [ ] T030 [US5] Tests: internal/admin auth (bearer valid/invalid/throttled/cookie
      precedence), upsert validation matrix (catalog + fields ok / catalog − fields
      rejected / dim mismatch), sdk/go/admin against a stub server; admin-plane e2e
      (`make test-e2e-heavy`). Update the affected CLAUDE.md admin-route rows (auth
      column + upsert semantics) in the same change.

**Checkpoint**: The recommended configuration is the paved road

---

## Phase 8: User Story 6 — Silent degradation becomes visible (P3)

**Goal**: Hybrid-configured namespaces with empty subject dense vectors surface as
warnings + admin alerts; SDK reaches object metadata; optional readiness read
(FR-017–FR-019)

**Independent Test**: Configure hybrid scoring with an empty `{ns}_subjects_dense` → admin
overview shows the alert (SC-008); populate vectors → alert clears

- [ ] T031 [P] [US6] Upgrade the silent fallback to warning level in
      internal/recommend/service.go (the Debug at :365 and the equivalent Rank path once
      US1 lands) (FR-017).
- [ ] T032 [US6] Admin overview alert in internal/admin: flag namespaces with
      `alpha < 1 && dense_source != disabled` and zero points in `{ns}_subjects_dense`
      (the overview already aggregates per-namespace Qdrant counts + an alerts list);
      service + repository tests. No new Prometheus metric (decided tier).
- [ ] T033 [P] [US6] `PutObject` wrapper in sdk/go (`PUT /v1/namespaces/{ns}/objects/{id}`,
      `ObjectUpsertRequest`/`ObjectResponse` already exist in pkg/codohuetypes) in
      sdk/go/embedding.go + tests (FR-018).
- [ ] T034 [US6] **Deferred — build only when DarkVoid commits to gating on it** (FR-019):
      data-plane readiness read (indexed object count, subject sparse/dense vector
      presence, last successful recompute, catalog backlog) — new endpoint + wire type +
      golden case + SDK wrapper + CLAUDE.md row.

**Checkpoint**: No supported mode fails silently

---

## Final Phase: Polish & Cross-Cutting

- [ ] T035 [P] Update CLAUDE.md: rankings row annotations (`scored` +
      `no_subject_vector` + filters) and Key Design Decisions (catalog-as-core,
      batch-independent normalization, shared eligibility). New-endpoint table rows are
      NOT here — they ship inside T023/T024/T030 per constitution III.
- [ ] T036 [P] D4 measurement gate: benchmark `Rank` with `HasID` filters at 500/1000/2000
      points against a real Qdrant; record results + cap decision in
      specs/006-darkvoid-alignment/benchmarks.md. Cap changes only from this data
      (FR-007).
- [ ] T037 Release: regenerate/verify goldens once, release notes calling out the one-time
      score-value shift on `/recommendations`, coordinated module tags per the release
      process (push v* tag alone — >3 tags at once suppresses CI).
- [ ] T038 Full gate: `make lint`, `make coverage-check-all`, `make test-e2e` green across
      all modules.

---

## Dependencies

```text
T001 (fixture) ──► Phase 3 (must exist before the refactor starts)
T002 ──► T003 ──► all story phases (constants referenced everywhere)

US1 (T004→T005→T006→T007→T008→T009)   ─┐
US2 (T010,T011 ∥ → T012→T013→T014,T015)◄┤ US2 depends on US1 (fallback/source semantics)
US3 (T016→T017→T018)                   ◄┘ US3 depends on US2 (scored flag)

US4 (T019 ∥ T020 → T021→T022→T023→T024→T025→T026)  — independent of US1–US3
US5 (T027 ∥ T028 → T029→T030)                       — independent of US1–US4
US6 (T031 ∥ T033 anytime; T032 independent; T034 deferred)

T035–T038 close every completed phase; T037 requires US1+US2+US3 (the SDK release).
```

- **Sequential core**: US1 → US2 → US3 is one contract revision released together (one
  SDK bump — plan.md constraint).
- **Parallel tracks**: US4, US5, and US6 each touch disjoint domains and can proceed in
  parallel with the US1–US3 chain and with each other.

## Parallel Example

```text
Track A (recommend):  T001 → T004 → T005 → T006 → T007 → T008 → T009 → US2 → US3
Track B (catalog):    T019 ∥ T020 → T021 → T022 → T023 → T024 → T025 → T026
Track C (admin/sdk):  T027 ∥ T028 → T029 → T030;  T031 ∥ T032 ∥ T033 anytime
```

## Implementation Strategy

**MVP = Phase 1 + 2 + 3 (US1)**: server-side hybrid Rank is shippable alone — scores
change value (allowed once), shape unchanged. **First SDK release = US1+US2+US3** — the
unit that unblocks DarkVoid spec 006. Then US4 (largest, promoted by catalog-as-core),
then US5/US6. T034 stays deferred until the consumer asks; T036 gates any cap change on
measurement.
