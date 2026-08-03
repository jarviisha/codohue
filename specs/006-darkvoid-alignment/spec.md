# Feature Specification: Align Codohue with the DarkVoid integration

**Feature Branch**: `feat/recommend-darkvoid-alignment`
**Created**: 2026-08-03
**Status**: Accepted — derived from the accepted [design.md](design.md) (rev 4, all design
decisions resolved); this spec restates it in requirement form for task generation
**Input**: User description: "Generate spec.md for the existing feature
specs/006-darkvoid-alignment. Source material: the accepted design.md in that directory."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Background re-rank gets meaningful scores (Priority: P1)

DarkVoid's materialized-feed job periodically sends a user's timeline (a list of candidate
item ids) to the rankings endpoint and blends the returned relevance scores into its own
feed ordering. Today the endpoint scores only from co-interaction overlap; with DarkVoid's
sparse event coverage (no VIEW events), nearly every candidate comes back 0 and the call is
a paid no-op. After this feature, a namespace with dense vectors configured gets the same
hybrid (sparse + content-similarity) scoring on rankings that it already gets on
recommendations, so a user with only a handful of likes still receives differentiated
scores over their timeline.

**Why this priority**: This is the single blocker for DarkVoid's spec 006; every other item
exists in service of it or hardens around it.

**Independent Test**: Seed a namespace (catalog mode) with catalog content and a subject
with 2–3 like events; request rankings over a candidate list containing liked-similar and
unrelated items; scores must differentiate the two groups, and disabling the dense side
must reproduce today's sparse-only behaviour.

**Acceptance Scenarios**:

1. **Given** a namespace with dense vectors present for subject and candidates, **When**
   rankings are requested, **Then** returned scores blend sparse and dense signals per the
   namespace's configured blend weight (`alpha`), with the same freshness decay as today.
2. **Given** a subject with a dense vector but no sparse vector, **When** rankings are
   requested, **Then** candidates are scored dense-only (not returned as an unscored
   whole-list fallback).
3. **Given** a subject with neither vector, **When** rankings are requested, **Then** all
   candidates return unscored in request order, and the response says so explicitly (see
   Story 2).
4. **Given** the same timeline split across two rankings calls, **When** both responses are
   merged by score, **Then** the merged ordering is consistent with the ordering a single
   call over the union would produce (scores are comparable across calls).

---

### User Story 2 - Caller can tell "unscored" from "irrelevant" (Priority: P1)

DarkVoid's blend rule is "CF score > 0 → blend, else keep local score". Today a 0 can mean
the subject is unknown, the candidate was never indexed, or the candidate is genuinely
irrelevant — three cases with different correct caller behaviour. After this feature the
response distinguishes them: a whole-response marker when the subject has no vector at
all, and a per-item marker for candidates returned unscored.

**Why this priority**: Ships in the same contract revision as Story 1; it is also the only
external instrument that makes a silently degraded namespace visible to its consumer.

**Independent Test**: Request rankings for an unknown subject (whole-response marker set),
then for a known subject with one never-indexed candidate (that item marked unscored,
others scored).

**Acceptance Scenarios**:

1. **Given** a subject with no vector, **When** rankings are requested, **Then** the
   response source field carries a distinct "no subject vector" value instead of the
   normal one.
2. **Given** a mixed candidate list, **When** rankings are requested, **Then** every item
   carries a scored/unscored marker and the full candidate list still comes back.
3. **Given** an SDK consumer compiled against the previous types, **When** it decodes the
   new response, **Then** decoding still succeeds (additive change only).

---

### User Story 3 - One notion of eligibility across read surfaces (Priority: P2)

An operator configures "exclude items this user has recently seen" and "exclude the user's
own authored items" once per namespace. Today those exclusions apply to recommendations but
not to rankings, so the same user sees their own posts filtered from one surface and ranked
normally on the other. After this feature both endpoints apply the same exclusion set;
excluded candidates come back marked unscored rather than dropped.

**Why this priority**: Correctness/consistency; depends on the Story 1+2 contract work and
is part of the same release.

**Independent Test**: Enable `exclude_authored`, attribute an object to the subject, send
it in a rankings candidate list; it must return unscored while still present in the
response.

**Acceptance Scenarios**:

1. **Given** a namespace with seen-item exclusion active, **When** a recently seen item is
   sent as a rankings candidate, **Then** it returns marked unscored.
2. **Given** `exclude_authored` enabled and an object attributed to the requesting subject,
   **When** it is sent as a rankings candidate, **Then** it returns marked unscored.
