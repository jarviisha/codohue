# Quickstart: Catalog Auto-Embedding Service

**Feature**: 004-catalog-embedder
**Date**: 2026-05-09
**Audience**: developer ramping on the feature, plus operator turning it on for a tenant.

This document is the "I just landed the implementation, what do I run?" guide. It is the developer/operator counterpart to the operator-facing spec; it stays at the *what to run* / *what to expect* level rather than design rationale (that lives in [research.md](./research.md)).

---

## Prerequisites

- The repo is checked out and you can already run `make up-d` to bring up Postgres / Redis / Qdrant locally.
- Go 1.26.1 (matching `go.mod`).
- `psql`, `redis-cli` and `curl` on `PATH` for the verification steps below.
- Migrations 010 and 011 from this feature applied (`make migrate-up`).

---

## 1. Build and run the embedder locally

```bash
make build-embedder   # produces ./tmp/embedder
make up-infra         # postgres + redis + qdrant
make migrate-up       # apply 010 + 011

# foreground:
./tmp/embedder

# or via the dev target (live reload via air, mirroring make dev for cmd/api):
make dev-embedder
```

Run alongside `cmd/api` (`make dev`) — the catalog ingest endpoint lives in `cmd/api`; the embedder consumes the resulting Redis stream.

Default config is read from environment (`.env`):

| Variable | Default | Notes |
|---|---|---|
| `EMBEDDER_PORT` | unused | `cmd/embedder` does not expose HTTP except `/healthz` and `/metrics`; see `EMBEDDER_HEALTH_PORT`. |
| `EMBEDDER_HEALTH_PORT` | `2003` | Liveness + Prometheus metrics endpoint. |
| `CATALOG_MAX_CONTENT_BYTES` | `32768` | Global default; overridable per-namespace. |
| `EMBED_MAX_ATTEMPTS` | `5` | Global default for transient retries. |
| `EMBEDDER_REPLICA_NAME` | `${HOSTNAME}` | Used as the consumer name in the Redis Streams group. |
| `EMBEDDER_NAMESPACE_POLL_INTERVAL` | `30s` | How often `cmd/embedder` polls `namespace_configs` for newly-enabled namespaces. |

---

## 2. Enable catalog auto-embedding for a namespace

Assume a namespace `socialdemo` already exists with `embedding_dim=128`.

```bash
# Authenticate to the admin server (existing flow):
curl -c /tmp/admin.cookie \
  -H 'Content-Type: application/json' \
  -d '{"api_key":"dev-secret-key"}' \
  http://localhost:2002/api/v1/auth/sessions

# Enable catalog mode:
curl -b /tmp/admin.cookie -X PUT \
  -H 'Content-Type: application/json' \
  -d '{
        "enabled": true,
        "strategy_id": "internal-hashing-ngrams",
        "strategy_version": "v1",
        "params": {"dim": 128}
      }' \
  http://localhost:2002/api/admin/v1/namespaces/socialdemo/catalog
```

Expected: `200 OK` with the persisted config echo. If you get `400 Bad Request` with `strategy_dim` ≠ `namespace_embedding_dim`, the namespace's `embedding_dim` is not 128 — either change the namespace's `embedding_dim` first, or re-register the strategy at the namespace's existing dim.

Verify enablement:

```bash
curl -b /tmp/admin.cookie \
  http://localhost:2002/api/admin/v1/namespaces/socialdemo/catalog | jq
```

You should see `enabled: true`, the strategy descriptor, and zeros across the backlog object.

---

## 3. Ingest a catalog item

```bash
curl -X POST \
  -H 'Authorization: Bearer dev-secret-key' \
  -H 'Content-Type: application/json' \
  -d '{
        "object_id": "post_42abc",
        "content": "Hôm nay trời đẹp quá, ai cũng muốn ra biển! #weekend",
        "metadata": {"author_id": "user_77", "language_hint": "vi"}
      }' \
  http://localhost:2001/v1/namespaces/socialdemo/catalog
```

Expected: `202 Accepted` (empty body). Within a few seconds the embedder picks it up.

---

## 4. Verify embedding produced

Postgres side:

```sql
SELECT id, object_id, state, strategy_id, strategy_version, embedded_at
FROM catalog_items
WHERE namespace = 'socialdemo'
ORDER BY id DESC
LIMIT 5;
```

Expected row: `state='embedded'`, `strategy_id='internal-hashing-ngrams'`, `strategy_version='v1'`, `embedded_at` recent.

Qdrant side (via the existing admin Qdrant inspect endpoint):

```bash
curl -b /tmp/admin.cookie \
  http://localhost:2002/api/admin/v1/namespaces/socialdemo/qdrant | jq
```

Expected: the `socialdemo_objects_dense` count incremented by 1.

Redis side:

```bash
redis-cli XLEN catalog:embed:socialdemo          # should be 0 (consumed + ACK'd)
redis-cli XPENDING catalog:embed:socialdemo embedder   # should report 0 pending
```

