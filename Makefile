BIN_DIR   := ./tmp
API_BIN   := $(BIN_DIR)/api
CRON_BIN  := $(BIN_DIR)/cron

MIGRATIONS_DIR := ./migrations
COVERAGE_DIR := $(BIN_DIR)/coverage
COVERAGE_UNIT_OUT := $(COVERAGE_DIR)/unit.out
COVERAGE_RACE_OUT := $(COVERAGE_DIR)/race.out

MIN_TOTAL ?= 80
MIN_RECOMMEND ?= 85
MIN_COMPUTE ?= 85
MIN_CONFIG ?= 95
MIN_AUTH ?= 95
MIN_INGEST ?= 80
MIN_NSCONFIG ?= 80
MIN_IDMAP ?= 90
MIN_INFRA_REDIS ?= 95
MIN_INFRA_QDRANT ?= 75
MIN_INFRA_POSTGRES ?= 85
MIN_CMD_API ?= 40
MIN_CMD_CRON ?= 45

.PHONY: build build-api build-cron run run-cron dev lint fmt \
        test test-pkg test-verbose test-race test-e2e test-e2e-api test-e2e-heavy \
        coverage coverage-unit coverage-race coverage-report coverage-html coverage-check coverage-check-pkg coverage-check-all coverage-clean \
        up up-d up-infra down down-v logs logs-cron \
        migrate migrate-up migrate-down migrate-version migrate-create \
        clean

# ── Build ──────────────────────────────────────────────────────────────────────

build: build-api build-cron

build-api:
	go build -o $(API_BIN) ./cmd/api

build-cron:
	go build -o $(CRON_BIN) ./cmd/cron

# ── Run ────────────────────────────────────────────────────────────────────────

## Run the API server directly (requires infra to be up)
run:
	go run ./cmd/api

## Run one cron cycle manually (requires infra to be up)
run-cron:
	go run ./cmd/cron

## Run the API with live reload via air
dev:
	air

# ── Docker ─────────────────────────────────────────────────────────────────────

## Start the full stack (api + postgres + redis + qdrant)
up:
	docker compose up

up-d:
	docker compose up -d

## Start infra only (postgres + redis + qdrant), without the API
up-infra:
	docker compose up -d postgres redis qdrant

down:
	docker compose down

## Stop the stack and remove volumes (full local data reset)
down-v:
	docker compose down -v

logs:
	docker compose logs -f api

logs-cron:
	docker compose logs -f cron

# ── Lint & Format ──────────────────────────────────────────────────────────────

lint:
	env GOCACHE=/tmp/go-build GOTMPDIR=/tmp GOLANGCI_LINT_CACHE=/tmp/golangci-lint GOPROXY=off golangci-lint run ./...

fmt:
	env GOCACHE=/tmp/go-build GOTMPDIR=/tmp GOLANGCI_LINT_CACHE=/tmp/golangci-lint GOPROXY=off golangci-lint fmt ./...

# ── Test ───────────────────────────────────────────────────────────────────────

test:
	go test ./...

## Run tests for a specific package, for example:
##   make test-pkg PKG=./internal/ingest/...
test-pkg:
	go test $(PKG)

test-verbose:
	go test -v ./...

test-race:
	env GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test -race ./...

coverage: coverage-unit

coverage-unit:
	mkdir -p $(COVERAGE_DIR)
	env GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test -count=1 ./... -coverpkg=./... -coverprofile=$(COVERAGE_UNIT_OUT)
	env GOCACHE=/tmp/go-build GOTMPDIR=/tmp go tool cover -func=$(COVERAGE_UNIT_OUT) | tail -n 1

coverage-race:
	mkdir -p $(COVERAGE_DIR)
	env GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test -count=1 -race ./... -coverpkg=./... -coverprofile=$(COVERAGE_RACE_OUT)
	env GOCACHE=/tmp/go-build GOTMPDIR=/tmp go tool cover -func=$(COVERAGE_RACE_OUT) | tail -n 1

## Print per-function coverage. Default input: tmp/coverage/unit.out
coverage-report:
	@if [ ! -f "$(or $(OUT),$(COVERAGE_UNIT_OUT))" ]; then \
		echo "coverage profile not found: $(or $(OUT),$(COVERAGE_UNIT_OUT))"; \
		echo "run 'make coverage-unit' first or pass OUT=/path/to/profile.out"; \
		exit 1; \
	fi
	env GOCACHE=/tmp/go-build GOTMPDIR=/tmp go tool cover -func=$(or $(OUT),$(COVERAGE_UNIT_OUT))

## Open an HTML coverage report. Default input: tmp/coverage/unit.out
coverage-html:
	@if [ ! -f "$(or $(OUT),$(COVERAGE_UNIT_OUT))" ]; then \
		echo "coverage profile not found: $(or $(OUT),$(COVERAGE_UNIT_OUT))"; \
		echo "run 'make coverage-unit' first or pass OUT=/path/to/profile.out"; \
		exit 1; \
	fi
	env GOCACHE=/tmp/go-build GOTMPDIR=/tmp go tool cover -html=$(or $(OUT),$(COVERAGE_UNIT_OUT))

