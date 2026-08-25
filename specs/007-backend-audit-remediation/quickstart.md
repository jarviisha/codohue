# Quickstart: Verify Backend Audit Remediation

This is the implementation verification and rollout checklist. It deliberately excludes
`web/admin`.

## 1. Baseline and dependency gate

```bash
make lint
make build
make test
make test-race
make compose-check
go mod verify
make vuln
```

Expected: every command succeeds and the vulnerability scan reports zero reachable findings.
Run the module loop from `make vuln`, not only the root module.

## 2. Release 1 correctness checks

Start test infrastructure and apply migrations available in that release:

```bash
make up-infra
make migrate-up
make test-e2e
```

Verify these fixtures:

- Config database failure before Recommend/Rank/Trending returns 503 and performs no cache read
  or write.
- Missing namespace with a global admin credential returns 404 and creates no Qdrant collection
  or ID mapping.
- `object_created_at=now+5m` is accepted; a value beyond it returns 400; far-future legacy
  payloads never increase score or break JSON.
- Dense-delete failure makes object DELETE fail; retry after recovery returns 204.
- Re-embed from `strategy-a/v1` to `strategy-b/v1` resets all mismatched items and watcher
  completion waits for `strategy-b/v1`.
- A namespace with no events in the last 90 days is still visited by cron and stale owned vectors
  are removed according to the ownership matrix.
- Invalid one-request catalog provisioning leaves no base config mutation; author-write failure
  leaves no accepted catalog item.
- Equal `updated_at` rows across three catalog pages are returned exactly once with the keyset
  cursor.
- Public health contains no injected raw dependency error; `?details=true` is 404 without an
  observability token, 401 with a wrong token, and diagnostic with the correct token; metrics
  follows the same 404/401/protected behavior.

## 3. Redis retention canary

Before enabling retention, inspect every stream and group and confirm Redis 7+ with AOF and
adequate memory headroom. Deploy cursor fixes and removal of producer `MAXLEN` first.

Set:

```text
CODOHUE_STREAM_RETENTION_ENABLED=false
CODOHUE_STREAM_RETENTION_INTERVAL=1m
```

Observe one interval of computed frontier metrics without trimming. Then enable in this order:

1. Generation-1 embed streams.
2. `codohue:catalog`.
3. `codohue:events`.

For each stream, stop the consumer, publish more than ten normal retention windows of work, run
retention repeatedly, restart the consumer, and verify 100% processing. After each successful
pass, assert that no entry remains below the recorded safe frontier. If inspection, trim,
frontier, or group alerts fire, disable the flag; do not restore producer `MAXLEN`.

## 4. Lifecycle migration and dual-protocol rollout

After migrations 024 and 025:

```bash
make migrate-version
make test-e2e
```

Expected migration version: at least 25.

During mixed-version rollout:

- Keep delete/reset disabled.
- Confirm every internal producer stamps `namespace_generation`.
- Confirm generation-less entries are accepted only for grandfathered generation 1.
- Confirm all namespace mutations acquire a lifecycle lease and ID-map mutation without a lease
  fails in tests.

After every binary is upgraded, enable lifecycle enforcement and run the 100-iteration race
fixture. It must cover HTTP event/catalog/object/BYOE writes, both global streams, embedder,
cron/re-embed, delete, reset, and same-name recreation. After each successful delete/reset,
assert no PostgreSQL child row, current-generation Redis key, mapping, or Qdrant collection
exists. Publish an old-generation envelope after recreation and verify it is ACKed as stale with
zero writes.

Only after the SDK adoption window, run:

```bash
./tmp/admin lifecycle disable-legacy-envelopes --all --adoption-evidence <ref>
```

Verify every lifecycle has `legacy_messages_allowed=false`, the global closure timestamp is set,
and rerunning the command is a no-op success before enabling deleted-generation janitors.

## 5. ID mapping repair rehearsal

Use a production-shaped staging copy containing:

- the same string ID in two namespaces;
- the same string used as subject and object;
- old borrowed numeric IDs;
- sparse vectors whose indices use old subject IDs;
- catalog dense and client-provided object/subject dense vectors.

Run:

```bash
make build-admin
./tmp/admin idmap-repair audit
./tmp/admin idmap-repair apply --run <run-id>
./tmp/admin idmap-repair verify --run <run-id>
```

Audit must perform no Qdrant or mapping mutation. Apply must refuse without coordinated snapshot
references, with any quarantined payload, or without the global maintenance lease. Inject one
failure after every repair stage and verify `resume` converges without recopying verified work.

Verification must show:

- every manifest string ID resolves to its selected numeric ID;
- target payload IDs match;
- catalog/BYOE dense vector count and hashes are unchanged;
- sparse vectors were rebuilt using final subject IDs;
- no mismatched old point remains.

## 6. Production repair and rollback boundary

Schedule a maintenance window, stop or fence all external producers, acquire global maintenance,
take the coordinated PostgreSQL backup and all Qdrant snapshots, then run audit/apply/verify.
Unlock writers only after verify succeeds.

On failure, use `resume`. Do not run migration 022 down and do not roll back to a binary that
uses global `string_id` conflict semantics. A true rollback restores the recorded PostgreSQL and
Qdrant checkpoint together while maintenance mode remains active.

## 7. Final release gate

Repeat the complete baseline plus E2E and verify:

- zero reachable vulnerability finding;
- zero lost stream entry;
- zero stale-generation write in 100 races;
- zero non-finite score;
- zero skipped catalog row across cursor pages;
- zero public namespace metric or raw dependency error;
- clean `git diff --check` and no changes under `web/admin`.
