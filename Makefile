.PHONY: help build run test test-race test-integration vet tidy docker-up docker-down clean

# Local defaults; override on the command line, e.g. `make run JWT_SECRET=...`.
DATABASE_URL ?= postgres://orderflow:orderflow@localhost:5432/orderflow?sslmode=disable
TEST_DATABASE_URL ?= $(DATABASE_URL)
JWT_SECRET ?= dev-secret-change-me-0123456789

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

build: ## Build the API binary into ./bin
	go build -o bin/api ./cmd/api

run: ## Run the API locally (needs Postgres; see docker-up)
	DATABASE_URL="$(DATABASE_URL)" JWT_SECRET="$(JWT_SECRET)" go run ./cmd/api

test: ## Run unit tests
	go test ./...

test-race: ## Run unit tests with the race detector
	go test -race ./...

test-integration: ## Run integration tests (needs Postgres)
	TEST_DATABASE_URL="$(TEST_DATABASE_URL)" go test -tags=integration -race ./internal/repository/...

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy go.mod and go.sum
	go mod tidy

docker-up: ## Start the full stack (api + postgres) with Docker Compose
	docker compose up --build

docker-down: ## Stop the stack and remove volumes
	docker compose down -v

clean: ## Remove build artifacts
	rm -rf bin
