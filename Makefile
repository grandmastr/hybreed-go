.DEFAULT_GOAL := help

# Local dev DSNs (override via env).
DATABASE_URL ?= postgres://hybreed:hybreed@localhost:5432/hybreed?sslmode=disable
REDIS_URL    ?= redis://localhost:6379/0
MIGRATIONS   := db/migrations
VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-18s\033[0m %s\n", $$1, $$2}'

## ── Build & run ──────────────────────────────────────────────────────────────
.PHONY: run
run: ## Run the API locally (needs Postgres + Redis up)
	go run ./cmd/api

.PHONY: build
build: ## Build the API binary into ./bin
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o bin/api ./cmd/api

.PHONY: seed
seed: ## Seed the database with the demo dataset
	go run ./cmd/seed

## ── Quality ──────────────────────────────────────────────────────────────────
.PHONY: test
test: ## Run tests
	go test -race ./...

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: vet
vet: ## Run go vet
	go vet ./...

.PHONY: fmt
fmt: ## Format the code
	gofmt -w cmd internal db

.PHONY: fmt-check
fmt-check: ## Fail if code is not gofmt-clean
	@test -z "$$(gofmt -l cmd internal db)" || (echo "run 'make fmt'"; gofmt -l cmd internal db; exit 1)

.PHONY: tidy
tidy: ## Tidy go.mod / go.sum
	go mod tidy

.PHONY: ci
ci: fmt-check vet lint test ## Run the full local CI gate

## ── Codegen & migrations ─────────────────────────────────────────────────────
.PHONY: sqlc
sqlc: ## Regenerate type-safe DB code from db/queries
	sqlc generate

.PHONY: sqlc-verify
sqlc-verify: ## Fail if generated code is stale
	sqlc diff

.PHONY: migrate-up
migrate-up: ## Apply all migrations
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" up

.PHONY: migrate-down
migrate-down: ## Roll back the last migration
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" down 1

.PHONY: migrate-create
migrate-create: ## Create a migration: make migrate-create name=add_widgets
	migrate create -ext sql -dir $(MIGRATIONS) -seq $(name)

## ── Docker ───────────────────────────────────────────────────────────────────
.PHONY: up
up: ## Start the full stack (api + postgres + redis)
	docker compose up --build -d

.PHONY: down
down: ## Stop the stack
	docker compose down

.PHONY: logs
logs: ## Tail the API logs
	docker compose logs -f api

.PHONY: docker-seed
docker-seed: ## Seed via a one-off container
	docker compose --profile tools run --rm seed
