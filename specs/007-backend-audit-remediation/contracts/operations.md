# Operations Contract

## Namespace deletion

Deletion is a resumable state machine:

1. Acquire global-shared and namespace-exclusive lifecycle leases.
2. Transition `active -> deleting`; new leases and writes now fail.
3. Remove that generation's PostgreSQL children, Redis keys/stream, and Qdrant collections.
4. Verify absence, remove active config, and transition `deleting -> deleted`.
5. Report success only after verification.

Failure records `last_error`, remains `deleting`, and is retried through the same operation.
Recreation is forbidden until `deleted`; recreation increments generation. Global stream
entries are not scanned by namespace—the generation contract rejects stale entries and normal
retention later reclaims them.

## Application reset

Reset acquires the global-exclusive lease and persists `system_lifecycle=resetting` before
cleanup. All active namespaces transition to deleting; all recognized legacy and
generation-qualified artifacts are cleared; lifecycle tombstones remain. A process crash keeps
the system blocked until reset is retried and verified. Only then does system state return to
active.

## ID mapping repair CLI

The existing admin binary gains:

```text
admin idmap-repair audit
admin idmap-repair apply  --run <id>
admin idmap-repair verify --run <id>
admin idmap-repair resume --run <id>
```

`audit` is read-only and creates an immutable manifest. `apply` requires:

- system active and no competing repair;
- global-exclusive lifecycle lease;
- coordinated PostgreSQL backup reference;
- Qdrant snapshot reference for every affected collection;
- matching manifest hash;
- zero unresolved quarantined item.

Apply copies and verifies dense vectors before deleting old points. Sparse collections are
rebuilt after subject mappings settle. Any conflict or unavailable store halts the run and
keeps item-level resume state.

Sparse rebuild is exposed to `internal/core/idmap` as a narrow port. A `cmd/admin` composition
adapter invokes `internal/compute`; the repair core does not import the compute domain.

`verify` must prove mapping resolution, payload identity, dense vector count/hash preservation,
sparse rebuild completion, and absence of mismatched old points. The lifecycle lease is released
only after successful verification or a safely persisted resumable failure.

## Rollback

Migration 022 is forward-only once duplicate composite identities exist. Its down path performs
a duplicate preflight before dropping any constraint and refuses without mutation when unsafe.

Normal recovery is `resume`. True rollback requires restoring the PostgreSQL backup and every
Qdrant snapshot from the same repair checkpoint while writers remain frozen. Partial store
restore and old-binary rollback are unsupported.

## Legacy envelope closure

The existing admin binary gains:

```text
admin lifecycle disable-legacy-envelopes --all --adoption-evidence <ref>
```

The command acquires the global-exclusive lifecycle lease, verifies that adoption evidence is
present, atomically disables `legacy_messages_allowed` for every namespace, and records the
global closure timestamp. Repeating it is a no-op success. No ordinary create, recreate, or
config operation can re-enable legacy envelopes after closure.

## Deployment order

1. Release 1 isolated fixes; run a compensating full re-embed where strategy identity may have
   been skipped.
2. Release 2 with retention disabled; inspect/canary frontiers; enable streams sequentially.
3. Release 3 additive schema and dual envelope support; pause delete/reset during mixed binary
   versions; enable enforcement after every producer/consumer understands generation.
4. Disable legacy envelopes with the guarded admin command after producer adoption, verify the
   global closure timestamp, then enable deleted-generation janitors.
5. Release 4 audit, snapshots, maintenance freeze, apply, verify, and unlock.

At every stage, `make test-race` and E2E must use the exact dependency and Redis/PostgreSQL/Qdrant
versions deployed in production.
