# syntax=docker/dockerfile:1.6
#
# Layout intent: every long-running step (go mod download, go build, npm ci)
# is wrapped in BuildKit cache mounts so repeated builds reuse compiler
# artefacts and downloaded packages across CI runs and local rebuilds.
# Source is also copied selectively — touching docs / migrations / web/admin
# src does not invalidate the Go build cache layers.

# ─── Stage: workspace module manifests ───────────────────────────────────────
# Isolated so a source-only change skips `go mod download` entirely. Every
# go.work member must be present here or `go mod download` complains.
FROM golang:1.26-alpine AS go-deps

RUN apk add --no-cache git ca-certificates

WORKDIR /build

ENV GOCACHE=/root/.cache/go-build \
    GOMODCACHE=/go/pkg/mod

COPY go.work go.work.sum ./
COPY go.mod go.sum ./
COPY pkg/codohuetypes/go.mod ./pkg/codohuetypes/
COPY sdk/go/go.mod sdk/go/go.sum ./sdk/go/
COPY sdk/go/redistream/go.mod sdk/go/redistream/go.sum ./sdk/go/redistream/

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go mod download

# ─── Stage: builder for api / cron / embedder ────────────────────────────────
# Selective COPY — bringing in only what the Go compiler needs. The admin
# binary is built in a separate parallel branch so embedding the SPA does
# not block these three.
FROM go-deps AS bin-builder

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/codohuetypes/ ./pkg/codohuetypes/
COPY sdk/ ./sdk/
COPY web/admin/embed.go web/admin/embed_prod.go ./web/admin/

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api      ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/cron     ./cmd/cron && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/embedder ./cmd/embedder

# ─── Stage: API runtime ──────────────────────────────────────────────────────
FROM alpine:3.21 AS api

COPY --from=bin-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=bin-builder /out/api /api

EXPOSE 2001

ENTRYPOINT ["/api"]

# ─── Stage: Cron runtime ─────────────────────────────────────────────────────
FROM alpine:3.21 AS cron

COPY --from=bin-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=bin-builder /out/cron /cron

ENTRYPOINT ["/cron"]

# ─── Stage: Embedder runtime ─────────────────────────────────────────────────
FROM alpine:3.21 AS embedder

COPY --from=bin-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=bin-builder /out/embedder /embedder

EXPOSE 2003

ENTRYPOINT ["/embedder"]

# ─── Stage: Frontend builder ─────────────────────────────────────────────────
# package-lock first so `npm ci` lives in its own cacheable layer; the npm
# download cache itself is mounted across builds.
FROM node:20-alpine AS frontend

WORKDIR /web/admin

COPY web/admin/package.json web/admin/package-lock.json ./

RUN --mount=type=cache,target=/root/.npm \
    npm ci --no-audit --no-progress

COPY web/admin/ ./

RUN npm run build

# ─── Stage: Admin binary (embeds SPA via -tags=embedui) ──────────────────────
# Branches from go-deps (not bin-builder) so the api/cron/embedder build can
# run in parallel with the admin build under BuildKit.
FROM go-deps AS admin-builder

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/codohuetypes/ ./pkg/codohuetypes/
COPY sdk/ ./sdk/
COPY web/admin/embed.go web/admin/embed_prod.go ./web/admin/
COPY --from=frontend /web/admin/dist ./web/admin/dist

RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -tags=embedui -ldflags="-s -w" \
        -o /out/admin ./cmd/admin

# ─── Stage: Admin runtime ────────────────────────────────────────────────────
FROM alpine:3.21 AS admin

COPY --from=admin-builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=admin-builder /out/admin /admin

EXPOSE 2002

ENTRYPOINT ["/admin"]
