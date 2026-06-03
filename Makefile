# Govatars — local workflows (Go binaries run on the host; Compose runs dependencies only).

GO ?= go
BIN_DIR := bin
MIGRATIONS_DIR := ./migrations
DB_URL ?= postgres://govatars:govatars@localhost:5433/govatars?sslmode=disable

# golang-migrate registers DB drivers via build tags (e.g. `postgres` → lib/pq). `go tool migrate`
# does not rebuild the CLI with those tags, so Postgres is "unknown driver". Pin the version here
# and use `go run -tags postgres …` (same as `go install -tags postgres …` to GOPATH/bin).
MIGRATE_VER ?= v4.19.0
MIGRATE := $(GO) run -tags postgres github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VER)

# v2.x is required for recent go.mod versions (v1.x golangci-lint refuses a newer toolchain than it was built with).
GOLANGCI_LINT_VER ?= v2.11.4
GOPATH_BIN := $(shell $(GO) env GOPATH)/bin
GOLANGCI_LINT := $(GOPATH_BIN)/golangci-lint

.PHONY: all build dbuild run-server run-worker deps-up deps-down migrate-up migrate-down install-lint lint test test-integration test-coverage test-coverage-integration test-coverage-full generate tidy clean

all: build

build:
	mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/server ./cmd/server
	$(GO) build -o $(BIN_DIR)/worker ./cmd/worker

run-server:
	$(GO) run ./cmd/server

run-worker:
	$(GO) run ./cmd/worker

dbuild:
	docker compose build

up:
	docker compose --env-file .env.local up

down:
	docker compose down

migrate-up:
	$(MIGRATE) -database "$(DB_URL)" -path $(MIGRATIONS_DIR) up

migrate-down:
	$(MIGRATE) -database "$(DB_URL)" -path $(MIGRATIONS_DIR) down

install-lint:
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VER)

lint:
	$(GOLANGCI_LINT) run ./...

test:
	$(GO) test -race ./...

test-integration:
	$(GO) test -tags=integration -p 1 -race -timeout=20m ./...

test-coverage:
	$(GO) test -race $$($(GO) list ./... | grep -v '/internal/mocks$$') -coverprofile=coverage-unit.out -covermode=atomic
	$(GO) tool cover -func=coverage-unit.out

test-coverage-integration:
	$(GO) test -tags=integration -p 1 -race -count=1 -timeout=25m \
		$$($(GO) list ./... | grep -v '/internal/mocks$$') \
		-coverprofile=coverage-integration.out -covermode=atomic
	$(GO) tool cover -func=coverage-integration.out

generate:
	$(GO) generate ./...

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR)

.PHONY: build-worker-images
build-worker-images:
	docker build -f Dockerfile.worker -t govatars-worker:latest .

.PHONY: build-server-images
build-server-images:
	docker build -f Dockerfile.server -t govatars-server:latest .

.PHONY: build-images
build-images:
	make build-worker-images
	make build-server-images

.PHONY: deploy
deploy:
	helmfile apply --file deploy/helmfile.yaml