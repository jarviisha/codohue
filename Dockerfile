# Stage 1: Frontend builder — React SPA bundle (admin only)
FROM node:20-alpine AS frontend

WORKDIR /web/admin

COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci --no-audit --no-progress

COPY web/admin/ ./
RUN npm run build

# Stage 2: Go builder — api + cron binaries
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
COPY pkg/codohuetypes/go.mod ./pkg/codohuetypes/go.mod
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api  ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/cron ./cmd/cron

# Stage 3: Go builder — admin binary (embeds the SPA bundle)
FROM builder AS admin-builder

COPY --from=frontend /web/admin/dist ./web/admin/dist

RUN CGO_ENABLED=0 GOOS=linux go build -tags=embedui -ldflags="-s -w" -o /out/admin ./cmd/admin

# Stage 4: API runtime
FROM alpine:3.21 AS api

RUN apk add --no-cache ca-certificates wget
COPY --from=builder /out/api /api

EXPOSE 2001

ENTRYPOINT ["/api"]

# Stage 5: Cron runtime
FROM alpine:3.21 AS cron

RUN apk add --no-cache ca-certificates
COPY --from=builder /out/cron /cron

ENTRYPOINT ["/cron"]

# Stage 6: Admin runtime
FROM alpine:3.21 AS admin

RUN apk add --no-cache ca-certificates wget
COPY --from=admin-builder /out/admin /admin

EXPOSE 2002

ENTRYPOINT ["/admin"]

# Stage 7: Migrate
FROM alpine:3.21 AS migrate

RUN apk add --no-cache ca-certificates curl postgresql-client && \
    curl -fsSL https://github.com/golang-migrate/migrate/releases/download/v4.18.1/migrate.linux-amd64.tar.gz \
    | tar xz -C /usr/local/bin migrate && \
    apk del curl

COPY migrations /migrations
COPY docker/migrate-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
