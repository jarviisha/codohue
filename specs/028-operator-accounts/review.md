# Pre-merge review: operator accounts

Reviewed against `main` at `689b08d1cadde2ec7562284ba46c5372be4542ed` on 2026-09-17.
Scope: the complete PR diff, including production wiring, authorization, account
and token persistence, provisioning, migrations, Compose, deployment scripts,
SDK changes, browser login and regression coverage.

## Findings corrected

1. **P1 — read-only service tokens could mutate an object named `rankings`.**
   The data middleware classified every URL ending in `/rankings` as a read,
   including `PUT` and `DELETE /v1/namespaces/{ns}/objects/rankings`.
   Both regression cases reached the downstream handler before the fix.
   The exception now requires both POST and the exact namespace ranking path.
   Unit and live API E2E tests require 403 for the object mutation paths.

2. **P2 — immutable provisioning retries failed after dimension changes.**
   After an operator changed the namespace dimension, replaying the original
   provisioning request was incorrectly treated as another dimension change,
   or its catalog strategy was validated against the current dimension.
   PostgreSQL regression tests reproduced both failures. Provisioning now
   validates its initial dimension (including the schema default), while the
   repository continues to verify the immutable specification and credential
   and return current configuration without changing it. Ordinary updates keep
   their existing dense-collection dimension guard.

## Reviewed invariants

- Cookie and bearer capabilities remain separate; owner-only account, token
  and reset operations do not become accessible to service credentials.
- Session issuance checks the account version, and session validation joins
  current account state. Password changes and disable/demotion invalidate
  sessions; long-lived streams revalidate credentials.
- Login attempts reserve the shared PostgreSQL budget before bcrypt. Proxy
  headers only influence client identity when received through trusted CIDRs.
- Bootstrap and last-owner changes serialize through a transaction lock.
  Service-token provisioning is immutable, and retries cannot revive revocation.
- Namespace configuration, provisioning metadata and supplied key hash commit
  together. Conflicting provisioning does not overwrite existing credentials.
- Migration 028 adds tables; it does not rewrite existing namespace keys or
  event/vector data. Namespace deletion cascades to provisioning metadata.
- Compose separates owner, proxy and application secrets. Admin waits for init;
  consumers do not receive operator secrets through the default configuration.
- Browser mutations supply the CSRF header; authentication responses and public
  types match the username/password session contract.

## Validation

Passed locally: `make test`, `make test-race`, `make coverage-check-all` with
PostgreSQL, `make lint`, full `make test-e2e` with isolated PostgreSQL/Redis/Qdrant,
`make compose-check test-docker`, and admin SPA lint, tests and production build.
The two findings above have regression tests that failed before correction.

## Deployment conditions

This is deliberately not a drop-in authentication upgrade. Follow
[the migration runbook](../../deploy/operator-auth.md#migration-from-the-global-admin-key):
apply migration 028, create the initial owner, issue the admin proxy and automation
tokens, and prepare persistent secret files before switching binaries. Existing
Compose installations should disable initial application provisioning unless the
target namespace is already managed by this provisioning flow.

Existing consumer namespace keys remain valid. Old console cookies and global
key login do not. Production rejects legacy bearer compatibility. Admin now
publishes on loopback by default, so remote access needs the documented proxy
configuration. A rollback requires coordinated application/configuration/data
handling; it is not safe to simply run the down migration under new binaries.

No live deployment or production-data migration was performed during this review.
Local tests and CI do not establish that a particular host has completed these
preparations, nor guarantee the absence of all defects.
