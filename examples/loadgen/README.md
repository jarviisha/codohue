# loadgen — Codohue example client

A runnable example that **continuously pumps realistic behavioral data** into a
Codohue deployment and reads recommendations back out, so you can watch the
recommendation loop close end-to-end.

What it does:

1. **Bootstraps** a namespace through the admin plane (`cmd/admin`): logs in,
   upserts the namespace config (`dense_strategy=byoe`, `alpha=0.7`), enables
   catalog auto-embedding (`internal-hashing-ngrams@v1`, dim 256), and seeds a
   small content catalog.
2. **Pumps events** through the public HTTP ingest path (`cmd/api`) using the
   Go SDK. Each simulated user has a category affinity, so the
   VIEW/LIKE/COMMENT/SHARE/SKIP stream carries a learnable preference signal
   instead of uniform noise.
3. **Reads back** recommendations and trending on an interval and logs them.

It only generates data — the `cron` binary (sparse + dense vectors, trending)
and the `embedder` binary (catalog dense vectors) do the actual computation, so
recommendations turn non-empty after the first cron tick.

## Prerequisites

The full stack must be running (api + cron + admin + embedder + infra):

```bash
make up-d
```

## Run

```bash
make run-loadgen
# or pass flags:
make run-loadgen ARGS="-rate 20 -users 200 -ns playground"
# or directly:
go run ./examples/loadgen -rate 10
```

## Flags

| Flag           | Default                 | Description                                        |
| -------------- | ----------------------- | -------------------------------------------------- |
| `-api`         | `http://localhost:2001` | `cmd/api` base URL (`CODOHUE_API_URL`)             |
| `-admin`       | `http://localhost:2002` | `cmd/admin` base URL (`CODOHUE_ADMIN_URL`)         |
| `-ns`          | `loadgen`               | Namespace to pump into                             |
| `-admin-key`   | `dev-secret-key`        | Global admin key (`CODOHUE_ADMIN_API_KEY`)         |
| `-rate`        | `5`                     | Target events per second                           |
| `-users`       | `50`                    | Number of simulated users                          |
| `-dim`         | `256`                   | Embedding dimension (64/128/256/512)               |
| `-recs-every`  | `15s`                   | How often to read recommendations/trending back    |
| `-seed`        | `42`                    | PRNG seed for reproducible runs                    |
| `-bootstrap`   | `true`                  | Set `-bootstrap=false` to pump into an existing ns |

The global admin key is always accepted as the data-plane Bearer token, so the
example does not need a per-namespace key.

Stop with `Ctrl-C`; it prints `sent`/`failed` totals on exit.
