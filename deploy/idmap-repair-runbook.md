# ID-map repair runbook (migration 022 reconciliation)

Migration 022 re-keyed `id_mappings` from a global `string_id` primary key to
`(namespace, entity_type, string_id)`. Rows that were previously shared — the
same string used in two namespaces, or as both a subject and an object — kept
one namespace's numeric id, so Qdrant points can be stored under a numeric id
that no longer resolves to the identity that owns them.

This runbook covers the `cmd/admin idmap-repair` workflow that reconciles them.

## When you need it

Run the audit when any of these is true:

- Migration 022 was applied to a deployment that had cross-namespace or
  cross-entity string ids.
- A full recompute per namespace was **not** run immediately after 022.
- Rolling back 022 or 026 fails with the duplicate preflight error.

The audit is read-only. Run it first regardless — it either reports a clean
fleet or tells you exactly what is wrong.

## Why a full recompute is not enough

A normal recompute rebuilds sparse vectors from `events`, but it cannot
reproduce:

- **BYOE dense vectors** — pushed by the client through
  `PUT /v1/namespaces/{ns}/{objects,subjects}/{id}/embedding`. The server never
  had the source material.
- **Catalog dense vectors** produced by a strategy version that is no longer
  registered.

Those points have to be *moved*, byte-identical, to the authoritative numeric
id. That is what this workflow does, and why it verifies hashes rather than
trusting the copy.

## Preconditions

- Migration 026 is applied (`make migrate-up`).
- You can take a PostgreSQL backup and a Qdrant snapshot per affected
  collection at the same checkpoint.
- A maintenance window: apply holds the **global exclusive lifecycle lease**,
  which blocks every writer in the fleet (API ingest, catalog ingest, cron,
  embedder).

## 1. Audit (read-only)

```bash
./tmp/admin idmap-repair audit
```

Output names the run id, the manifest hash, how many identities need repair,
and every quarantined item. **Nothing is mutated.**

The audit quarantines rather than guesses. An item is quarantined when:

| Reported | Meaning | Resolution |
|----------|---------|------------|
| `point payload carries no logical id` | A Qdrant point has no `subject_id`/`object_id` in its payload, so it cannot be tied to a mapping | Delete the orphan point, or re-ingest the object so the payload is rewritten |
| `two points in one collection claim this identity` | Two numeric ids in the same collection carry the same logical id; the authoritative vector is ambiguous | Decide which is current, delete the other, re-audit |
| `points exist for an identity with no id_mappings row` | Points reference an identity PostgreSQL has never mapped | Re-ingest the object/subject so a mapping is minted, or delete the points |

Apply refuses to run while any item is quarantined. Resolve them all, then run
`audit` again — a new run supersedes the old manifest, provided the previous
run has not started applying.

To re-check what is still blocking a run without discarding its manifest:

```bash
./tmp/admin idmap-repair quarantine --run 7
```

## 2. Coordinated snapshots

Take both, at the same checkpoint, while no writer is active:

```bash
pg_dump "$DATABASE_URL" -Fc -f /backups/codohue-idmap-repair.dump

# One per collection named in the audit output.
curl -X POST "http://$QDRANT_HOST:6333/collections/<collection>/snapshots"
```

Record the references — they are required arguments to `apply`, and they are
the only rollback path.

## 3. Apply

```bash
./tmp/admin idmap-repair apply \
  --run 7 \
  --pg-snapshot /backups/codohue-idmap-repair.dump \
  --qdrant-snapshot prod_objects=prod_objects-2026-08-25.snapshot \
  --qdrant-snapshot prod_objects_dense=prod_objects_dense-2026-08-25.snapshot
```

Apply refuses to start when:

- The manifest hash no longer matches what was audited (something changed
  underneath you — re-audit).
- Any item is quarantined.
- Either snapshot reference is missing.

Apply requires a snapshot for **every** collection the manifest touches, not
just one — it names the uncovered collections if any are missing.

Per identity it copies the **dense** point to the authoritative id, verifies the
payload and vector hashes, retargets the mapping, then deletes the original —
in that order, so a failure at any step never loses an unrecomputable vector.
Sparse collections are not copied: their coordinates encode subject numeric ids,
so the full recompute that follows rebuilds them once every mapping has settled.

## 4. Verify before unlocking

```bash
./tmp/admin idmap-repair verify --run 7
```

Verification checks every manifest tuple: the target point exists, carries the
right logical id, still hashes to the payload and vector recorded at audit
time, and no old point remains. It also confirms every namespace in the
manifest had its sparse vectors rebuilt. Failures are reported per identity
with the reason, and each confirmed item is promoted from `cleaned` to
`verified` so the item record matches the run record. It exits non-zero when anything is unfinished —
**do not unlock the fleet until it passes.**

## 5. Resume after a failure

Apply persists per-item state, so a crash or transient store failure is
resumed, not restarted:

```bash
./tmp/admin idmap-repair resume --run 7
```

Resume skips every item already cleaned and continues from the first that is
not. It re-checks the manifest hash and the snapshots, exactly like apply.

## Rollback

Migration 022 is **forward-only** once composite duplicates exist — restoring
the global `string_id` key would have to merge two legitimately distinct
identities. Both `022` and `026` down-migrations refuse before touching any
constraint when duplicates are present, so a rollback attempt leaves the schema
in a known state rather than half-migrated.

A true rollback restores *both* stores from the same checkpoint, with the
global lease still held:

```bash
pg_restore -d "$DATABASE_URL" --clean /backups/codohue-idmap-repair.dump

curl -X PUT "http://$QDRANT_HOST:6333/collections/<collection>/snapshots/recover" \
  -H 'Content-Type: application/json' \
  -d '{"location":"file:///qdrant/snapshots/<collection>/<snapshot>"}'
```

Restoring only one store reintroduces exactly the cross-store divergence the
repair exists to fix.

## After a successful run

1. Release the maintenance window.
2. Run one full cron tick per namespace and confirm `batch_run_logs` shows a
   successful run.
3. Spot-check recommendations for a subject whose identity was repaired.
4. Record the run id, manifest hash and snapshot references in the release
   notes — that tuple is what makes the change auditable later.
