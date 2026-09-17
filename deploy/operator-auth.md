# Operator accounts and application provisioning

Migration 028 separates human console sessions, administrative automation and consumer credentials. This release includes local accounts; OIDC/SSO and MFA are deferred extensions. It supersedes issue #41's JWT replacement: production sessions are opaque and stored in PostgreSQL, not signed JWTs or process-local revocation lists.

## Fresh Compose installation

Generate secrets once, before `make up-d` (do not regenerate them on each deployment):

```sh
mkdir -p .secrets
umask 077
openssl rand -hex 24 > .secrets/owner_password
openssl rand -hex 32 > .secrets/admin_proxy_token
openssl rand -hex 32 > .secrets/application_token
# The image runs as UID 65532. Make the files readable to that UID only.
sudo chown 65532:65532 .secrets/owner_password .secrets/admin_proxy_token .secrets/application_token
cp .env.example .env
make up-d
```

Read the owner password through an operator-controlled secret manager or privileged local editor; do not log it. Sign in as `owner` at `http://localhost:2002`. Set `CODOHUE_BOOTSTRAP_USERNAME` to choose another initial username. The application namespace defaults to `app`; change `CODOHUE_APPLICATION_NAMESPACE` before initial provisioning and set `CODOHUE_NAMESPACE_CONFIG_PATH` to an operator-owned copy of `deploy/examples/namespace.json`. Keep that file outside deployment-managed examples so upgrades cannot replace your desired specification.

The trusted `provision` init service waits for migrations, creates the initial owner only when there are no accounts, provisions the admin proxy token, and creates the application namespace with the pre-generated application token. Admin startup waits for successful completion. Concurrent init runs are serialized. Retrying the same specification succeeds without updating existing configuration or returning secrets. Different credentials, a different initial specification, an existing namespace without provisioning metadata, or a revoked service token cause a conflict. Resolve conflicts explicitly; startup never silently rotates credentials.

A consumer service should depend on `provision` with `condition: service_completed_successfully` and mount only `application_token`. Use its contents as the bearer credential on `/v1/namespaces/app/*`. A consumer does not need the owner password, proxy token, database credentials or a provisioning token. Only the operator-controlled init service receives provisioning authority. Membership in the Docker network grants no HTTP administrative authority. Direct access to PostgreSQL, Redis streams or Qdrant is privileged infrastructure access and must not be given to independent consumers.

Both Compose deployments publish admin on `127.0.0.1` by default. The host-network layout also binds admin to loopback. For remote access, use a TLS reverse proxy, set `CODOHUE_ADMIN_COOKIE_SECURE=true`, and explicitly list proxy CIDRs in `CODOHUE_TRUSTED_PROXIES`. Forwarded client addresses are walked from the trusted edge toward the caller. Untrusted forwarded headers are ignored. Avoid broad proxy CIDRs that include clients.

Production sets `CODOHUE_ENV=production` and rejects legacy authentication and `dev-secret-key`. An optional Bluesky feeder no longer receives a default admin credential; provision its namespace separately, or explicitly issue an appropriately scoped administrative service token for its bootstrap path.

## Native binaries, account lifecycle and recovery

Apply migration 028 first. Commands below read `DATABASE_URL` or `DATABASE_URL_FILE`; database access is the local operator authority. They accept secrets through files only, never command-line plaintext, and never print secrets.

```sh
tmp/admin access bootstrap --username owner --secret-file /secure/owner-password
tmp/admin access token --name admin-proxy --secret-file /secure/proxy-token --permissions data:read,data:write --namespaces '*'
tmp/admin access provision --name app --secret-file /secure/application-token --config-file deploy/examples/namespace.json
```

Configure `CODOHUE_ADMIN_PROXY_TOKEN_FILE=/secure/proxy-token` for admin's data API reads and event injection. It is an explicitly issued data token; it cannot authenticate to administrative endpoints. An alternative first-run mechanism sets both `CODOHUE_BOOTSTRAP_USERNAME` and `CODOHUE_BOOTSTRAP_PASSWORD_FILE` on admin. Restarting with these variables never overwrites an existing account. Remove the bootstrap secret mount after provisioning if it is no longer needed.

Owners manage accounts using `PUT /api/admin/v1/accounts/{username}` with `{"password":"a-new-long-password","role":"admin","disabled":false}`. Passwords must be 12–72 bytes. An omitted password preserves the existing hash; new accounts require a password. Every update invalidates all sessions for that account. `GET /api/admin/v1/accounts` includes disabled accounts. The last enabled owner cannot be disabled or demoted. Admin accounts operate namespaces but cannot manage accounts, service tokens or reset the instance. There is no public registration.

