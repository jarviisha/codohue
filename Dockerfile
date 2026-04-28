# Stage 1: Builder
FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

COPY go.mod go.sum ./
COPY pkg/codohuetypes/go.mod ./pkg/codohuetypes/go.mod
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/api  ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/cron ./cmd/cron

# Stage 2: API runtime
FROM scratch AS api

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/api /api

EXPOSE 2001

ENTRYPOINT ["/api"]

# Stage 3: Cron runtime
FROM scratch AS cron

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/cron /cron

ENTRYPOINT ["/cron"]

# Stage 4: Migrate
FROM alpine:3.21 AS migrate

RUN apk add --no-cache ca-certificates curl postgresql-client && \
    curl -fsSL https://github.com/golang-migrate/migrate/releases/download/v4.18.1/migrate.linux-amd64.tar.gz \
    | tar xz -C /usr/local/bin migrate && \
    apk del curl

COPY migrations /migrations
COPY docker/migrate-entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
