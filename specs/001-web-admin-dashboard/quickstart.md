# Quickstart: Web Admin Dashboard

**Date**: 2026-04-28 | **Plan**: [plan.md](./plan.md)

---

## Prerequisites

- `cmd/api` already running (admin proxies health/recommend/trending to it)
- PostgreSQL running with migration 006 applied (`make migrate-up`)
- `RECOMMENDER_API_KEY` env var set (same value as `cmd/api`)
- Node.js 20+ and npm for building the React frontend

---

## Development Workflow

### 1. Build the React frontend

```bash
cd web/admin
npm install
npm run build        # outputs to web/admin/dist/
```

For hot-reload during frontend development:
```bash
npm run dev          # Vite dev server on :5173 — proxies /api/* to :2002
```

### 2. Run cmd/admin locally

```bash
# Requires infra up (make up-infra) and cmd/api running (make run)
make run-admin
# or directly:
go run ./cmd/admin
```

Admin dashboard available at `http://localhost:2002`.

### 3. Login

Open `http://localhost:2002` → enter the value of `RECOMMENDER_API_KEY` → submit.

---

## Docker Compose integration

`cmd/admin` is added as a new service in `docker-compose.yml`:

```yaml
admin:
  build:
    context: .
    dockerfile: Dockerfile.admin
  ports:
    - "2002:2002"
  environment:
    - DATABASE_URL=${DATABASE_URL}
    - REDIS_URL=${REDIS_URL}
    - RECOMMENDER_API_KEY=${RECOMMENDER_API_KEY}
    - CODOHUE_API_URL=http://api:2001     # internal URL for proxying
    - CODOHUE_ADMIN_PORT=2002
  depends_on:
    - api
```

---

## Environment Variables (cmd/admin)

| Variable | Required | Default | Notes |
|----------|----------|---------|-------|
| `DATABASE_URL` | Yes | — | Same as cmd/api |
| `RECOMMENDER_API_KEY` | Yes | — | Used for session auth and proxy auth |
| `CODOHUE_API_URL` | Yes | `http://localhost:2001` | Internal URL of cmd/api |
| `CODOHUE_ADMIN_PORT` | No | `2002` | Port for cmd/admin HTTP server |
| `CODOHUE_LOG_FORMAT` | No | `text` | `text` or `json` |

---

## Make targets

```bash
make build-admin     # build React then compile Go binary → ./tmp/admin
make run-admin       # run admin binary (requires infra + api up)
make build           # updated to also build-admin
```

---

## Migration

```bash
make migrate-up      # applies 006_batch_run_logs.up.sql (and any pending)
make migrate-down    # rolls back one migration
```