## Check total coverage against MIN_TOTAL. Default profile: tmp/coverage/unit.out
coverage-check:
	@if [ ! -f "$(or $(OUT),$(COVERAGE_UNIT_OUT))" ]; then \
		echo "coverage profile not found: $(or $(OUT),$(COVERAGE_UNIT_OUT))"; \
		echo "run 'make coverage-unit' first or pass OUT=/path/to/profile.out"; \
		exit 1; \
	fi
	@actual=$$(env GOCACHE=/tmp/go-build GOTMPDIR=/tmp go tool cover -func=$(or $(OUT),$(COVERAGE_UNIT_OUT)) | awk '/^total:/ {gsub("%","",$$3); print $$3}'); \
	min=$${MIN_TOTAL:-60}; \
	awk 'BEGIN { exit !('"$$actual"' >= '"$$min"') }' || { \
		echo "coverage check failed: total=$$actual% min=$$min%"; \
		exit 1; \
	}; \
	echo "coverage check passed: total=$$actual% min=$$min%"

## Check a single package against MIN by running package-local coverage.
## Example: make coverage-check-pkg PKG=./internal/recommend/... MIN=70
coverage-check-pkg:
	@if [ -z "$(PKG)" ]; then \
		echo "PKG is required, for example: make coverage-check-pkg PKG=./internal/recommend/... MIN=70"; \
		exit 1; \
	fi
	@mkdir -p $(COVERAGE_DIR)
	@pkg_slug=$$(printf '%s' "$(PKG)" | tr '/.' '__'); \
	profile="$(COVERAGE_DIR)/pkg-check-$${pkg_slug}.out"; \
	env GOCACHE=/tmp/go-build GOTMPDIR=/tmp go test -count=1 $(PKG) -coverprofile=$$profile >/dev/null; \
	actual=$$(env GOCACHE=/tmp/go-build GOTMPDIR=/tmp go tool cover -func=$$profile | awk '/^total:/ {gsub("%","",$$3); print $$3}'); \
	min=$${MIN:-70}; \
	awk 'BEGIN { exit !('"$$actual"' >= '"$$min"') }' || { \
		echo "coverage check failed: pkg=$(PKG) total=$$actual% min=$$min%"; \
		exit 1; \
	}; \
	echo "coverage check passed: pkg=$(PKG) total=$$actual% min=$$min%"

## Check repository total and critical packages against default thresholds.
coverage-check-all: coverage-unit
	$(MAKE) coverage-check MIN_TOTAL=$(MIN_TOTAL)
	$(MAKE) coverage-check-pkg PKG=./internal/recommend/... MIN=$(MIN_RECOMMEND)
	$(MAKE) coverage-check-pkg PKG=./internal/compute/... MIN=$(MIN_COMPUTE)
	$(MAKE) coverage-check-pkg PKG=./internal/config/... MIN=$(MIN_CONFIG)
	$(MAKE) coverage-check-pkg PKG=./internal/auth/... MIN=$(MIN_AUTH)
	$(MAKE) coverage-check-pkg PKG=./internal/ingest/... MIN=$(MIN_INGEST)
	$(MAKE) coverage-check-pkg PKG=./internal/nsconfig/... MIN=$(MIN_NSCONFIG)
	$(MAKE) coverage-check-pkg PKG=./internal/core/idmap/... MIN=$(MIN_IDMAP)
	$(MAKE) coverage-check-pkg PKG=./internal/infra/redis/... MIN=$(MIN_INFRA_REDIS)
	$(MAKE) coverage-check-pkg PKG=./internal/infra/qdrant/... MIN=$(MIN_INFRA_QDRANT)
	$(MAKE) coverage-check-pkg PKG=./internal/infra/postgres/... MIN=$(MIN_INFRA_POSTGRES)
	$(MAKE) coverage-check-pkg PKG=./cmd/api/... MIN=$(MIN_CMD_API)
	$(MAKE) coverage-check-pkg PKG=./cmd/cron/... MIN=$(MIN_CMD_CRON)

coverage-clean:
	rm -rf $(COVERAGE_DIR)

## E2E tests require infra to be running (make up-infra) and migrations applied (make migrate-up).
## Full sequence:
##   make up-infra && make migrate-up && make test-e2e
## This target builds both binaries because the heavy suites execute `tmp/cron`.
test-e2e: build
	go test -v -tags=e2e -timeout=120s ./e2e/...

## API-only E2E coverage for health, config, embedding, recommendation, ranking, and trending.
test-e2e-api: build-api
	go test -v -tags=e2e -timeout=120s ./e2e/... -run 'Ping|Healthz|Config|Embedding|Recommend|Rank|Trending'

## Heavier E2E coverage that exercises Redis Streams ingest, cron recompute, and hybrid/computed artifacts.
test-e2e-heavy: build
	go test -v -tags=e2e -timeout=180s ./e2e/... -run 'Ingest|Cron|RecommendComputed|RankComputed|Hybrid'

# ── Migrations ─────────────────────────────────────────────────────────────────

## Apply all pending migrations
migrate-up:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" up

## Roll back one migration step
migrate-down:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" down 1

## Show the current migration version
migrate-version:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" version

## Create a new migration file, for example:
##   make migrate-create NAME=add_indexes
migrate-create:
	migrate -path $(MIGRATIONS_DIR) -ext sql -seq create $(NAME)

migrate: migrate-up

# ── Clean ──────────────────────────────────────────────────────────────────────

clean:
	rm -rf $(BIN_DIR)
