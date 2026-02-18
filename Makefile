.PHONY: build dev test test-sdk test-ui test-integration test-e2e test-smoke test-browser-full test-full test-all lint clean ui release docker help sync-openapi

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

build: ## Build the ayb binary
	go build $(LDFLAGS) -o ayb ./cmd/ayb

dev: ## Build and run with a test database URL (set DATABASE_URL)
	go run $(LDFLAGS) ./cmd/ayb start --database-url "$(DATABASE_URL)"

test: ## Run Go unit tests (no DB, fast)
	go tool gotestsum --format testdox -- -count=1 ./...

test-sdk: ## Run SDK unit tests (vitest, no browser)
	cd sdk && npm test

test-ui: ## Run UI component tests (vitest + jsdom, no browser)
	cd ui && pnpm test

test-integration: ## Run integration tests (uses AYB's managed Postgres — no Docker needed)
	go run ./internal/testutil/cmd/testpg -- go tool gotestsum --format testdox -- -tags=integration -count=1 ./...

test-smoke: build ## Run Playwright smoke tests — 8 critical paths, ~5 min (builds + starts server)
	@./ayb start > /tmp/ayb-e2e.log 2>&1 & AYB_PID=$$!; \
	trap "kill $$AYB_PID 2>/dev/null" EXIT; \
	until curl -s http://localhost:8090/health > /dev/null 2>&1; do sleep 0.5; done; \
	cd ui && npx playwright test --project=smoke; \

test-browser-full: build ## Run Playwright full browser suite, ~15 min (builds + starts server)
	@./ayb start > /tmp/ayb-e2e.log 2>&1 & AYB_PID=$$!; \
	trap "kill $$AYB_PID 2>/dev/null" EXIT; \
	until curl -s http://localhost:8090/health > /dev/null 2>&1; do sleep 0.5; done; \
	cd ui && npx playwright test --project=full; \

test-e2e: build ## Run all Playwright tests — smoke + full (builds + starts server)
	@./ayb start > /tmp/ayb-e2e.log 2>&1 & AYB_PID=$$!; \
	trap "kill $$AYB_PID 2>/dev/null" EXIT; \
	until curl -s http://localhost:8090/health > /dev/null 2>&1; do sleep 0.5; done; \
	cd ui && npx playwright test; \

test-all: test test-integration test-sdk test-ui ## Run all fast tests: Go unit + integration + SDK + UI components

test-full: test-all test-e2e ## Run every automated test: unit + integration + SDK + UI + all browser tests (~1.5 hrs)

lint: ## Run linters (requires golangci-lint)
	golangci-lint run ./...

ui: ## Build the admin dashboard SPA
	cd ui && pnpm install && pnpm build

docker: ## Build Docker image locally
	docker build -t allyourbase/ayb:latest -t allyourbase/ayb:$(VERSION) .

clean: ## Remove build artifacts
	rm -f ayb
	rm -rf dist/

release: ## Build release binaries via goreleaser (dry run)
	goreleaser release --snapshot --clean

vet: ## Run go vet
	go vet ./...

fmt: ## Check formatting
	@test -z "$$(gofmt -l .)" || (echo "Files need formatting:" && gofmt -l . && exit 1)

sync-openapi: ## Copy OpenAPI spec to docs-site public dir
	cp openapi/openapi.yaml docs-site/public/openapi.yaml
