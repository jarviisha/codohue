# Issue 57: operator accounts and credential separation

Branch: `feat/admin-operator-accounts`.

Use local bcrypt operator accounts with owner/admin roles. PostgreSQL is the shared authority for accounts, opaque sessions, login budgets, service tokens and audit records; it already serves as the durable configuration store, so revocation is independent of Redis availability. Session issuance binds the account version and validation joins live account state. Account updates serialize last-owner invariants and invalidate all sessions. Bootstrap is a separate local, initial-only operation; recovery requires explicit database authority.

Administrative service permissions and data permissions are independent and namespace-scoped. The admin data proxy has an explicitly issued data token. Global bearer compatibility is opt-in outside production, rejected by production deployment defaults, and cannot create human sessions or manage identities. Cookie mutations require a custom CSRF header plus Origin validation. Trusted proxy CIDRs are explicit. Login attempts reserve a shared per-IP budget before bcrypt.

Namespace provisioning takes a pre-generated 32-byte hex secret. Its hash, namespace configuration and immutable provisioning specification digest commit atomically. Retrying the same desired state returns current state without reapplying it; conflicts never rotate credentials. Compose init runs with operator-controlled DB credentials and secret files before dependent applications.

Migration 028 adds six security/provisioning tables. Existing namespace wire contracts remain compatible; new operator/token contracts live in pkg/codohuetypes with golden snapshots. Local OIDC/SSO and MFA are deferred; issue 41's narrower JWT replacement is superseded. Required checks: unit, race, full E2E, frontend build/lint/URL tests, Compose validation, and security regression coverage for replicas, concurrent bootstrap/provisioning, CSRF and cross-namespace denial.
