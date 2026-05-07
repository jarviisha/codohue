# Stage 1: Go builder — api + cron binaries
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
FROM alpine:3.21 AS api

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/api /api

EXPOSE 2001

ENTRYPOINT ["/api"]

# Stage 3: Cron runtime
FROM alpine:3.21 AS cron

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /out/cron /cron

ENTRYPOINT ["/cron"]

# Stage 4: Frontend builder — React SPA bundle (admin only)
FROM node:20-alpine AS frontend

WORKDIR /web/admin

COPY web/admin/package.json web/admin/package-lock.json ./
RUN npm ci --no-audit --no-progress

COPY web/admin/ ./
RUN npm run build

# Stage 5: Go builder — admin binary (embeds the SPA bundle)
FROM builder AS admin-builder

COPY --from=frontend /web/admin/dist ./web/admin/dist

RUN CGO_ENABLED=0 GOOS=linux go build -tags=embedui -ldflags="-s -w" -o /out/admin ./cmd/admin

# Stage 6: Admin runtime
FROM alpine:3.21 AS admin

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=admin-builder /out/admin /admin

EXPOSE 2002

ENTRYPOINT ["/admin"]
