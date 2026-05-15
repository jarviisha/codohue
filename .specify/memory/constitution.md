<!--
SYNC IMPACT REPORT
==================
Version change: [UNVERSIONED] → 1.0.0
Modified principles: N/A (initial ratification)
Added sections:
  - Core Principles (I–IV)
  - Architecture Constraints
  - Quality Gates & Review Process
  - Governance
Removed sections: N/A (initial ratification)
Templates requiring updates:
  ✅ .specify/templates/plan-template.md — Constitution Check gates aligned
  ✅ .specify/templates/spec-template.md — No structural changes required
  ✅ .specify/templates/tasks-template.md — No structural changes required
Follow-up TODOs: None — all placeholders resolved.
-->

# Codohue Constitution

## Core Principles

### I. Code Quality (NON-NEGOTIABLE)

Every domain MUST be organized under `internal/<domain>/` with a consistent,
predictable file structure: `handler.go`, `service.go`, `repository.go`,
`types.go`, and `docs.go`. Each package MUST have a `docs.go` containing only
the canonical `// Package <name> ...` doc comment — no package-level
explanations scattered across other files.

Import boundaries are strictly enforced: domains MAY import `core/`, `infra/`,
and `config/`; domains MUST NOT import each other. The sole exception is that
`recommend` MAY import `nsconfig` for config lookups.

Comments MUST be written in English. Inline comments MUST explain WHY, not
WHAT — well-named identifiers already describe what code does. Multi-line
comment blocks and multi-paragraph docstrings are prohibited. Code MUST NOT
introduce abstractions beyond what the current task requires; three similar
lines are preferable to a premature abstraction.

**Why**: Consistency across 6+ domains prevents cognitive overhead and import
cycles. English-only comments ensure the full team can read and audit any file.

### II. Testing Standards (NON-NEGOTIABLE)

Every file containing business logic — `service.go`, `repository.go`,
`job.go`, `worker.go` — MUST have a corresponding `_test.go` file. Handler
tests MUST live in `handler_test.go`. Files that only declare types (`types.go`)
or wire dependencies (`docs.go`) do NOT require test files.

Tests MUST be independently runnable via `make test` or `make test-pkg`. No
test MAY depend on external state that is not explicitly set up within the
test or a shared test helper. New behavioral changes MUST include tests that
verify the changed behavior before the change is considered complete.

**Why**: Missing test files for business logic have historically allowed silent
regressions in the ingest, compute, and recommend pipelines. The rule is
structural — tied to file names — to make compliance easy to audit via tooling.

### III. API Consistency

All REST endpoints MUST follow the `/v1/<resource>` path convention. Error
responses MUST use a uniform JSON structure across all handlers. Authentication
MUST use the two-tier model: the global `CODOHUE_ADMIN_API_KEY` for admin routes
(namespace config upsert); per-namespace bcrypt-hashed keys for data routes,
with fallback to the global key when a namespace has no provisioned key.

No new endpoint MAY be added without a corresponding entry in the REST API
table in `CLAUDE.md`. Endpoint behavior MUST be idempotent where specified
(e.g., `DELETE /v1/objects/{ns}/{id}` is explicitly idempotent).

**Why**: Inconsistent auth or error shapes break downstream consumers silently.
The API table in CLAUDE.md is the authoritative contract — keeping it current
prevents drift between implementation and documentation.

### IV. Performance Requirements

Recommendation responses MUST be served with a Redis cache layer: results are
cached for 5 minutes per `(namespace, subject_id, limit)` key. Cache MUST be
invalidated or bypassed only on explicit config changes, never on every request.

The cron batch job MUST complete its full recompute cycle within the configured
`CODOHUE_BATCH_INTERVAL_MINUTES`. If a phase (sparse, dense, trending) cannot complete
within that budget, it MUST log a warning and continue rather than blocking
subsequent phases.

Cold-start subjects (0 interactions) MUST fall back to Redis trending within a
single request round-trip. Subjects with fewer than 5 interactions MUST receive
a 70/30 trending-to-CF blend without additional latency.

Events older than 90 days MUST be excluded from all vector computations.
Freshness decay (`e^(-λ × days_since)`) and object freshness (γ-based rerank)
MUST be applied consistently — never skipped for performance reasons.

**Why**: The full-recompute strategy is intentional (avoids incremental race
conditions). Performance targets ensure that recompute cost does not bleed into
recommendation serving latency.

## Architecture Constraints

The system MUST run as exactly two binaries: `cmd/api` (HTTP + Redis Streams
ingest worker) and `cmd/cron` (batch recompute daemon). Adding a third binary
requires explicit governance approval and a documented rationale.

All string ID → numeric Qdrant point ID mappings MUST go through the
`id_mappings` table (BIGSERIAL). Hash-based ID generation is prohibited due to
collision risk.

Database schema changes MUST be expressed as numbered migration pairs
(`NNN_name.up.sql` / `NNN_name.down.sql`) in `migrations/`. Direct schema
mutations outside the migration system are prohibited.

The full-recompute strategy for vectors MUST be maintained. Incremental
online updates (e.g., online Word2Vec) are explicitly forbidden because they
cause catastrophic forgetting in the embedding models.

## Quality Gates & Review Process

Every pull request MUST pass:
- `make lint` — zero golangci-lint violations
- `make test` — all tests green
- `make build` — both binaries compile without error

A PR that adds a new domain MUST include `docs.go`, at least one `_test.go`
for business logic files, and an updated CLAUDE.md REST API table entry for
any new endpoints.

Code review MUST verify:
1. Import boundary compliance (no cross-domain imports)
2. English-only comments
3. Test file coverage for all `service.go`, `repository.go`, `job.go`, `worker.go`
4. No abstraction introduced beyond the immediate task

Performance-sensitive changes (recommend path, compute phases) MUST include a
before/after latency rationale in the PR description.

## Governance

This constitution supersedes all other development practices. Amendments
require:
1. A documented rationale explaining the change and its impact.
2. A version bump following semantic versioning:
   - **MAJOR**: Removal or redefinition of a core principle.
   - **MINOR**: New principle or materially expanded guidance.
   - **PATCH**: Clarifications, wording, or non-semantic refinements.
3. Updates to all dependent templates and `CLAUDE.md` as part of the same
   change set.

All PRs and code reviews MUST verify compliance with this constitution.
Complexity violations require justification in the Complexity Tracking section
of the relevant `plan.md`. Runtime development guidance lives in `CLAUDE.md`.

**Version**: 1.0.0 | **Ratified**: 2026-04-28 | **Last Amended**: 2026-04-28