---

## 5. Re-embed a namespace

To pretend the strategy version was bumped (e.g. when V2 lands):

```bash
# Update strategy version to v2 (assumes v2 is registered):
curl -b /tmp/admin.cookie -X PUT \
  -H 'Content-Type: application/json' \
  -d '{
        "enabled": true,
        "strategy_id": "internal-hashing-ngrams",
        "strategy_version": "v2",
        "params": {"dim": 128}
      }' \
  http://localhost:2002/api/admin/v1/namespaces/socialdemo/catalog

# Trigger re-embed:
curl -b /tmp/admin.cookie -X POST \
  http://localhost:2002/api/admin/v1/namespaces/socialdemo/catalog/re-embed
```

Expected: `202 Accepted` with `Location` header pointing to a new batch-run row. Watch progress via `GET .../catalog` (`backlog.pending` ticks down) and the existing batch-run inspector. New ingests during this window are embedded under `v2` immediately (Q2 acceptance scenario US3#2).

---

## 6. Inspect dead-letter and re-drive

If an item has gone to `dead_letter` (e.g. it produced a zero-norm vector — content was 100% punctuation):

```bash
curl -b /tmp/admin.cookie \
  'http://localhost:2002/api/admin/v1/namespaces/socialdemo/catalog/items?state=dead_letter' | jq
```

To bulk re-drive every dead-letter item:

```bash
curl -b /tmp/admin.cookie -X POST \
  http://localhost:2002/api/admin/v1/namespaces/socialdemo/catalog/items/redrive-deadletter
```

Expected: `202 Accepted`; items move back to `pending` and are re-attempted.

---

## 7. Disable the catalog feature for a namespace

```bash
curl -b /tmp/admin.cookie -X PUT \
  -H 'Content-Type: application/json' \
  -d '{"enabled": false}' \
  http://localhost:2002/api/admin/v1/namespaces/socialdemo/catalog
```

Effects:

- New `POST /v1/namespaces/socialdemo/catalog` returns `404` (FR-008).
- Existing dense vectors in `socialdemo_objects_dense` are NOT removed — recommendations continue to serve them. Operator must explicitly `DELETE .../catalog/items/{id}` or use the existing object-deletion flow to clear them.
- `PUT /v1/namespaces/socialdemo/objects/{id}/embedding` (BYOE write) returns to its pre-feature behaviour (R8) for new writes.

---

## 8. Run the test suite for this feature

```bash
make test-pkg PKG=./internal/catalog/...
make test-pkg PKG=./internal/embedder/...
make test-pkg PKG=./internal/core/embedstrategy/...
make test-e2e-heavy   # exercises the catalog ingest → embed → recommend cycle
```

The e2e suite covers US1 acceptance scenario #2 (item discoverable through dense recommendations after embedding) end-to-end.

---

## 9. Common operator pitfalls

| Symptom | Cause | Fix |
|---|---|---|
| `400 Bad Request: strategy dimension mismatch` on PUT catalog config | The selected strategy's dim doesn't match `namespace.embedding_dim`. | Pick a different `(strategy_id, strategy_version)` whose Dim matches, or change the namespace's `embedding_dim` first (which itself triggers existing dense re-create). |
| Items stuck at `pending` with `XLEN > 0` | No embedder replica is running for that namespace. | Bring `cmd/embedder` up, or scale the deployment. |
| `409 Conflict` on BYOE write `PUT .../objects/{id}/embedding` | Namespace has `catalog_enabled=true`. BYOE is rejected when catalog is the source of truth. | Either disable catalog or delete the catalog item and re-write via BYOE. |
| Items silently embedded under old strategy version after a config change | The re-embed was not triggered. The strategy version change alone does not re-embed (Assumption "Re-embed cadence"). | Call `POST .../catalog/re-embed` explicitly. |
| Oversized content rejected with `413` | `content > catalog_max_content_bytes`. | Either truncate client-side or increase `catalog_max_content_bytes` in the namespace config (no truncation in V1). |
| Vietnamese recommendations feel weak | Expected on V1 — language-agnostic n-gram baseline (Q3 / Assumption "Language scope"). A Vietnamese-aware tokenizer is a follow-up strategy version, not a V1 deliverable. | Plan a V2 strategy registration once a Vietnamese tokenizer is available. |

---

## 10. Cleanup

```bash
# Drop catalog rows for a namespace (re-creates BYOE source-of-truth posture):
psql $DATABASE_URL -c "DELETE FROM catalog_items WHERE namespace = 'socialdemo';"

# Drop the Redis stream:
redis-cli DEL catalog:embed:socialdemo

# Reset namespace config:
curl -b /tmp/admin.cookie -X PUT -H 'Content-Type: application/json' \
  -d '{"enabled": false}' \
  http://localhost:2002/api/admin/v1/namespaces/socialdemo/catalog
```

This leaves Qdrant `*_objects_dense` points in place; clear them via the existing object-deletion flow if a clean slate is needed.
