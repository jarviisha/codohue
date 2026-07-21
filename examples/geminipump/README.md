# geminipump — Codohue data-pump bot with Gemini-generated content

A runnable bot that **continuously pumps data** into a Codohue deployment, where
the catalog content is **generated on the fly by the Google Gemini API** instead
of a fixed dataset. It reads recommendations back out so you can watch the
recommendation loop close end-to-end.

What it does:

1. **Bootstraps** a namespace through the admin plane (`cmd/admin`): logs in,
   upserts the namespace config (`dense_strategy=byoe`, `alpha=0.7`), and enables
   catalog auto-embedding (`internal-hashing-ngrams@v1`, dim 256).
2. **Generates catalog content** by asking Gemini for a batch of items (title +
   summary + category) via structured JSON output, then ingests each through the
   public catalog path (`cmd/api`). The embedder worker upserts their dense
   vectors. A background loop keeps topping the catalog up until `-max-items`.
3. **Pumps events** through the public HTTP ingest path using the Go SDK. Each
   simulated user has a category affinity, so the VIEW/LIKE/COMMENT/SHARE/SKIP
   stream carries a learnable preference signal instead of uniform noise.
4. **Reads back** recommendations and trending on an interval and logs them.

It only generates data — the `cron` binary (sparse + dense vectors, trending)
and the `embedder` binary (catalog dense vectors) do the actual computation, so
recommendations turn non-empty after the first cron tick.

## Prerequisites

The full stack must be running (api + cron + admin + embedder + infra):

```bash
make up-d
```

And a Gemini API key (from Google AI Studio):

```bash
export GEMINI_API_KEY=your-key-here
```

## Run

```bash
# uses GEMINI_API_KEY from the environment
go run ./examples/geminipump

# faster pump, bigger catalog, custom namespace:
go run ./examples/geminipump -rate 20 -users 200 -ns playground -max-items 500

# pass the key explicitly and pick a model:
go run ./examples/geminipump -gemini-key "$GEMINI_API_KEY" -gemini-model gemini-2.5-flash
```

> Run from inside the module (`cd examples/geminipump`) or with `GOWORK=off` — it
> is a standalone module that resolves the SDK via `replace` directives, so it is
> intentionally outside the repo `go.work`.

## Flags

| Flag             | Default                                              | Description                                          |
| ---------------- | ---------------------------------------------------- | ---------------------------------------------------- |
| `-api`           | `http://localhost:2001`                              | `cmd/api` base URL (`CODOHUE_API_URL`)               |
| `-admin`         | `http://localhost:2002`                              | `cmd/admin` base URL (`CODOHUE_ADMIN_URL`)           |
| `-ns`            | `geminipump`                                         | Namespace to pump into                               |
| `-admin-key`     | `dev-secret-key`                                     | Global admin key (`CODOHUE_ADMIN_API_KEY`)           |
| `-gemini-key`    | `$GEMINI_API_KEY`                                    | Google Gemini API key (**required**)                 |
| `-gemini-model`  | `gemini-2.5-flash`                                   | Gemini model id (`GEMINI_MODEL`)                     |
| `-gemini-base`   | `https://generativelanguage.googleapis.com/v1beta`   | Gemini API base URL (`GEMINI_BASE_URL`)              |
| `-rate`          | `5`                                                  | Target events per second                             |
| `-users`         | `50`                                                 | Number of simulated users                            |
| `-dim`           | `256`                                                | Embedding dimension (64/128/256/512)                 |
| `-catalog-batch` | `12`                                                 | Catalog items to request from Gemini per generation  |
| `-gen-every`     | `2m`                                                 | How often to generate a fresh catalog batch          |
| `-max-items`     | `300`                                                | Stop generating once the catalog reaches this size   |
| `-recs-every`    | `15s`                                                | How often to read recommendations/trending back      |
| `-seed`          | `42`                                                 | PRNG seed for reproducible event patterns            |
| `-bootstrap`     | `true`                                               | Set `-bootstrap=false` to pump into an existing ns   |

The global admin key is always accepted as the data-plane Bearer token, so the
bot does not need a per-namespace key.

Stop with `Ctrl-C`; it prints `sent`/`failed`/`catalog` totals on exit.

## Run on a VPS

The bot is **not** part of the deployed compose stack and is **not** published to
GHCR or Docker Hub — it is an operator tool. Ship it as a static binary:

```bash
# 1. cross-compile (defaults to linux/amd64; PUMP_GOARCH=arm64 for ARM hosts)
make build-geminipump

# 2. copy the binary and the systemd unit
scp tmp/geminipump                        VPS:/usr/local/bin/geminipump
scp examples/geminipump/geminipump.service VPS:/etc/systemd/system/

# 3. write the secrets file (root-only; never pass keys as CLI args)
ssh VPS 'install -m 600 /dev/stdin /etc/codohue-geminipump.env' <<'EOF'
GEMINI_API_KEY=your-gemini-key
CODOHUE_ADMIN_API_KEY=your-production-admin-key
EOF

# 4. start it
ssh VPS 'systemctl daemon-reload && systemctl enable --now geminipump'
ssh VPS 'journalctl -u geminipump -f'
```

`docker-compose.prod.yml` publishes api (2001) and admin (2002) to the host, so
the unit points the bot at `127.0.0.1` — no compose network membership needed and
no admin key crossing the public internet. Tune `-ns` / `-rate` / `-gen-every` /
`-max-items` in the unit's `ExecStart`, then `systemctl restart geminipump`.

To stop and remove it:

```bash
ssh VPS 'systemctl disable --now geminipump && rm /etc/systemd/system/geminipump.service /usr/local/bin/geminipump /etc/codohue-geminipump.env'
```

The synthetic data it wrote stays behind — drop it with
`DELETE /api/admin/v1/namespaces/{ns}` from the admin console.

## Notes

- Gemini content generation uses **structured output** (a response JSON schema),
  so the model returns a parseable array rather than free-form prose.
- Object IDs are re-derived locally (`g<batch>_<index>_<slug>`) so they stay
  unique across batches regardless of what the model suggests.
- If a Gemini call fails mid-run, the generator logs it and keeps the existing
  catalog — the event pump never stops. The **first** batch must succeed for the
  bot to have anything to pump.
