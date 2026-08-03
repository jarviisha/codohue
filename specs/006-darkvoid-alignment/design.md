# Design: Align Codohue with the DarkVoid integration

Status: Accepted — all open questions decided 2026-08-03, nothing implemented yet
Date: 2026-07-31 (rev 3: 2026-08-03 — catalog becomes the core mode; rev 4:
2026-08-03 — D1–D6 plus the `k` constant and the P7 warning tier decided)

## Context

DarkVoid is Codohue's only consumer today, and it is about to change how it uses
Codohue. Its `specs/006-materialized-ranked-feed` moves *all* feed ranking out of
the request path into a background re-rank job whose core step is
`POST /v1/namespaces/{ns}/rankings` (DarkVoid `design.md` §4.2). Until now
DarkVoid has only used `Recommend` and `Trending`; `Rank` is wired in
`pkg/codohue/client.go:141` but has **no production caller**.

That shift moves the least-exercised endpoint in this repo onto the critical
path, and reading DarkVoid's integration end to end surfaces more places where
Codohue's shape forces the consumer into a workaround. This document collects
them so the order of work can be decided before any code changes.

Consumer-side references below point at
`/home/jarviisha/development/darkvoid`.

## Direction: catalog auto-embedding is the core mode

**Product decision (rev 3).** `dense_source="catalog"` is the primary,
recommended mode of the system. The other phase-2 producers — `item2vec`,
`svd`, `byoe` — remain supported as options, but they are options: the default
path a consumer is steered onto is "send raw content, Codohue owns embedding".

Why catalog, concretely:

- **It is the only mode with no silent half.** Under `catalog`, `cmd/embedder`
  fills `{ns}_objects_dense` and `cmd/cron` phase 2 mean-pools
  `{ns}_subjects_dense` from it (`internal/compute/job.go:507-518`). Under
  `byoe` — DarkVoid's current default — phase 2 is skipped, nothing writes
  `{ns}_subjects_dense` unless the client does, DarkVoid's client never does
  (`UpsertSubjectEmbedding` has no production caller), and `Recommend` silently
  falls back to pure sparse at `Debug` level
  (`internal/recommend/service.go:359-365`). The config says hybrid; the service
  serves sparse-only. Catalog mode cannot get into that state.
- **Authorship flows natively.** `CatalogIngestRequest.AuthorSubjectID` is
  already written through to the `objects` table, so `exclude_authored` works
  with no extra client call. Under `byoe` it requires a `PutObject` the SDK does
  not have (P8).
- **Embedding ownership is where it can improve.** DarkVoid's client-side
  TF-IDF (`pkg/tfidf/vectorizer.go`, FNV hash mod dim) and Codohue's
  hashing+ngrams strategy are the same class of baseline. But only one of them
  sits behind the `embedstrategy` registry seam
  (`internal/embedder/strategy.go`), where a real model strategy can be added
  per-namespace and rolled out with the existing re-embed machinery
  (`strategy_version` re-drive). Client-side vectors can never be upgraded from
  Codohue's side.

**The consumer is already ready.** DarkVoid gates its entire integration on
`CODOHUE_DENSE_SOURCE` (`pkg/config/load.go:122`, default `"byoe"`): production
ingest (`internal/app/post.go:170`), provisioning
(`pkg/codohue/provision.go:155` enables catalog auto-embedding), and the
`darkvoidctl codohue reindex` repair pass (`cmd/darkvoidctl/codohue.go:201`) all
switch on it, and the catalog path already sends `author_subject_id`. Migration
is: set `CODOHUE_DENSE_SOURCE=catalog`, provision (dim must be one of the
registered variants 64/128/256/512), run `reindex` to ingest the corpus as
catalog items. No DarkVoid code change.

Two consequences of the decision run through the rest of this document:

- **Catalog-path durability is promoted** from "worth doing eventually" to
  core-path correctness (P5): the primary ingest transport cannot stay
  fire-and-forget lossy.
- **The byoe subject-vector gap is demoted** from blocker to hardening (P7):
  optional modes must fail loudly, but Codohue no longer needs to make `byoe`
  self-sufficient before DarkVoid's spec 006 can ship.

