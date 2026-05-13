BIN_DIR := ./tmp

API_BIN      := $(BIN_DIR)/api
CRON_BIN     := $(BIN_DIR)/cron
ADMIN_BIN    := $(BIN_DIR)/admin
EMBEDDER_BIN := $(BIN_DIR)/embedder

MIGRATIONS_DIR := ./migrations
COVERAGE_DIR   := $(BIN_DIR)/coverage

COVERAGE_UNIT_OUT := $(COVERAGE_DIR)/unit.out
COVERAGE_RACE_OUT := $(COVERAGE_DIR)/race.out

# Go modules covered by workspace-wide lint, format, and test targets.
GO_MODULES := . ./pkg/codohuetypes ./sdk/go ./sdk/go/redistream

GO_CACHE_ENV := env GOCACHE=/tmp/go-build GOTMPDIR=/tmp
LINT_ENV     := $(GO_CACHE_ENV) GOLANGCI_LINT_CACHE=/tmp/golangci-lint GOPROXY=off

# DOCKER_BUILDKIT / COMPOSE_DOCKER_CLI_BUILD are no-ops on modern Docker (23+,
# compose v2) where BuildKit is already the default. Setting them keeps older
# host installs working with the Dockerfile's `# syntax=` directive and
# `--mount=type=cache` instructions.
COMPOSE          := DOCKER_BUILDKIT=1 COMPOSE_DOCKER_CLI_BUILD=1 docker compose
COMPOSE_APP      := $(COMPOSE) -f docker-compose.app.yml
COMPOSE_PROD     := $(COMPOSE) -f docker-compose.prod.yml
COMPOSE_PROD_ENV := CODOHUE_DATABASE_URL=postgres://example CODOHUE_RECOMMENDER_API_KEY=dummy

MIN_TOTAL          ?= 60
MIN_RECOMMEND      ?= 85
MIN_COMPUTE        ?= 75
MIN_CONFIG         ?= 65
MIN_AUTH           ?= 95
MIN_INGEST         ?= 80
MIN_NSCONFIG       ?= 80
MIN_IDMAP          ?= 90
MIN_INFRA_REDIS    ?= 95
MIN_INFRA_QDRANT   ?= 75
MIN_INFRA_POSTGRES ?= 85
MIN_CMD_API        ?= 40
MIN_CMD_CRON       ?= 45
# Feature 004 — catalog auto-embedding domains.
MIN_CATALOG        ?= 85
MIN_EMBEDDER       ?= 70
MIN_EMBEDSTRATEGY  ?= 90

.PHONY: \
	build build-api build-cron build-admin build-embedder \
	run run-cron run-admin run-embedder dev dev-admin dev-embedder dev-all \
	up up-all up-build up-d up-build-d \
	up-infra up-infra-build up-infra-d up-infra-build-d \
	up-app up-app-build up-app-d up-app-build-d down down-v down-app \
	logs logs-api logs-cron logs-admin logs-embedder logs-app \
	compose-check compose-check-app compose-check-prod \
	lint fmt \
	test test-pkg test-verbose test-race \
	coverage coverage-unit coverage-race coverage-report coverage-html \
	coverage-check coverage-check-pkg coverage-check-all coverage-clean \
	test-e2e test-e2e-api test-e2e-heavy \
	migrate migrate-up migrate-down migrate-version migrate-create \
	clean

# Build

build: build-api build-cron build-admin build-embedder

build-api:
	go build -o $(API_BIN) ./cmd/api

build-cron:
	go build -o $(CRON_BIN) ./cmd/cron

build-admin:
	go build -o $(ADMIN_BIN) ./cmd/admin

build-embedder:
	go build -o $(EMBEDDER_BIN) ./cmd/embedder

# Run and development

run:
	go run ./cmd/api

run-cron:
	go run ./cmd/cron

run-admin:
	go run ./cmd/admin

run-embedder:
	go run ./cmd/embedder

dev:
	air

