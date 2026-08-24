# Phase 0 Research: Backend Audit Remediation

## Decision 1: Upgrade vulnerable dependencies and make scanning a release gate

**Decision**: Upgrade pgx to at least v5.9.2, `x/text` to at least v0.39.0, and gRPC to at
least v1.82.1. Pin the scanner tool version in CI while accepting that its vulnerability
database changes over time.

**Rationale**: The audit found reachable symbols for all three modules and fixed compatible
releases already exist. A recurring gate prevents the same class from silently returning.

**Alternatives considered**: Suppressing findings based on current call context was rejected;
future config/input changes could make the vulnerable paths exploitable and the compatible
upgrades are lower risk than maintaining suppressions.

## Decision 2: Trim streams from consumer progress, never producer length

**Decision**: Remove every producer-side `MAXLEN`. Once per minute, inspect all consumer
groups and trim only IDs strictly below the minimum safe frontier: oldest pending ID when a
PEL is non-empty, otherwise last-delivered ID. Use `XTRIM MINID ~`, fail closed on inspection
errors, and retain unexpected groups in the frontier calculation.

**Rationale**: Length caps and TTL know nothing about processing progress and can delete the
only durable copy during an outage. Redis documents `XAUTOCLAIM` as cursor-based PEL scanning
and confirms that deleted stream entries create PEL cleanup behavior rather than restoring
their payload ([Redis XAUTOCLAIM](https://redis.io/docs/latest/commands/xautoclaim/)). Redis
8.2 offers native multi-group-aware `XACKDEL ... ACKED`, but production is Redis 7
([Redis XACKDEL](https://redis.io/docs/latest/commands/xackdel/)).

**Alternatives considered**: `XADD MAXLEN`, TTL, and per-message `XACK`+`XDEL` were rejected
because they are not safe for multiple groups. A Lua group check on every ACK was rejected as
hot-path overhead and still embeds retention policy in consumers.

## Decision 3: Persist namespace generation and fence writers with lifecycle leases

**Decision**: Keep a tombstone lifecycle row after deletion, increment generation on same-name
recreation, and combine generation-tagged stream envelopes with PostgreSQL shared-writer /
exclusive-delete leases. Persist global reset state. Generation 1 maps to legacy physical
names; later generations use versioned Redis keys/streams and Qdrant collections.

**Rationale**: Locks prevent in-flight writers from finishing after deletion but cannot
identify old queued work. Generation identifies old work but cannot stop an already-running
Qdrant write. Both are required for a complete cross-store boundary. Persistent reset state
keeps the system blocked after a reset process crash.

**Alternatives considered**: FKs alone, scanning global streams by namespace, permanently
forbidding name reuse, generation without locks, and locks without generation each leave a
known resurrection path.

## Decision 4: Retain backward compatibility only for first-generation stream envelopes

**Decision**: Backfilled generation-1 namespaces temporarily accept envelopes without a
generation. New or recreated namespaces require an exact generation, and an adoption gate
later disables the legacy allowance everywhere.

**Rationale**: Existing Redis SDK clients cannot be switched atomically with servers. The
grandfathered rule preserves their current namespaces without permitting a legacy backlog to
enter generation 2.

**Alternatives considered**: Accepting missing generation forever was rejected as unsafe;
requiring it immediately was rejected as an avoidable producer outage.

## Decision 5: Compute from configured namespaces and clean by ownership

**Decision**: Cron enumerates every configured active namespace, including empty event
windows. Empty keep sets clear sparse vectors. Item2vec/SVD clear cron-owned item and subject
dense vectors; catalog preserves embedder-owned objects but clears cron-owned subject vectors;
BYOE preserves client-owned dense vectors.

**Rationale**: Namespace activity is not an ownership registry. Explicit ownership prevents
both permanent stale vectors and accidental deletion of vectors the server cannot rebuild.

**Alternatives considered**: Extending the event window only postpones the bug. Clearing all
dense collections would destroy catalog/BYOE state.

## Decision 6: Freeze re-embed identity as a tuple

**Decision**: Reset selection, stale counts, embedded counts, watcher progress, and completion
all use `(strategy_id,strategy_version)` stored on the batch run.

**Rationale**: Strategy versions are scoped to a strategy, not globally unique. The schema
already stores both fields, so no compatibility migration is required.

**Alternatives considered**: Globally unique version strings were rejected because the
strategy seam explicitly permits independent future strategies.

## Decision 7: Reject unsafe timestamps and defend scoring independently

**Decision**: Apply the existing five-minute skew limit to every client-controlled
`object_created_at`. Centralize freshness calculation with non-negative age and multiplier
bounded to `[0,1]`; drop and observe any non-finite candidate before serialization.

**Rationale**: Validation protects new writes while defensive scoring keeps legacy/corrupt
payloads from breaking JSON or boosting rank.

**Alternatives considered**: Validation alone leaves existing bad Qdrant payloads dangerous;
clamping alone silently accepts malformed client data.

## Decision 8: Fail closed and make compound writes atomic

**Decision**: Required config failures return 404/503 before cache or mutation. Object delete
returns success only after all stores are clean. Namespace+catalog configuration and catalog
content+author attribution use one PostgreSQL transaction each.

**Rationale**: These operations already target one PostgreSQL database or idempotent external
deletes. False success is harder to repair than an explicit retryable failure.

**Alternatives considered**: Degraded defaults, best-effort metadata, and prevalidation plus
two autocommit writes all retain partial-state or semantic-drift windows.

## Decision 9: Repair migration 022 from a durable cross-store manifest

**Decision**: Add an audit/apply/verify/resume workflow under `cmd/admin`. Freeze all writers,
take coordinated PostgreSQL and Qdrant snapshots, inventory payload IDs and vector hashes,
copy unrecomputable dense points to authoritative IDs, rebuild sparse coordinates, and verify
before unlock. Treat post-duplicate migration 022 as forward-only.

**Rationale**: Sparse object vectors encode subject numeric IDs, while catalog and BYOE dense
vectors may not be server-recomputable. A database-only key fix or normal cron run cannot
prove preservation.

**Alternatives considered**: Blind full recompute, lazy repair on lookup, and SQL-only rollback
were rejected because they lose BYOE state, add hot-path ambiguity, or can partially destroy
the key scheme.

## Decision 10: Use keyset reconciliation and authenticated metrics

**Decision**: Catalog reconciliation returns an opaque cursor representing `(updated_at,id)`.
Public health retains its status shape but contains no raw dependency errors. Metrics require
a dedicated observability bearer token and are absent when it is not configured.

**Rationale**: A total-order cursor prevents equal-timestamp gaps. A separate monitoring token
preserves existing deployment topology without exposing tenant labels or giving Prometheus the
global admin credential.

**Alternatives considered**: Offset plus timestamp remains unstable; binding only to loopback
does not protect bare-metal or misconfigured deployments; reusing the global admin key violates
least privilege.