---

## Problem 1 — `Rank` ignores dense vectors while claiming to be hybrid

`Service.Rank` fetches the **sparse** subject vector and searches the **sparse**
`{ns}_objects` collection only:

- `internal/recommend/service.go:1187` — `fetchSubjectVecFn` → `*qdrant.SparseVector`
- `internal/recommend/service.go:1216` — `searchObjectsFn` (sparse collection)
- `internal/recommend/types.go:49` — yet the response reports `Source: "hybrid_rank"`

The `alpha` blend, dense retrieval, and normalization all live in
`hybridRecommend` (`service.go:400-518`) and are reachable only from `Recommend`.
So a namespace with `dense_source ∈ {item2vec, svd, byoe, catalog}` gets dense
scoring on `/recommendations` and pure sparse CF on `/rankings`.

**Why this blocks DarkVoid specifically.** DarkVoid does not emit `VIEW` events
yet — its own design says so plainly ("thiếu `VIEW` (signal lớn nhất) → 0
subject", DarkVoid `design.md` §10), and a grep confirms only `LIKE`, `COMMENT`
and `SKIP` reach `PublishBehaviorEvent`. Sparse CF needs a candidate to co-occur
with the subject through *other* subjects; on that interaction graph it returns 0
for nearly every candidate, so the re-rank job would be a no-op that still costs
a round trip per user.

The dense path is what generalizes from those few signals: a handful of likes
mean-pool into a subject vector, and content similarity carries it to items no
one the user overlaps with has touched. Under the catalog direction this works
end to end with no further prerequisite — the embedder fills the object side,
phase 2 fills the subject side for every subject with at least one event.
(Zero-event subjects still have no vector on either side; they stay in
`rankFallback`, which is correct and is what P2's `Source` value makes visible.)

### Proposal

Give `Rank` the same retrieval shape as `Recommend`: fetch the subject dense
vector, score the candidate set against `{ns}_objects_dense`, normalize both
score sets, blend with `alpha`, then apply the γ freshness decay that `Rank`
already does via `rerankScored` (`service.go:1226`).

Concretely this means extracting the blend body of `hybridRecommend`
(`service.go:436-479`) into a helper that both call sites share, rather than
duplicating it. `Rank` differs from `Recommend` in retrieval (a `HasID` filter
over a caller-supplied candidate set, not top-K search) and in paging (none) —
the blend itself is identical. See P4 for what "normalize" should mean in that
shared helper; the two changes touch the same code and should land together.

Behaviour when only one side is available must stay defined: no dense vector for
the subject → sparse-only, as today; no sparse vector but a dense one → dense-only
rather than the current whole-list fallback.

**Decided (D1).** The dense path in `Rank` respects `alpha` from the namespace
config — no per-request override, no new request field. Revisit only if
DarkVoid's tuning demands a different sparse/dense balance for the background
job than for interactive `/recommendations`.

---

## Problem 2 — `score: 0` is ambiguous and `Source` never disambiguates it

Three distinct outcomes are indistinguishable to the caller:

1. Subject has no vector at all → `rankFallback` returns every candidate with
   `Score: 0` in request order (`service.go:1259-1272`).
2. Candidate is not in the index (never upserted) → `Score: 0`, appended in
   request order (`service.go:1238-1244`).
3. Candidate is indexed but has zero overlap with the subject → `Score: 0`.

All three report `Source: "hybrid_rank"` and are documented as one case in
`pkg/codohuetypes/recommend.go:36-37`.

DarkVoid's blend rule is "CF > 0 → blend, else keep the local score"
(DarkVoid `design.md` §4.2 step 3). With this contract it cannot tell "CF is
switched off for this subject" from "CF scored this item as irrelevant", cannot
emit a coverage metric, and cannot decide to skip the call for subjects Codohue
knows nothing about.

This ambiguity is also what let the byoe silent-downgrade (P7) go unnoticed: a
namespace serving sparse-only looks identical, from the outside, to one where CF
genuinely has nothing to say.

### Proposal

Two changes to the wire contract:

- A distinct `Source` for the whole-response fallback — the existing enum already
  has precedent (`SourceFallbackPopular`, `SourceHybridCold`,
  `internal/recommend/types.go:46-50`). Suggested: `no_subject_vector`.
- A per-item boolean on `RankedItem` distinguishing "scored" from "returned
  unscored so the caller's candidate list comes back whole". Suggested:
  `"scored": true|false`.

Adding a field is backward compatible for existing decoders, but it is still a
contract change: it needs a `pkg/codohuetypes/testdata/*.golden.json`
regeneration (`go test ./pkg/codohuetypes/... -run Golden -update`) and a
matching SDK release.

**Decided (D2).** Add the `scored` flag and keep returning every candidate.
Omitting unscored items would be a breaking change — today every candidate comes
back, and `rankFallback` depends on that.

---

## Problem 3 — `Rank` applies neither the seen-items nor the `exclude_authored` filter

`Recommend` builds an exclusion set covering recently-seen items and, when
`exclude_authored` is on, the subject's own objects
(`service.go:913` `buildSeenItemsFilter`, `:945` `authoredObjectSet`,
`:974` `excludedObjectIDs`). `Rank` builds only a `HasID` filter over the
candidate ids (`service.go:1210-1214`).

The result is that the same namespace config produces two different notions of
"eligible object" depending on which endpoint is asked. A user's own posts and
posts they have already seen rank normally through `/rankings`, so DarkVoid would
have to reimplement both filters on its side.

Under the catalog direction the authored half has its data: DarkVoid's catalog
ingest already sends `author_subject_id`
(`internal/feature/post/service/post_service.go:387`), so once the namespace
runs `dense_source="catalog"` the `objects` table is populated and this filter
is immediately observable. (Under `byoe` it additionally needs P8's `PutObject`.)

### Proposal

Reuse `excludedObjectIDs` in `Rank` and merge the resulting `MustNot` into the
existing `HasID` filter, so one code path defines eligibility for both endpoints.

Excluded candidates should be returned with `scored: false` (P2) rather than
dropped, so the caller's candidate list still comes back whole and the caller can
apply its own fallback ordering.

**Decided (D3).** Unconditional — one notion of "eligible object" for both
endpoints, consistent with `Recommend`. Excluded candidates come back with
`scored: false`, so a caller that wants its own ordering still has the full
list. A skip-filters request flag is added only if DarkVoid asks.

---

## Problem 4 — per-request min-max normalization makes chunked calls incomparable

`normalizeScores` is min-max **over the candidates in one request**
(`service.go:440-441`). Once P1 lands, that property leaks to `Rank`: the top of
any request normalizes toward 1.0 regardless of its absolute relevance, so scores
from two calls cannot be compared. DarkVoid writes re-rank output into one ZSET
per user (`design.md` §4.2 step 4), which is exactly that comparison.

The 500-candidate cap (`internal/recommend/handler.go:22`) is what forces the
chunking, but it is the symptom, not the disease. Note also that the consumer's
page size is **not** a fixed number to match: `FEED_TIMELINE_MAX_ITEMS` is no
longer read from the environment — it moved to `settings.feed`, is edited at
runtime via `PATCH /admin/settings/feed`, and its documented range is 1–10000
(DarkVoid `pkg/config/load.go:207-212`, `internal/feature/settings/db/models.go:15`).
Any cap picked to "match the consumer" is matching a knob an operator can turn.

There is a second reason to touch normalization while P1 is open: dense cosine
scores are **already** on an absolute, cross-request-comparable scale — and
under the catalog direction the dense side is the primary signal, so this is the
common case, not the corner. Min-max over a batch throws that scale away. Only
the sparse side — unbounded dot products — genuinely lacks one.

### Proposal

Options, in preference order:

1. **Make normalization batch-independent.** Leave dense scores as-is (cosine is
   already bounded and comparable) and map sparse scores through a fixed
   saturating function such as `x/(x+k)`. Blending then stays meaningful across
   requests, chunking stops being a hazard, and the cap reverts to a pure
   Qdrant-cost question. Cost: `k` needs a defensible default, and
   `/recommendations` score *values* shift (ordering within a single response is
   what users see, and P1 already reserves the right to change values — but this
   is worth calling out in the release notes).

   **Decided: `k` is a single global constant** in code, tuned once against the
   demo dataset and DarkVoid's corpus. Not per-namespace (that is a schema
   migration this document forbids in Non-goals, promotable later without
   ceremony if a namespace actually needs a different value) and not adaptive
   from per-namespace score statistics (a `k` recomputed each cron tick
   reintroduces batch dependence through the back door — two calls straddling a
   tick would again be incomparable).
2. **Raise the cap** and document the per-request normalization as a hazard.
   Cheapest, and strictly a stopgap: it narrows the window in which chunking
   happens without making chunked results correct.
3. **Batch rank** — accept multiple subjects per request so a fleet-wide re-rank
   sweep is one call per batch rather than N calls per tick. Orthogonal to
   normalization; purely a per-call-overhead optimization.

Decision: (1) as part of the P1 blend-helper extraction, since both edit the
same code and (2) alone leaves the scores wrong. (3) only when DarkVoid's sweep
is measured and per-call overhead actually shows up.

**Decided (D4).** The cap stays at 500 for now. After (1), chunking costs round
trips but is no longer incorrect, so there is no correctness pressure to guess a
new number. Before changing it, benchmark `Rank` with `HasID` filters of
500/1000/2000 points against a real Qdrant — the id lookup is already one round
trip (`service.go:1196`), so the bound is about filter size, not fan-out — and
pick the cap from that measurement.

---

## Problem 5 — the core ingest path loses a failed catalog ingest permanently

DarkVoid indexes a post exactly once, at creation, fire-and-forget. Its own
operator CLI documents the consequence: "a post created while Codohue was
unreachable is simply absent from recommendations forever. There is no queue and
no repair pass; this command is the repair pass"
(`cmd/darkvoidctl/codohue.go:44-51`). That repair pass walks the whole corpus at
one HTTP call per post (`:21-24`).

Behavioural events do **not** have this problem, because they travel over a Redis
Stream: the producer XADDs, and the ingest worker consumes whenever it comes
back. The asymmetry is Codohue's, not DarkVoid's — events have a durable
transport and catalog content does not.

**Promoted by the catalog direction.** While catalog was one option among five,
a lossy HTTP-only ingest was a quality-of-implementation issue. As the core mode
it is the primary write path of the system, and "the recommended configuration
silently drops content during any outage" is not a posture the core mode can
keep. This is the largest work item in this document, but it is no longer
optional.

### Proposal

Three pieces, independently useful, in order:

1. **A client-facing catalog stream**, symmetric with `codohue:events`: the
   ingest worker consumes it and writes `catalog_items`, from where the existing
   `catalog:embed:{ns}` publish and embedder pipeline take over unchanged. This
   removes the entire "lost on outage" class rather than making it recoverable.
   The SDK side mirrors the event transport: `sdk/go/redistream` grows a catalog
   publisher, and DarkVoid's `IngestCatalogItem` call site switches transports
   without changing shape.
2. **Batch catalog ingest** on the HTTP path, so the repair walk is
   O(corpus/batch) requests instead of O(corpus). Still wanted with (1): the
   reindex/backfill path is HTTP.
3. **An incremental reconciliation read** — a data-plane way to ask which object
   ids the namespace already holds, or which changed since a timestamp — so a
   repair pass re-sends the gap instead of the whole corpus. The admin plane can
   already browse catalog items; the data plane cannot.

**Decided (D5).** (1) lives in the existing `ingest` worker. It already owns a
Redis Streams
consumer, and the work on the far side of the stream — persist `catalog_items`,
publish `catalog:embed:{ns}` — is exactly what `internal/catalog` does today for
the HTTP path. Putting it in `cmd/embedder` means either duplicating that logic
or routing it through a `cmd/`-level adapter, because the import rule forbids
`internal/embedder` from importing `internal/catalog`. The counter-argument
(`cmd/embedder` owns the namespace poller) is weaker: the poller decides which
namespaces to *drain*, and a stream consumer keyed by namespace does not need it.

---

## Problem 6 — the core mode is the awkward one to provision

`/api/admin/v1/*` accepts only the session cookie. Every automated provisioner
therefore has to exchange the admin key for a cookie first, then carry it
manually. DarkVoid's `pkg/codohue/provision.go` is 255 lines doing exactly that,
with a comment noting "the official runtime SDK intentionally does not wrap these
operator-facing routes" (`:83-87`).

On top of that, `dense_source="catalog"` is rejected on the namespace upsert
route and must be set through the separate catalog endpoint
(`internal/admin/types.go:195` `ErrCatalogSourceViaUpsert`,
`internal/admin/handler.go:230`). DarkVoid works around it by blanking the field
and issuing a second request, with a four-line comment explaining why
(`provision.go:64-67`, `:117-119`).

The rejection has a real reason — the catalog endpoint validates the strategy's
`dim` against `embedding_dim`, and the upsert route cannot. But under the
catalog direction the effect is upside down: **the mode we steer every consumer
onto is the only one that cannot be set in one request.** The optional modes are
one `PUT`; the core mode is a two-step dance with a workaround comment in every
provisioner that touches it.

### Proposal

- Accept `dense_source="catalog"` on `PUT /api/admin/v1/namespaces/{ns}` when
  the catalog strategy fields accompany it in the same body, applying the same
  dim validation the catalog endpoint does. Reject as today when they are absent
  — the error message becomes "supply strategy_id/strategy_version" rather than
  "use the other endpoint". This is the piece the direction promotes.
- Accept `Authorization: Bearer <CODOHUE_ADMIN_API_KEY>` on `/api/admin/v1/*`
  alongside the session cookie, guarded by the same shape of per-IP rate limiter
  the login endpoint uses (failed attempts only — a correct key is never
  throttled). Sessions stay as they are for the browser UI.
- Ship `sdk/go/admin` with, at minimum, a one-call
  `ProvisionCatalogNamespace(ns, dim, ...)` so the recommended configuration is
  the SDK's paved road rather than a hand-rolled client in each consumer.

**Decided (D6).** Bearer with the existing admin key, plus the rate limiter
above — not a separate rotatable automation key. The trade-off was weighed: the
bearer path grants no privilege the key does not already have through login, but
it does change the shape of the exposure — admin sessions are HMAC JWTs carrying
a `jti` and are revocable server-side, while the static key is revocable only by
rotating it, which also invalidates every data-plane caller since the key is
accepted for every namespace. With a single internal consumer that fleet-wide
rotation cost is acceptable; a second credential to provision and manage is not.
Revisit if a second consumer or an external operator appears.

---

## Problem 7 — optional modes downgrade silently when `{ns}_subjects_dense` is empty

`phase2Runs` returns true for `item2vec`, `svd` and `catalog` only
(`internal/compute/job.go:512`). Under `byoe` the dense phase is skipped
entirely, so nothing writes `{ns}_subjects_dense` unless the client calls
`PUT /v1/namespaces/{ns}/subjects/{id}/embedding` — and DarkVoid's client never
does (`UpsertSubjectEmbedding`, `pkg/codohue/client.go:204`, has no production
caller). `Recommend` then fetches the subject dense vector, gets nil, and falls
through to pure sparse CF with a `Debug`-level log
(`internal/recommend/service.go:359-365`): the config says
`alpha=0.5, dense_source=byoe`, the service serves sparse-only, and nothing
tells the operator.

In revision 2 this was the top blocker, because DarkVoid was going to *stay* on
`byoe`. Under the catalog direction the primary fix is the migration itself —
`catalog` cannot get into this state — and this problem demotes to hardening
for the modes that remain optional. Demoted is not dismissed: `byoe` stays
supported, and a supported mode must fail loudly, not quietly serve something
else.

### Proposal

Two tiers:

1. **Now (cheap): make the downgrade visible — decided as warning log + admin
   overview alert.** A warning-level log in the recommend service and an alert
   on the admin overview when a namespace has dense scoring configured
   (`alpha < 1`, `dense_source != "disabled"`) and an empty
   `{ns}_subjects_dense`. P2's `no_subject_vector` source gives per-request
   visibility on the wire; this gives the operator the fleet view. The admin
   plane already assembles per-namespace Qdrant counts and an alerts list for
   the overview, so the signal is one comparison away. (A dedicated Prometheus
   counter was considered and skipped for now — the existing
   `RecommendRequests` source labels already expose the fallback rate.)
2. **Later (optional): run phase 2 under `byoe`**, mean-pooling subject vectors
   from the client-pushed object vectors — the same read-back shape the
   `catalog` branch already has (`FetchItemDenseVectors`,
   `internal/compute/dense.go:275`). This would need a non-clobber rule so a
   client-pushed subject vector is not overwritten by the next tick (a `source`
   payload field on the point, mirroring how `dense_source="catalog"` resolves
   object-vector ownership at `internal/recommend/service.go:184-188`). Do this
   only if a consumer actually commits to `byoe` for the long term; do not
   build it speculatively.

---

## Problem 8 — SDK gaps: object metadata write and `dense_source` constants

`PUT /v1/namespaces/{ns}/objects/{id}` exists and its wire type is defined
(`pkg/codohuetypes/object.go`), but the Go SDK has no wrapper for it —
`sdk/go/embedding.go:41` builds that path only for `DeleteObject`. Under the
optional non-catalog modes this is the only way to attach `author_subject_id`,
so `exclude_authored` is unreachable there (the failure mode migration 021
removed on the server; the client-side reach never landed). Under the catalog
direction this loses its urgency — authorship flows through catalog ingest —
but the endpoint is documented as working under every `dense_source`, and the
SDK should be able to say what the API can.

Separately, and independent of mode: the `dense_source` values are bare string
literals everywhere, including this repo's own validation and special-case
sites (`internal/recommend/service.go:188`, `internal/compute/job.go:514`).
DarkVoid redeclares its own set (`pkg/codohue/provision.go:18-33`) because
there is nothing to import.

Note: `Action` constants already exist (`pkg/codohuetypes/event.go:9-14`) and
DarkVoid already depends on that module (`go.mod:13`). Its redeclaration at
`pkg/codohue/client.go:22-28` is a consumer-side cleanup, not a Codohue gap.

### Proposal

- Export the `dense_source` values as constants from `pkg/codohuetypes` and use
  them in `internal/` — additive, rides the P2 SDK release.
- Add `Namespace.PutObject(ctx, objectID string, req codohuetypes.ObjectUpsertRequest) (*codohuetypes.ObjectResponse, error)`
  to `sdk/go` — small, no contract change, no urgency.

---

## Problem 9 — no data-plane way to ask "do you have data for this namespace?"

DarkVoid's `/health` reports `codohue: off|active|degraded` from a `Ping` probe
plus its circuit breaker. That answers "is Codohue reachable", not "does Codohue
have anything to say about this namespace". A background re-rank job will happily
run against an empty index and write zero-signal scores.

Codohue knows the answer — object counts, subject vector presence, last
successful batch run, and (relevant under catalog) the embed backlog — but only
through the admin plane, which a data-plane client has no key for.

### Proposal

A small authenticated data-plane readiness read: indexed object count, whether
the requesting subject has a vector (sparse and dense, separately — P7 is
exactly the case where one is present and the other is not), the last successful
recompute time, and the catalog backlog size. This is the lowest-priority item
here and is listed for completeness; it is worth doing only if DarkVoid actually
wants to gate its job on it.

---

## Wire contract impact

| Change | Contract impact |
|---|---|
| DarkVoid migration to `catalog` | None — consumer config flip + reindex |
| P1 dense blend in `Rank` | None on its own (scores change value, not shape) |
| P2 `Source` value + `scored` flag | `RankedItem` + `RankResponse` — golden regen + SDK release |
| P3 filters in `Rank` | None if excluded items return `scored: false` |
| P4 batch-independent normalization | None structurally; `/recommendations` score *values* shift |
| P5 catalog stream / batch ingest | New transport + new endpoint; new types |
| P6 `catalog` on upsert + bearer auth | Admin plane only, not `pkg/codohuetypes` |
| P7 downgrade warning | None (log + admin overview alert) |
| P8 `dense_source` constants / `PutObject` | Additive to `pkg/codohuetypes` / none |
| P9 readiness read | New endpoint; new type |

P2 is the only item that touches an existing client-facing type, and P1/P4
change score values under the same `Source`. Shipping P1–P4 plus the P8
constants together means DarkVoid takes one SDK bump and one behavioural change.

## Proposed order

1. **DarkVoid flips to `catalog`** — `CODOHUE_DENSE_SOURCE=catalog`, provision,
   `darkvoidctl codohue reindex`. Consumer-side only; everything is already
   wired. This alone fixes the silent sparse-only downgrade in production
   `/recommendations`, before any Codohue code changes.
2. **P1 + P2 + P3 + P4 + P8-constants together** — one coherent change to
   `Rank` and the blend helper, one contract revision, one SDK release. This is
   what unblocks DarkVoid's spec 006.
3. **P5** — catalog-path durability. Promoted by the direction; the largest
   piece, start with the stream (piece 1).
4. **P6** — provisioning the core mode in one request, then bearer auth per D6.
5. **P7 tier 1** (downgrade warning), **P8 `PutObject`**, then **P9** —
   hardening and ergonomics, no correctness impact. P7 tier 2 only if a
   consumer commits to `byoe` long-term.

## Non-goals

- Changing `/recommendations` **output shape**, `Trending`, or the cron phases.
  P1/P4 refactor the blend into a shared helper and change score *values* on
  `/recommendations`; they must not change its contract or ordering semantics.
- Removing or deprecating `item2vec`, `svd`, or `byoe`. Catalog is the core
  mode, not the only mode; the options stay supported and P7 makes their
  failure mode loud instead of silent.
- Emitting `VIEW` events. That is DarkVoid's side (their `design.md` §10) and no
  amount of Codohue work substitutes for it — the catalog migration plus P1
  make the dense signal usable in the meantime, they do not repair the sparse
  CF graph.
- A new embedding strategy. Catalog-as-core makes embedding quality Codohue's
  responsibility, and hashing+ngrams is a baseline on par with the TF-IDF it
  replaces — the migration is quality-neutral by design. A model-backed
  strategy is the obvious next investment, but it drops into the existing
  registry seam (`internal/embedder/strategy.go`) and needs no contract work,
  so it is deliberately out of scope here.
- Any migration. Nothing here requires a schema change.

## Rejected alternatives

**Keep `byoe` as DarkVoid's default and make it self-sufficient instead**
(revision 2's plan: run phase 2 under `byoe` with a non-clobber rule). It fixes
the subject-vector gap but keeps every other cost of the mode: authorship needs
a separate client call per object, embedding quality is frozen client-side where
Codohue's re-embed machinery cannot reach it, and every future consumer
reimplements a vectorizer. The catalog migration gets the same subject vectors
from the phase-2 branch that already exists, and the non-clobber machinery
becomes unnecessary. Phase-2-under-byoe survives only as P7 tier 2, built on
demand.

**Make `Rank` an alias for `Recommend` with a candidate filter.** They differ in
more than the filter: `Rank` returns the caller's full candidate list including
unscorable items and does no paging, while `Recommend` pages a ranked top-K and
falls back to trending/popular on cold start. Folding them together would force
one of those behaviours onto the other.

**Have DarkVoid call `/recommendations` and intersect client-side.** This is what
avoiding P1 would mean in practice. It does not work: `Recommend` returns a
top-K, so a timeline item outside that K comes back with no score at all, and
the intersection shrinks exactly where the timeline is long — the case the
re-rank job exists to serve.

**Match the candidate cap to the consumer's page size (skip P4's option 1).**
The number it would match is a runtime-tunable setting with a documented range up
to 10000, so there is no page size to match — and a cap large enough to avoid
chunking does not make chunked scores comparable, it only makes chunking rarer.
