# Quickstart: Dense Source Unification

Verify the unified `dense_source` model end-to-end after implementation.

## Prerequisites

- Infra up: `make up-infra` (postgres + redis + qdrant)
- Migration applied: `make migrate-up` (includes `016_dense_source`)
- Binaries built: `make build`

## 1. Migration is reversible and total

```bash
make migrate-up      # applies 016: rename + backfill + drop catalog_enabled + CHECK
make migrate-version # confirm at 016
make migrate-down    # rolls back 016 cleanly
make migrate-up      # re-apply
```

Expected: every existing namespace has a `dense_source` in
`{disabled,item2vec,svd,byoe,catalog}`; rows that had `catalog_enabled=true` are now `catalog`;
no `catalog_enabled` column remains; an invalid value is rejected by the CHECK constraint.

## 2. Enable catalog in one action (US1 / SC-001)

On a fresh namespace with `embedding_dim=128`:

```bash
# set dense_source=catalog with strategy params in a single config write
curl -X PUT localhost:2002/api/admin/v1/namespaces/demo/catalog \
  -b "$SESSION" -H 'Content-Type: application/json' \
  -d '{"dense_source":"catalog","strategy_id":"internal-hashing-ngrams","strategy_version":"v1","params":{"dim":128}}'

# catalog ingest now accepted
curl -X POST localhost:2001/v1/namespaces/demo/catalog \
  -H "Authorization: Bearer $NS_KEY" -H 'Content-Type: application/json' \
  -d '{"object_id":"item-1","content":"hello world"}'   # → 202
```

Expected: no separate enable toggle, no requirement to pre-set any other dense field, no
`dense_strategy_conflict` error possible.

## 3. Conflicting producers are unrepresentable (US2 / SC-002, SC-005)

```bash
# there is no field combination that selects two producers; dense_source is one value
curl -X PUT localhost:2002/api/admin/v1/namespaces/demo \
  -b "$SESSION" -H 'Content-Type: application/json' \
  -d '{"dense_source":"item2vec", ...}'   # switching producer is a single choice
```

Expected: the `dense_strategy_conflict` 400 no longer exists in the codebase or API.

## 4. BYOE object PUT blocked only under catalog (US2)

```bash
# dense_source=catalog → object BYOE rejected (409); subject BYOE still allowed
curl -X PUT localhost:2001/v1/namespaces/demo/objects/x/embedding -d '[...]'   # → 409
curl -X PUT localhost:2001/v1/namespaces/demo/subjects/u/embedding -d '[...]'  # → 204
```

## 5. Behavior parity per value (SC-003)

For each `dense_source`, confirm unchanged behavior:

| `dense_source` | Check |
|----------------|-------|
| `item2vec`/`svd` | run `make run-cron`; Phase 2 trains; recommend blends dense |
| `byoe` | object+subject BYOE PUT accepted; recommend blends dense |
| `catalog` | embedder upserts `{ns}_objects_dense`; recommend blends dense |
| `disabled` | Phase 2 skipped; recommend sparse-only |

## 6. Tests

```bash
make test         # all modules green, including updated nsconfig/compute/recommend/embedder tests
make test-e2e     # admin-plane + catalog flows
make lint
cd web/admin && npm run build   # single dense_source dropdown compiles
```

Expected: deleted conflict-validation tests are gone; new CHECK-rejection + backfill tests pass.