dev-admin:
	cd web/admin && npm run dev

dev-embedder:
	go run ./cmd/embedder

dev-all:
	@go build -o $(ADMIN_BIN) ./cmd/admin
	@trap 'kill 0' SIGINT SIGTERM; \
	air & \
	$(ADMIN_BIN) & \
	(cd web/admin && npm run dev) & \
	wait

# Docker

up up-all:
	$(COMPOSE) up

up-build:
	$(COMPOSE) up --build

up-d:
	$(COMPOSE) up -d

up-build-d:
	$(COMPOSE) up -d --build

up-infra up-infra-d:
	$(COMPOSE) up -d postgres redis qdrant

up-infra-build up-infra-build-d:
	$(COMPOSE) up -d --build postgres redis qdrant

up-app:
	$(COMPOSE_APP) up

up-app-build:
	$(COMPOSE_APP) up --build

up-app-d:
	$(COMPOSE_APP) up -d

up-app-build-d:
	$(COMPOSE_APP) up -d --build

down:
	$(COMPOSE) down

down-v:
	$(COMPOSE) down -v

down-app:
	$(COMPOSE_APP) down

logs logs-api:
	$(COMPOSE) logs -f api

logs-cron:
	$(COMPOSE) logs -f cron

logs-admin:
	$(COMPOSE) logs -f admin

logs-embedder:
	$(COMPOSE) logs -f embedder

logs-app:
	$(COMPOSE_APP) logs -f

compose-check: compose-check-app compose-check-prod
	$(COMPOSE) config --quiet

compose-check-app:
	$(COMPOSE_APP) config --quiet

compose-check-prod:
	$(COMPOSE_PROD_ENV) $(COMPOSE_PROD) config --quiet

# Lint and format

lint:
	@for m in $(GO_MODULES); do \
		echo "==> lint $$m"; \
		(cd $$m && $(LINT_ENV) golangci-lint run ./...) || exit 1; \
	done

fmt:
	@for m in $(GO_MODULES); do \
		echo "==> fmt $$m"; \
		(cd $$m && $(LINT_ENV) golangci-lint fmt ./...) || exit 1; \
	done

# Tests

test:
	@for m in $(GO_MODULES); do \
		echo "==> test $$m"; \
		(cd $$m && go test ./...) || exit 1; \
	done

test-pkg:
	go test $(PKG)

test-verbose:
	@for m in $(GO_MODULES); do \
		echo "==> test -v $$m"; \
		(cd $$m && go test -v ./...) || exit 1; \
	done

test-race:
	@for m in $(GO_MODULES); do \
		echo "==> test -race $$m"; \
		(cd $$m && $(GO_CACHE_ENV) go test -race ./...) || exit 1; \
	done

# Coverage

coverage: coverage-unit

coverage-unit:
	mkdir -p $(COVERAGE_DIR)
	$(GO_CACHE_ENV) go test -count=1 ./... -coverpkg=./... -coverprofile=$(COVERAGE_UNIT_OUT)
	$(GO_CACHE_ENV) go tool cover -func=$(COVERAGE_UNIT_OUT) | tail -n 1

coverage-race:
	mkdir -p $(COVERAGE_DIR)
	$(GO_CACHE_ENV) go test -count=1 -race ./... -coverpkg=./... -coverprofile=$(COVERAGE_RACE_OUT)
	$(GO_CACHE_ENV) go tool cover -func=$(COVERAGE_RACE_OUT) | tail -n 1

coverage-report:
	@if [ ! -f "$(or $(OUT),$(COVERAGE_UNIT_OUT))" ]; then \
		echo "coverage profile not found: $(or $(OUT),$(COVERAGE_UNIT_OUT))"; \
		echo "run 'make coverage-unit' first or pass OUT=/path/to/profile.out"; \
		exit 1; \
	fi
	$(GO_CACHE_ENV) go tool cover -func=$(or $(OUT),$(COVERAGE_UNIT_OUT))