If every owner credential is lost, run the explicit local recovery command:

```sh
tmp/admin access recover --username owner --secret-file /secure/replacement-password
```

Recovery creates or re-enables the named owner, changes its password, invalidates its sessions and records the local recovery actor. It requires database authority and is never exposed as an unauthenticated HTTP operation.

## Service credentials and browser sessions

Owner-authenticated `PUT /api/admin/v1/service-tokens/{name}` accepts `{"token":"<64 hex characters>","permissions":["admin:read"],"namespaces":["shop"]}`. Generate tokens with a cryptographic RNG, such as `openssl rand -hex 32`. Permissions are explicit: `admin:read`, `admin:write`, `namespace:provision`, `data:read`, `data:write`. An explicit `*` namespace permits fleet-wide access; otherwise administrative aggregate routes are denied. `namespace:provision` requires `provision_api_key` on the namespace PUT and cannot modify an existing namespace through ordinary upsert semantics. Service tokens cannot manage accounts/tokens, reset the instance or create human sessions. Consumer keys authorize data operations only in their own namespace.

Revoke via `DELETE /api/admin/v1/service-tokens/{name}` or `tmp/admin access revoke-token --name NAME`. Reusing a revoked name fails; use a new name and secret for replacement. Names and permissions are not secrets. Only bcrypt password/application-key hashes and SHA-256 hashes of random service/session tokens are persisted.

Browser login is `POST /api/v1/auth/sessions` with `{"username":"owner","password":"..."}` and `X-Codohue-CSRF: 1`. The response contains `expires_at` and `actor: {name, role}`; the opaque token is only in the HttpOnly, SameSite=Strict cookie. `GET /api/v1/auth/sessions/current` returns the individual identity. Cookie-authenticated mutations require the CSRF header and, when supplied, a matching Origin. Bearer requests do not use cookies. Five login attempts per minute per client IP are reserved atomically before password verification; a matching candidate after the budget is exhausted receives 429. Shared PostgreSQL state preserves the budget and all revocations across processes. Expired security state is pruned hourly. Sessions expire after eight hours. Open session- or service-token-authenticated SSE streams recheck their credential every 15 seconds and close on revocation or identity-store failure.

Audit rows in `admin_audit` contain actor, action, target and time, without request bodies, query strings or credential headers. HTTP mutation audit entries record authorized attempts, not a guarantee of successful completion; account/token changes also record their completed action in the same transaction. Preserve/export this table under the installation's audit retention policy. Instance data reset does not remove accounts or audit records.

## Migration from the global admin key

1. Back up PostgreSQL and apply migration 028. Provision an owner via the local CLI. Existing namespace key hashes remain valid. When upgrading an existing Compose installation, set `CODOHUE_PROVISION_APPLICATION=false` so init creates only the owner and proxy token and preserves existing namespaces. The three configured secret files must still exist; the application file may contain an existing consumer secret or an unused random token when namespace provisioning is disabled. Re-enable namespace provisioning only for a new namespace or one already managed by this provisioning path.
2. Issue a distinct data token for admin proxy calls and service tokens for automation, with only required permissions/namespaces. Update integrations and store their secrets outside source control. Ordinary namespace creation still returns a generated key once; deployments requiring retry-safe creation should use `provision_api_key` or the CLI.
3. Update admin UI clients to username/password plus the CSRF header. Old JWT cookies are invalidated; all operators sign in again. `CODOHUE_ADMIN_SESSION_SECRET` is no longer used.
4. If a staged rollout needs the old bearer key, explicitly set `CODOHUE_LEGACY_ADMIN_AUTH=true` on API and admin outside the production deployment path. During this limited transition the legacy key still accesses every data namespace and ordinary administrative operations, but cannot sign into the console, manage identities/tokens or reset the instance. Supplying the old key without this flag fails startup with migration guidance. This prevents an unnoticed compatibility bypass or silent lockout.
5. After integrations use their new tokens, remove both legacy settings and use `CODOHUE_ENV=production`. Production Compose does not enable compatibility. Do not downgrade the application or migration while depending on operator accounts and service tokens; restore a coordinated backup if rollback is unavoidable.

`DATABASE_URL`, `REDIS_URL`, `CODOHUE_ADMIN_API_KEY` (migration only), `CODOHUE_ADMIN_PROXY_TOKEN`, `CODOHUE_BOOTSTRAP_PASSWORD` and `CODOHUE_OBSERVABILITY_TOKEN` support `_FILE`. Setting both a value and its file is an error. Missing or empty files fail startup. Only trailing CR/LF from secret files is removed.
