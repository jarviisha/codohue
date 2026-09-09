# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS go-source
WORKDIR /build
# The server only needs the root module and its local wire-types replacement.
ENV GOWORK=off CGO_ENABLED=0
COPY go.mod go.sum ./
COPY pkg/codohuetypes/go.mod ./pkg/codohuetypes/
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/codohuetypes/ ./pkg/codohuetypes/
COPY web/admin/embed.go web/admin/embed_prod.go ./web/admin/

FROM go-source AS bin-builder
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -ldflags="-s -w" -o /out/ ./cmd/api ./cmd/cron ./cmd/embedder

FROM node:22-alpine AS frontend
WORKDIR /web/admin
COPY web/admin/package.json web/admin/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm npm ci --no-audit --no-progress
COPY web/admin/ ./
RUN npm run build

FROM go-source AS admin-builder
COPY --from=frontend /web/admin/dist ./web/admin/dist
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    go build -tags=embedui -ldflags="-s -w" -o /out/admin ./cmd/admin

FROM alpine:3.22 AS runtime
RUN apk add --no-cache ca-certificates
USER 65532:65532

FROM runtime AS api
COPY --from=bin-builder /out/api /api
EXPOSE 2001
ENTRYPOINT ["/api"]

FROM runtime AS cron
COPY --from=bin-builder /out/cron /cron
ENTRYPOINT ["/cron"]

FROM runtime AS embedder
COPY --from=bin-builder /out/embedder /embedder
EXPOSE 2003
ENTRYPOINT ["/embedder"]

FROM runtime AS admin
COPY --from=admin-builder /out/admin /admin
EXPOSE 2002
ENTRYPOINT ["/admin"]
