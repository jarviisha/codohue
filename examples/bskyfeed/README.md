# bskyfeed — live Bluesky firehose feeder

Pumps live public Bluesky activity into a Codohue namespace, so a development
stack runs on real, always-fresh behavioral data instead of synthetic traffic.

The AT Protocol firehose is public: no account, no API key, no rate limit, and
no sampling. That last point is what makes it useful here — a smaller platform
you can read *completely* beats a larger one you can only sample, because
collaborative filtering reads co-occurrence and sampling destroys it
quadratically.

## What it ingests

| AT Protocol record | Becomes |
| --- | --- |
| `app.bsky.feed.like` | `LIKE` event — actor DID → liked post URI |
| `app.bsky.feed.repost` | `SHARE` event — actor DID → reposted post URI |
| `app.bsky.feed.post` | catalog item (`object_id` = the post's AT-URI, `content` = its text) |
| `app.bsky.feed.post` with `reply` | additionally a `COMMENT` event — actor DID → parent post URI |

Subjects are DIDs (`did:plc:…`) and objects are post AT-URIs
(`at://did:plc:…/app.bsky.feed.post/…`), so a post ingested as catalog content
and a like referencing it agree on `object_id`.

Events go to `codohue:events` and catalog content to `codohue:catalog` through
the `redistream` SDK, so the feeder does not need `cmd/api` to be reachable and
content published during an ingest outage is consumed on recovery.

## Running it

In the Compose stack, enable the profile in `.env`:

```dotenv
COMPOSE_PROFILES=bskyfeed
```

Provision the `bluesky` catalog namespace first using the [trusted operator provisioning path](../../deploy/operator-auth.md), then run `make up-d`. The feeder is a trusted Redis producer, not an isolated HTTP consumer. It receives no PostgreSQL credentials. Outside Compose, with the stack already running:

```bash
make run-bskyfeed
```

## Configuration

Environment only — there are no flags.

| Variable | Default | Meaning |
| --- | --- | --- |
| `CODOHUE_BSKY_NAMESPACE` | `bluesky` | Namespace to feed |
| `CODOHUE_BSKY_SAMPLE_PERCENT` | `100` | Share of actors to keep, 1–100 |
| `CODOHUE_BSKY_EMBEDDING_DIM` | `256` | Dense dimension; one of 64/128/256/512 |
| `CODOHUE_BSKY_BOOTSTRAP` | `false` | Provision the namespace on startup |
| `CODOHUE_BSKY_JETSTREAM_URL` | public Jetstream instance | Override the endpoint |
| `REDIS_URL` | `redis://localhost:6379` | Trusted stream producer connection |
| `CODOHUE_ADMIN_URL` | `http://localhost:2002` | Optional bootstrap endpoint |
| `CODOHUE_ADMIN_API_KEY` | empty | Explicit namespace-scoped service token for optional bootstrap; Compose reads `CODOHUE_BSKY_PROVISION_TOKEN` |

`CODOHUE_BSKY_SAMPLE_PERCENT` samples **by actor**, not by record: it keeps a
deterministic subset of DIDs and every record those DIDs produce. Sampling
records instead would thin every actor's history and collapse the
co-occurrence signal, which is exactly what CF depends on.

With `CODOHUE_BSKY_BOOTSTRAP=true` and a service token granting `admin:read,admin:write` for this namespace, the feeder upserts the namespace with
`dense_source=catalog` and the `internal-hashing-ngrams@v1` strategy. The
upsert has PATCH semantics, so restarts are safe.

## Measured volume

A 5-minute sample of the firehose (2026-09-16):

```
367 records/sec total — 268 likes, 57 posts, 42 reposts
91,901 likes from 27,592 distinct actors
55.8% of actors produced 2+ likes within the window
4,943 actors reached 5+ likes after 5 minutes
```

At 100% sampling that is roughly 1.1M interaction events per hour and about
60k subjects per hour crossing the `coldStartThreshold = 5` mark that moves a
subject off the `hybrid_cold` path.

## Known limitations

- **Dense coverage is partial.** Object vectors only exist for posts whose
  creation the feeder witnessed. A like on a post from last week references an
  `object_id` with no catalog content, so it contributes to the sparse CF
  signal but not to the dense one. Coverage improves the longer it runs.
- **Posts under 20 runes are skipped** as too short to embed usefully.
- **No backfill.** The feeder is live-only; Jetstream's replay window covers
  reconnects, not history.
- `occurred_at` comes from the firehose's `time_us`, not the record's
  client-supplied `createdAt`, which is routinely skewed or backdated.