3. **Given** the same namespace config, **When** an item is eligible on recommendations,
   **Then** it is eligible on rankings (and vice versa).

---

### User Story 4 - Catalog content survives an outage (Priority: P2)

Catalog mode is now the system's core mode, but its ingest path is fire-and-forget HTTP: a
post created while Codohue is unreachable is absent from recommendations forever, and the
consumer's only remedy is a full-corpus re-send. Behavioural events already have a durable
queue transport; catalog content gets the symmetric treatment: a durable stream transport,
a batch ingest for repair walks, and a way to ask which objects the namespace already
holds so repairs re-send only the gap.

**Why this priority**: Promoted by the catalog-as-core decision — the primary write path
cannot silently drop content — but independent of the Story 1–3 release.

**Independent Test**: Publish catalog content to the stream while the API server is down;
bring it up; the content must be embedded and appear in dense retrieval with no consumer
retry.

**Acceptance Scenarios**:

1. **Given** catalog content published to the durable transport during a Codohue outage,
   **When** Codohue recovers, **Then** the content is ingested and embedded without any
   action from the producer.
2. **Given** a repair pass over N items, **When** batch ingest is used, **Then** the number
   of requests is O(N / batch size) rather than N.
3. **Given** a partially ingested corpus, **When** the reconciliation read is queried,
   **Then** the consumer can identify exactly the missing/changed object ids and re-send
   only those.

---

### User Story 5 - Provisioning the core mode is one supported call (Priority: P3)

An automated provisioner setting up a catalog-mode namespace today must impersonate a
browser (exchange the admin key for a session cookie) and issue two requests because the
core mode's value is rejected on the namespace upsert. After this feature, admin routes
accept the admin key directly as a bearer token (rate-limited on failures), the namespace
upsert accepts catalog mode when the strategy fields accompany it (same validation as the
dedicated endpoint), and the SDK offers a supported provisioning call.

**Why this priority**: Ergonomics; no correctness impact. The decided trade-off (bearer =
the existing admin key, no separate automation credential) is acceptable while there is a
single internal consumer.

**Independent Test**: Provision a catalog-mode namespace with one bearer-authenticated
upsert carrying strategy fields; verify a dim mismatch is rejected with the same error the
dedicated endpoint gives; verify repeated bad bearer attempts from one IP are throttled
while correct ones never are.

**Acceptance Scenarios**:

1. **Given** a request with the admin key as bearer token, **When** it hits any admin
   route, **Then** it is authenticated without a session cookie; the browser session flow
   is unchanged.
2. **Given** a namespace upsert selecting catalog mode with strategy id/version present,
   **When** dims validate, **Then** the namespace is created/updated in that one request;
   with strategy fields absent, the request is rejected with an error naming the missing
   fields.
3. **Given** repeated failed bearer attempts from one IP, **When** the threshold is hit,
   **Then** further attempts are throttled; a correct key is never throttled.

---

### User Story 6 - Silent degradation becomes visible (Priority: P3)

A namespace configured for hybrid scoring whose subject dense vectors are absent (the
standing state of every bring-your-own-embedding namespace that never pushes subject
vectors) currently serves sparse-only results with a debug-level log — operator-invisible.
After this feature the condition produces a warning log and an alert on the admin
overview. Related low-priority reads: the SDK gains the object-metadata write (so authored
exclusion is reachable under non-catalog modes), and a data-plane readiness read lets a
consumer ask "do you have data for this namespace / this subject" before running a job.

**Why this priority**: Hardening for the modes that remain optional; the core-mode
migration itself removes the state for catalog namespaces.

**Independent Test**: Configure a namespace with hybrid scoring and an empty subject dense
collection; the admin overview must show an alert for it; populate the collection; the
alert must clear.

**Acceptance Scenarios**:

1. **Given** a namespace with hybrid scoring configured and no subject dense vectors,
   **When** the admin overview is loaded, **Then** it shows an alert identifying the
   namespace and the condition, and the recommend path logs at warning level.
2. **Given** the SDK, **When** a client sets or clears an object's author under any dense
   mode, **Then** the metadata write succeeds through a supported SDK call.
3. **Given** the readiness read, **When** queried with data-plane credentials, **Then** it
   reports indexed object count, per-subject vector presence (sparse and dense
   separately), last successful recompute, and catalog backlog size.

---

### Edge Cases

