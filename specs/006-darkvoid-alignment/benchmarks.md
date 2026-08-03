# D4 measurement — Rank `HasID` filter cost vs candidate count

Decision under test (design.md D4): the 500-candidate cap stays until a
measured Qdrant filter cost justifies changing it.

## Method

Standalone Go program against a local Qdrant (v1.17.1, Docker, same image the
dev stack runs), 2026-08-03:

- Collection: 20,000 points with sparse vectors (`sparse_interactions`),
  8–31 non-zero entries each over a 50k-dim index space — the shape
  `{ns}_objects` has after a cron recompute.
- Query: one sparse query vector (16 NNZ) through the Query API with a
  `HasID` filter over N randomly chosen point ids, `limit=N`,
  payload included — exactly the request `Service.Rank` issues per side.
- 3 warmup + 30 measured iterations per N, localhost gRPC.

## Results

| Candidates (N) | mean | worst |
|---|---|---|
| 500 | 1.00 ms | 1.76 ms |
| 1000 | 2.08 ms | 2.72 ms |
| 2000 | 3.02 ms | 3.98 ms |

## Reading

- Cost is linear in N and small in absolute terms: going 500 → 2000 adds
  ~2 ms per search side. The `HasID` filter itself is not a bottleneck at any
  size a rankings call would plausibly carry.
- Since normalization became batch-independent (FR-003), chunking is
  correct — a 1000-item timeline as 2×500 costs one extra round trip
  (~1–2 ms server-side plus network), not wrong results.

## Decision

**Cap stays at 500.** Nothing here forces a raise, and the cap's real bounds
are now request-body size and the `GetOrCreateObjectIDs` batch (one SQL
round-trip whose statement grows with N), not Qdrant. If a consumer measures
meaningful per-call overhead from chunking, raising to 1000 is cheap per this
data — re-run the method above on a production-sized collection first, then
change `maxCandidates` in internal/recommend/handler.go and this file
together.