coverage-html:
	@if [ ! -f "$(or $(OUT),$(COVERAGE_UNIT_OUT))" ]; then \
		echo "coverage profile not found: $(or $(OUT),$(COVERAGE_UNIT_OUT))"; \
		echo "run 'make coverage-unit' first or pass OUT=/path/to/profile.out"; \
		exit 1; \
	fi
	$(GO_CACHE_ENV) go tool cover -html=$(or $(OUT),$(COVERAGE_UNIT_OUT))

coverage-check:
	@if [ ! -f "$(or $(OUT),$(COVERAGE_UNIT_OUT))" ]; then \
		echo "coverage profile not found: $(or $(OUT),$(COVERAGE_UNIT_OUT))"; \
		echo "run 'make coverage-unit' first or pass OUT=/path/to/profile.out"; \
		exit 1; \
	fi
	@actual=$$($(GO_CACHE_ENV) go tool cover -func=$(or $(OUT),$(COVERAGE_UNIT_OUT)) | awk '/^total:/ {gsub("%","",$$3); print $$3}'); \
	min=$${MIN_TOTAL:-60}; \
	awk 'BEGIN { exit !('"$$actual"' >= '"$$min"') }' || { \
		echo "coverage check failed: total=$$actual% min=$$min%"; \
		exit 1; \
	}; \
	echo "coverage check passed: total=$$actual% min=$$min%"

coverage-check-pkg:
	@if [ -z "$(PKG)" ]; then \
		echo "PKG is required, for example: make coverage-check-pkg PKG=./internal/recommend/... MIN=70"; \
		exit 1; \
	fi
	@mkdir -p $(COVERAGE_DIR)
	@pkg_slug=$$(printf '%s' "$(PKG)" | tr '/.' '__'); \
	profile="$(COVERAGE_DIR)/pkg-check-$${pkg_slug}.out"; \
	$(GO_CACHE_ENV) go test -count=1 $(PKG) -coverprofile=$$profile >/dev/null; \
	actual=$$($(GO_CACHE_ENV) go tool cover -func=$$profile | awk '/^total:/ {gsub("%","",$$3); print $$3}'); \
	min=$${MIN:-70}; \
	awk 'BEGIN { exit !('"$$actual"' >= '"$$min"') }' || { \
		echo "coverage check failed: pkg=$(PKG) total=$$actual% min=$$min%"; \
		exit 1; \
	}; \
	echo "coverage check passed: pkg=$(PKG) total=$$actual% min=$$min%"

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
	$(MAKE) coverage-check-pkg PKG=./internal/catalog/... MIN=$(MIN_CATALOG)
	$(MAKE) coverage-check-pkg PKG=./internal/embedder/... MIN=$(MIN_EMBEDDER)
	$(MAKE) coverage-check-pkg PKG=./internal/core/embedstrategy/... MIN=$(MIN_EMBEDSTRATEGY)

coverage-clean:
	rm -rf $(COVERAGE_DIR)

# End-to-end tests

test-e2e: build
	go test -v -tags=e2e -timeout=120s ./e2e/...

test-e2e-api: build-api
	go test -v -tags=e2e -timeout=120s ./e2e/... -run 'Ping|Healthz|Config|Embedding|Recommend|Rank|Trending'

test-e2e-heavy: build
	go test -v -tags=e2e -timeout=180s ./e2e/... -run 'Ingest|Cron|RecommendComputed|RankComputed|Hybrid|Catalog'

# Migrations

migrate: migrate-up

migrate-up:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" up

migrate-down:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" down 1

migrate-version:
	@set -a; [ -f .env ] && . ./.env; set +a; \
	migrate -path $(MIGRATIONS_DIR) -database "$$DATABASE_URL" version

migrate-create:
	migrate -path $(MIGRATIONS_DIR) -ext sql -seq create $(NAME)

# Clean

clean:
	rm -rf $(BIN_DIR)