- Rankings for an empty candidate list → empty items, normal source value (unchanged).
- Candidate list where every item is excluded by filters → full list returned, all
  unscored; whole-response source stays the normal value (the subject has a vector).
- Exclusion-set failure (store error) degrades to unfiltered scoring rather than failing
  the request — same posture recommendations take today.
- Two chunks of one timeline ranked while a recompute lands between the calls: scores stay
  comparable (normalization is batch-independent and its constant is fixed, not derived
  from per-tick statistics).
- Stream-ingested catalog content for a namespace not in catalog mode → rejected/dead-
  lettered the same way the HTTP path rejects it, not silently dropped.
- Duplicate delivery on the catalog stream (producer retry) → idempotent by object id, no
  duplicate embed work beyond the existing content-hash short-circuit.
- Bearer auth and session cookie both present → bearer wins deterministically (documented
  precedence), no 500.
- A subject with events referencing objects that have no dense vectors (catalog backlog
  not yet drained) → treated as "no dense contribution yet", sparse-only for that subject,
  not an error.

## Requirements *(mandatory)*

### Functional Requirements

**Rankings scoring (Stories 1, 2, 3 — one contract revision)**

- **FR-001**: Rankings MUST score candidates with the same hybrid blend recommendations
  use: sparse and dense signals blended by the namespace's configured `alpha`, then the
  existing freshness decay. The blend logic MUST be shared, not duplicated (decision D1:
  namespace `alpha`, no per-request override).
- **FR-002**: When only one signal side exists for a subject, rankings MUST degrade to
  that side alone: sparse-only (as today) or dense-only (replacing today's whole-list
  fallback).
- **FR-003**: Score normalization MUST be batch-independent: dense scores keep their
  natural bounded scale; sparse scores pass through a fixed saturating mapping whose
  constant is global and compile-time (decided: not per-namespace, not adaptive), so
  scores from separate requests are comparable.
- **FR-004**: The rankings response MUST carry a distinct source value when the subject
  has no vector at all, and every ranked item MUST carry a scored/unscored boolean.
  Every submitted candidate MUST still be returned (decision D2).
- **FR-005**: Rankings MUST apply the namespace's seen-items and authored-objects
  exclusions unconditionally (decision D3), returning excluded candidates as unscored
  rather than dropping them.
- **FR-006**: The contract change MUST be additive: existing decoders keep working, golden
  snapshots are regenerated once, and the SDK ships the new fields in a single release
  together with FR-001–FR-005.
- **FR-007**: The candidate cap stays at its current value (500); any change MUST be
  justified by a measured retrieval-cost benchmark at 500/1000/2000 candidates
  (decision D4).
- **FR-008**: The dense-mode identifiers (`disabled`/`item2vec`/`svd`/`byoe`/`catalog`)
  MUST be exported as shared constants from the wire-types module and used by the server
  domains, replacing bare string literals.

**Catalog-path durability (Story 4)**

- **FR-009**: Codohue MUST accept catalog content over a durable stream transport
  symmetric with the behavioural-events stream: producer-side fire-and-forget, consumed by
  the existing ingest worker (decision D5), feeding the existing catalog persistence and
  embed pipeline unchanged.
- **FR-010**: The SDK's stream transport module MUST offer a catalog publisher with the
  same ergonomics as the existing event publisher.
- **FR-011**: The HTTP catalog path MUST accept batched ingest so a corpus repair walk
  costs O(corpus / batch size) requests. Batches are capped at 100 items per request
  (bounding request size at 100 × the per-item content cap), with per-item results so one
  invalid item does not fail the batch.
- **FR-012**: A data-plane read MUST let an authenticated consumer enumerate which object
  ids the namespace holds (or which changed since a timestamp) so repairs re-send only
  the gap.
- **FR-013**: Stream-delivered catalog content MUST be validated identically to
  HTTP-delivered content (namespace mode, size caps, required fields); invalid items MUST
  be observably rejected, not silently dropped — concretely: recorded through the existing
  catalog failure surfaces (failure state + failure reason, visible in the admin failures
  summary) where a catalog record exists, and via warning log + failure metric where none
  can (e.g. unknown namespace) — and redelivery MUST be idempotent.

**Provisioning and admin access (Story 5)**

- **FR-014**: Admin routes MUST accept the global admin key as a bearer token alongside
  the session cookie (decision D6: the existing key, no separate automation credential),
  with failed attempts rate-limited per IP and successful ones never throttled.
