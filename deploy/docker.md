# Docker deployment

The three Compose files are standalone layouts. They share the same four app
images and migration image; each file keeps common connection settings and
startup dependencies in YAML anchors.

| Layout | Command | Infrastructure |
| --- | --- | --- |
| Local full stack | `make up-build-d` | PostgreSQL, Redis, Qdrant; automatic migrations |
| App-only on Linux | `make up-app-build-d` | Existing services reachable through host networking |
| Published images | `bash deploy/deploy.sh` | Local profiles, external services, or a mixture |

Use Docker Compose 2.24+ and run commands from the repository/deployment directory.
`deploy.sh` also works from another directory. API, cron, and embedder wait for
successful migrations and healthy local Redis/Qdrant; admin waits for the API.
App containers restart unless stopped and get 40 seconds to exit gracefully.
API and embedder probe `/healthz`; admin probes the embedded SPA at `/`.
Qdrant probes `/readyz`. Cron has no HTTP healthcheck: deployment verifies that
its process is running, not that a batch has completed.

## App-only

Copy `.env.app.example` to `.env.app` and set the external connections and admin
key. Containers load `.env` first, then `.env.app`. The latter takes precedence,
including retention flags and observability tokens. Host networking uses the
host's ports directly; `host.docker.internal` maps to loopback in this layout.
If changing the API port, also change `CODOHUE_API_URL` for the admin proxy.
Apply migrations to the same database before starting the apps.

## Published images

Keep `compose.prod.yaml`, `deploy/deploy.sh`, and a private `.env` together
on the host. Configure the production section of `.env.example`:

- Set `CODOHUE_ADMIN_API_KEY` and an `IMAGE_TAG` matching the published release.
- For each local infrastructure service, enable its profile: `local-db`,
  `local-redis`, or `local-qdrant`. Set `CODOHUE_POSTGRES_PASSWORD` for local DB.
- For each external service, omit its profile and set its `CODOHUE_*` connection
  variable. All apps must connect to the same PostgreSQL, Redis, and Qdrant.
- Set published port overrides when sharing a host with another stack.

Log in to GHCR, then run `bash deploy/deploy.sh`. An exported `IMAGE_TAG` overrides
the value in `.env`. The script validates configuration, pulls images, and waits
for every service to be running or healthy. It prints status/logs and returns a
failure on startup errors or timeouts. It does not prune host images.

## Database migrations and upgrades

The local PostgreSQL image creates `codohue` on first initialization. Provision
external databases before deployment. The migration image only applies schema
migrations; its role needs schema privileges, not database-creation privileges.
A failed migration exits without automatic retries and blocks app startup.
Investigate the migration error/dirty version before retrying; never force a
schema version simply to get deployment past a failure.

`CODOHUE_MIGRATION_DATABASE_URL` defaults to the application's production URL.
If `CODOHUE_DATABASE_URL` has pgx-specific `pool_*` options, set the migration URL
to the same database without those options; the migrate PostgreSQL driver does
not accept pgx pool settings. The same distinction applies to host-side migrate.

Keep the Compose project name/directory and named volumes unchanged on upgrade.
Container names now follow Compose defaults; use `docker compose exec SERVICE`
and `docker compose logs SERVICE` instead of hard-coded names. Local full-stack
infra and embedder published ports now bind to `127.0.0.1`.

Changing an image tag does not reverse migrations. Verify old-binary schema
compatibility before rollback and follow the relevant migration/restore runbook.