- **FR-015**: The namespace upsert MUST accept catalog mode when strategy id/version
  accompany it, applying the same dimension validation as the dedicated catalog endpoint;
  when they are absent it MUST reject with an error naming the missing fields.
- **FR-016**: The SDK MUST offer a supported provisioning call that sets up a catalog-mode
  namespace in one invocation.

**Hardening and observability (Story 6)**

- **FR-017**: When a namespace has hybrid scoring configured and no subject dense vectors
  exist, the system MUST emit a warning-level log on the serving path and an alert on the
  admin overview (decided tier: log + overview alert; no new metric — the existing
  per-source request counters already expose the fallback rate).
- **FR-018**: The SDK MUST offer the object-metadata write (set/clear author attribution)
  that the API already exposes, under every dense mode.
- **FR-019**: A data-plane readiness read MUST report, per namespace: indexed object
  count, requesting subject's vector presence (sparse and dense separately), last
  successful recompute time, and catalog backlog size. (Lowest priority; build when the
  consumer commits to gating on it.)

### Key Entities

- **Ranking request/response**: a subject plus an ordered candidate list in; the full
  list back, each item with score, rank, and a scored/unscored marker; a response-level
  source describing how scoring was performed (normal, or no-subject-vector fallback).
- **Namespace configuration**: per-tenant settings — dense mode (catalog is the core
  mode), blend weight, freshness decay, exclusion toggles — already existing; this feature
  changes which surfaces honour it, not its shape.
- **Catalog item**: raw content submitted for server-side embedding; gains a second,
  durable arrival path (stream) and a batched arrival path (HTTP), same lifecycle
  afterwards.
- **Object metadata**: per-object facts independent of embedding (author attribution);
  gains SDK reach.
- **Readiness summary**: per-namespace data-plane view — object count, subject vector
  presence, last recompute, backlog size.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a subject with at least one behavioural interaction in a catalog-mode
  namespace, ranking a mixed candidate list yields differentiated scores (not all-zero)
  covering ≥ the fraction of candidates that have embedded content.
- **SC-002**: A 1000-item timeline ranked in two 500-item calls merges into the same
  ordering a single call over the union would give, within score-tie noise.
- **SC-003**: A consumer can compute CF coverage (scored candidates / submitted
  candidates) and detect "namespace knows nothing about this subject" from the response
  alone, with zero additional round trips.
- **SC-004**: An item excluded on the recommendations surface is never scored on the
  rankings surface for the same subject and config, across 100% of eligibility cases.
- **SC-005**: Catalog content produced during a full Codohue outage of any duration is
  embedded after recovery with zero producer retries and zero operator action — the
  client-facing stream is not producer-trimmed (symmetric with the events stream); entries
  persist until consumed and acknowledged.
- **SC-006**: A full-corpus repair pass of 10,000 items completes in ≤ 100 ingest requests
  (batch ≥ 100), and an incremental repair re-sends only the differing items.
- **SC-007**: Provisioning a catalog-mode namespace succeeds with exactly one
  authenticated configuration request (down from a login + two configuration requests).
- **SC-008**: 100% of namespaces serving sparse-only while configured for hybrid scoring
  are visible as alerts on the admin overview within one refresh interval.
- **SC-009**: Recommendations output ordering for existing namespaces is unchanged by the
  shared-blend refactor (score values may shift once, ordering must not), verified against
  recorded pre-change fixtures.

## Assumptions

- DarkVoid flips its integration to catalog mode (config change + corpus reindex on its
  side) before or alongside adopting the new rankings semantics; its integration is
  already fully mode-aware, so this is a consumer-side operational step, not development
  work in this repo.
- One internal consumer (DarkVoid) for the foreseeable future: this is what makes the
  decided bearer-token trade-off (existing admin key, fleet-wide rotation on leak)
  acceptable; revisit if a second consumer or external operators appear.
- The sparse-normalization constant has a single global default tuned against the demo
  dataset and DarkVoid's corpus; per-namespace tuning is deliberately deferred until a
  namespace demonstrably needs it (it would require a schema change, which this feature
  forbids).
- No database schema migration anywhere in this feature.
- Other dense modes (`item2vec`, `svd`, `byoe`) remain supported options; making `byoe`
  self-sufficient (server-side subject vector pooling) is explicitly deferred until a
  consumer commits to that mode long-term.
- Emitting VIEW events is DarkVoid's responsibility and out of scope; this feature makes
  the dense signal usable despite their absence, it does not repair sparse coverage.
