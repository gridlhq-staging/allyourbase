.PHONY: build dev test test-sdk test-sdk-go test-sdk-python test-sdk-dart test-sdk-swift test-sdk-kotlin test-sdk-react test-sdk-ssr test-sdk-all test-sdk-integration test-sdk-python-integration test-ui test-demos-unit test-quickstart-contract test-integration test-multinode test-cell test-demo-smoke test-demo-instantsearch test-demo-e2e test-demo-launch test-demo-cross-smoke test-demo-movies-real-provider test-push-smoke test-e2e test-smoke test-browser-full test-full test-all test-everything test-api-smoke test-api-journey lint govulncheck check demo-check hygiene check-hygiene check-sizes check-screen-specs check-demo-specs check-followups check-ui-bundle-size check-ui-lint check-browser-tests-lint check-func-sizes check-closeout-ancestry check-installer check-sdk-build launch-check adoption release-candidate-check clean ui demos release docker docker-runtime-smoke help sync-openapi build-postgres load-admin-status load-admin-status-local load-auth-request-path load-auth-request-path-local load-data-path load-data-path-local load-data-pool-pressure load-data-pool-pressure-local load-http-100 load-http-100-local load-http-500 load-http-500-local load-http-1000 load-http-1000-local load-realtime-ws load-realtime-ws-local load-realtime-ws-1000 load-realtime-ws-1000-local load-realtime-ws-5000 load-realtime-ws-5000-local load-realtime-ws-10000 load-realtime-ws-10000-local load-sustained-soak load-sustained-soak-local

# Build variables
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS  = -ldflags "-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)"
GOVULNCHECK_VERSION ?= v1.6.0

LOAD_K6_BIN ?= k6
LOAD_DEFAULT_VUS ?= 1
LOAD_DEFAULT_ITERATIONS ?= 1
LOAD_DEFAULT_BASE_URL ?= http://127.0.0.1:8090
LOAD_ADMIN_STATUS_SCENARIO := tests/load/scenarios/admin_status.js
LOAD_AUTH_REQUEST_PATH_SCENARIO := tests/load/scenarios/auth_register_login_refresh.js
LOAD_DATA_PATH_SCENARIO := tests/load/scenarios/data_path_crud_batch.js
LOAD_DATA_POOL_PRESSURE_SCENARIO := tests/load/scenarios/data_pool_pressure.js
LOAD_REALTIME_WS_SCENARIO := tests/load/scenarios/realtime_ws_subscribe.js
LOAD_SUSTAINED_SOAK_SCENARIO := tests/load/scenarios/sustained_soak.js
LOAD_AUTH_ENABLED_DEFAULT := true
LOAD_AUTH_RATE_LIMIT_DEFAULT := 10000
LOAD_API_RATE_LIMIT_DEFAULT := 10000/min
LOAD_API_ANON_RATE_LIMIT_DEFAULT := 10000/min
BROWSER_AUTH_ENABLED_DEFAULT := true
BROWSER_LOCAL_BASE_URL := http://localhost:8090
BROWSER_LOCAL_AYB_START_COMMAND := ./ayb start --foreground --host 127.0.0.1
LOAD_ADMIN_STATUS_K6_COMMAND := $(LOAD_K6_BIN) run --vus $${K6_VUS:-$(LOAD_DEFAULT_VUS)} --iterations $${K6_ITERATIONS:-$(LOAD_DEFAULT_ITERATIONS)} $(LOAD_ADMIN_STATUS_SCENARIO)
LOAD_AUTH_REQUEST_PATH_K6_COMMAND := $(LOAD_K6_BIN) run --vus $${K6_VUS:-$(LOAD_DEFAULT_VUS)} --iterations $${K6_ITERATIONS:-$(LOAD_DEFAULT_ITERATIONS)} $(LOAD_AUTH_REQUEST_PATH_SCENARIO)
LOAD_DATA_PATH_K6_COMMAND := $(LOAD_K6_BIN) run --vus $${K6_VUS:-$(LOAD_DEFAULT_VUS)} --iterations $${K6_ITERATIONS:-$(LOAD_DEFAULT_ITERATIONS)} $(LOAD_DATA_PATH_SCENARIO)
LOAD_DATA_POOL_PRESSURE_K6_COMMAND := AYB_POOL_PRESSURE_VUS=$${K6_VUS:-$(LOAD_DEFAULT_VUS)} AYB_POOL_PRESSURE_ITERATIONS=$${K6_ITERATIONS:-$(LOAD_DEFAULT_ITERATIONS)} env -u K6_VUS -u K6_ITERATIONS $(LOAD_K6_BIN) run $(LOAD_DATA_POOL_PRESSURE_SCENARIO)
LOAD_REALTIME_WS_K6_COMMAND := $(LOAD_K6_BIN) run --vus $${K6_VUS:-$(LOAD_DEFAULT_VUS)} --iterations $${K6_ITERATIONS:-$(LOAD_DEFAULT_ITERATIONS)} $(LOAD_REALTIME_WS_SCENARIO)
LOAD_SUSTAINED_SOAK_K6_COMMAND := $(LOAD_K6_BIN) run $(LOAD_SUSTAINED_SOAK_SCENARIO)
LOAD_LOCAL_AYB_START_COMMAND := ./ayb start --foreground --config tests/load/ayb-load.toml

define LOAD_BOOTSTRAP_FUNCTIONS
set -euo pipefail; \
load_export_env() { \
	export AYB_AUTH_RATE_LIMIT="$${AYB_AUTH_RATE_LIMIT:-$(LOAD_AUTH_RATE_LIMIT_DEFAULT)}"; \
	export AYB_RATE_LIMIT_API="$${AYB_RATE_LIMIT_API:-$(LOAD_API_RATE_LIMIT_DEFAULT)}"; \
	export AYB_RATE_LIMIT_API_ANONYMOUS="$${AYB_RATE_LIMIT_API_ANONYMOUS:-$(LOAD_API_ANON_RATE_LIMIT_DEFAULT)}"; \
	export AYB_BASE_URL="$${AYB_BASE_URL:-$(LOAD_DEFAULT_BASE_URL)}"; \
}; \
	load_export_auth_env() { \
		local resolved_auth_jwt_secret="$${AYB_AUTH_JWT_SECRET:-}"; \
		if [ -z "$$resolved_auth_jwt_secret" ]; then \
			resolved_auth_jwt_secret="$$(python3 -c "import secrets; print(secrets.token_urlsafe(48))")"; \
		fi; \
		export AYB_AUTH_ENABLED="$${AYB_AUTH_ENABLED:-$(LOAD_AUTH_ENABLED_DEFAULT)}"; \
		export AYB_AUTH_JWT_SECRET="$$resolved_auth_jwt_secret"; \
	}; \
	load_export_admin_password_env() { \
		local resolved_admin_password="$${AYB_ADMIN_PASSWORD:-}"; \
		if [ -z "$$resolved_admin_password" ]; then \
			resolved_admin_password="$$(python3 -c "import secrets; print(secrets.token_urlsafe(36))")"; \
		fi; \
		export AYB_ADMIN_PASSWORD="$$resolved_admin_password"; \
	}; \
	load_base_url_is_loopback() { \
		local _url="$${AYB_BASE_URL:-$(LOAD_DEFAULT_BASE_URL)}"; \
		local _hp="$${_url#*://}"; _hp="$${_hp%%/*}"; _hp="$${_hp%%\?*}"; \
		local _host; \
		case "$$_hp" in \
			\[*) _host="$${_hp#\[}"; _host="$${_host%%]*}" ;; \
			*) _host="$${_hp%%:*}" ;; \
		esac; \
		case "$$_host" in \
			localhost|127.0.0.1|::1) return 0 ;; \
			*) return 1 ;; \
		esac; \
	}; \
	load_exchange_admin_password_for_token() { \
		local admin_password="$$1"; \
		local login_payload login_response; \
		if [ -z "$$admin_password" ]; then \
			return 0; \
		fi; \
		login_payload="$$(printf "%s" "$$admin_password" | python3 -c "import json,sys; print(json.dumps(dict(password=sys.stdin.read())))")"; \
		login_response="$$(curl -fsS -H "Content-Type: application/json" --data "$$login_payload" "$${AYB_BASE_URL%/}/api/admin/auth" 2>/dev/null || true)"; \
		if [ -n "$$login_response" ]; then \
			printf "%s" "$$login_response" | python3 -c "import json,sys; print(json.load(sys.stdin).get(\"token\", \"\"))" 2>/dev/null || true; \
		fi; \
	}; \
	load_resolve_admin_token() { \
		local resolved_admin_token="$${AYB_ADMIN_TOKEN:-}"; \
		local admin_password_from_file file_exchange_result; \
		if [ -z "$$resolved_admin_token" ] && ! load_base_url_is_loopback; then \
			printf "AYB_ADMIN_TOKEN must be set for non-loopback AYB_BASE_URL; refusing ~/.ayb/admin-token fallback for %s\n" "$$AYB_BASE_URL" >&2; \
			return 1; \
		fi; \
		if [ -z "$$resolved_admin_token" ]; then \
			resolved_admin_token="$$(load_exchange_admin_password_for_token "$${AYB_ADMIN_PASSWORD:-}")"; \
		fi; \
		if [ -z "$$resolved_admin_token" ] && [ -f "$${HOME}/.ayb/admin-token" ]; then \
			admin_password_from_file="$$(head -n 1 "$${HOME}/.ayb/admin-token" | sed 's/\r$$//')"; \
			if [ -n "$$admin_password_from_file" ]; then \
				file_exchange_result="$$(load_exchange_admin_password_for_token "$$admin_password_from_file")"; \
				if [ -n "$$file_exchange_result" ]; then \
					resolved_admin_token="$$file_exchange_result"; \
				elif [ -z "$${AYB_ADMIN_PASSWORD:-}" ]; then \
					resolved_admin_token="$$admin_password_from_file"; \
				fi; \
			fi; \
		fi; \
		if [ -z "$$resolved_admin_token" ]; then \
			printf "Unable to resolve AYB admin token: set AYB_ADMIN_TOKEN or provide valid AYB_ADMIN_PASSWORD/~/.ayb/admin-token for %s\n" "$$AYB_BASE_URL" >&2; \
			return 1; \
		fi; \
		export AYB_ADMIN_TOKEN="$$resolved_admin_token"; \
	}
endef

define BROWSER_EXPORT_AUTH_ENV
set -euo pipefail; \
export AYB_AUTH_ENABLED="$${AYB_AUTH_ENABLED:-$(BROWSER_AUTH_ENABLED_DEFAULT)}"; \
if [ -z "$${AYB_AUTH_JWT_SECRET:-}" ]; then \
	export AYB_AUTH_JWT_SECRET="$$(python3 -c "import secrets; print(secrets.token_urlsafe(48))")"; \
fi
endef

help: ## Show this help
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-15s\033[0m %s\n", $$1, $$2}'

# Demo source dependencies (src + build config, not tests)
KANBAN_DEPS := $(shell find examples/kanban/src -type f) \
	examples/kanban/index.html examples/kanban/package.json examples/kanban/package-lock.json \
	examples/kanban/vite.config.ts examples/kanban/tsconfig.json \
	examples/kanban/tailwind.config.js examples/kanban/postcss.config.js
POLLS_DEPS := $(shell find examples/live-polls/src -type f) \
	examples/live-polls/index.html examples/live-polls/package.json examples/live-polls/package-lock.json \
	examples/live-polls/vite.config.ts examples/live-polls/tsconfig.json \
	examples/live-polls/tailwind.config.js examples/live-polls/postcss.config.js
MOVIES_DEPS := $(shell find examples/movies/src -type f) \
	examples/movies/index.html examples/movies/package.json examples/movies/package-lock.json \
	examples/movies/vite.config.ts examples/movies/tsconfig.json \
	examples/movies/tailwind.config.js examples/movies/postcss.config.js
INSTANTSEARCH_DEPS := $(shell find examples/instantsearch_demo/src -type f) \
	examples/instantsearch_demo/index.html examples/instantsearch_demo/package.json \
	examples/instantsearch_demo/package-lock.json examples/instantsearch_demo/vite.config.ts \
	examples/instantsearch_demo/tsconfig.json
UI_DEPS := $(shell find ui/src -type f) \
	ui/index.html ui/package.json ui/pnpm-lock.yaml \
	ui/vite.config.ts ui/tsconfig.json ui/postcss.config.js ui/tailwind.config.ts

sdk/dist/.stamp: $(shell find sdk/src -type f) sdk/package.json sdk/package-lock.json sdk/tsconfig.json
	cd sdk && npm ci && npm run build
	@touch $@

sdk_react/dist/.stamp: $(shell find sdk_react/src -type f) sdk_react/package.json sdk_react/pnpm-lock.yaml sdk_react/tsconfig.json sdk/dist/.stamp
	cd sdk_react && pnpm install && pnpm run build
	@touch $@

examples/kanban/dist/.stamp: $(KANBAN_DEPS) sdk/dist/.stamp sdk_react/dist/.stamp
	cd examples/kanban && npm ci && VITE_AYB_URL="" npx vite build
	@touch $@

examples/live-polls/dist/.stamp: $(POLLS_DEPS) sdk/dist/.stamp sdk_react/dist/.stamp
	cd examples/live-polls && npm ci && VITE_AYB_URL="" npx vite build
	@touch $@

examples/movies/dist/.stamp: $(MOVIES_DEPS) sdk/dist/.stamp sdk_react/dist/.stamp
	cd examples/movies && npm ci && VITE_AYB_URL="" npx vite build
	@touch $@

examples/instantsearch_demo/dist/.stamp: $(INSTANTSEARCH_DEPS) sdk/dist/.stamp
	cd examples/instantsearch_demo && npm ci && VITE_AYB_URL="" npm run build
	@touch $@

ui/dist/.stamp: $(UI_DEPS)
	cd ui && pnpm install && pnpm build
	@touch $@

build: ui/dist/.stamp examples/kanban/dist/.stamp examples/live-polls/dist/.stamp examples/movies/dist/.stamp examples/instantsearch_demo/dist/.stamp ## Build the ayb binary (rebuilds UI + demos if sources changed)
	go build $(LDFLAGS) -o ayb ./cmd/ayb

build-postgres: ## Build AYB-managed Postgres binaries for the current platform
	bash scripts/build-postgres.sh

dev: ## Build and run with a test database URL (set DATABASE_URL)
	go run $(LDFLAGS) ./cmd/ayb start --database-url "$(DATABASE_URL)"

test: ## Run Go unit tests (no DB, fast)
	go tool gotestsum --format testdox -- -count=1 ./...

test-sdk: ## Run SDK unit tests (vitest, no browser)
	cd sdk && npm ci && npm test

test-sdk-go: ## Run Go SDK checks
	cd sdk_go && go vet ./... && go test -count=1 ./...

test-sdk-python: ## Run Python SDK checks
	@# Isolated environments avoid PEP 668; uv --locked also exposes lock drift.
	@if command -v uv >/dev/null 2>&1; then \
		cd sdk_python && uv run --locked --extra dev ruff check src/ --ignore E501 && uv run --locked --extra dev pytest; \
	else \
		cd sdk_python && python3 -m venv .venv && .venv/bin/python -m pip install -q ".[dev]" && .venv/bin/ruff check src/ --ignore E501 && .venv/bin/pytest; \
	fi

test-sdk-dart: ## Run Dart SDK checks
	cd sdk_dart && dart pub get && dart analyze && dart test

test-sdk-swift: ## Run Swift SDK build + tests (macOS)
	cd sdk_swift && swift build && swift run AllyourbaseTestRunner

test-sdk-kotlin: ## Run Kotlin SDK tests (requires JDK)
	cd sdk_kotlin && ./gradlew test

test-sdk-react: ## Run React SDK checks (requires JS SDK deps for relative source imports)
	cd sdk && npm ci && cd ../sdk_react && pnpm install && pnpm test

test-sdk-ssr: ## Run SSR SDK checks (requires JS SDK deps for relative source imports)
	cd sdk && npm ci && cd ../sdk_ssr && pnpm install && pnpm test

test-sdk-all: ## Run all locally-runnable SDK checks (excludes Kotlin)
	$(MAKE) test-sdk
	$(MAKE) test-sdk-go
	$(MAKE) test-sdk-python
	$(MAKE) test-sdk-dart
	$(MAKE) test-sdk-swift
	$(MAKE) test-sdk-react
	$(MAKE) test-sdk-ssr

test-ui: ## Run UI component tests (vitest + jsdom, no browser)
	cd ui && pnpm install --frozen-lockfile && pnpm test

test-demos-unit: sdk/dist/.stamp sdk_react/dist/.stamp ## Run Vitest-only demo unit suites
	cd examples/live-polls && npm ci --prefer-offline --no-audit && npm test
	cd examples/movies && npm ci --prefer-offline --no-audit && npm test
	cd examples/instantsearch_demo && npm ci --prefer-offline --no-audit && npm test

test-quickstart-contract: check-installer ## Run installer, documented-command coverage, and live first-run contracts
	bash tests/test_run_with_ayb.sh
	bash tests/test_extract_doc_block.sh
	AYB_QUICKSTART_BIN="$(AYB_QUICKSTART_BIN)" bash tests/test_quickstart_e2e.sh

test-integration: ## Run integration tests (uses AYB's managed Postgres — no Docker needed)
	bash scripts/run-integration-tests.sh

test-multinode: build ## Real 2-node cross-node E2E: shared PG + MinIO, ephemeral ports (needs Docker)
	go run ./internal/testutil/cmd/testpg -- \
	  go test -tags=multinode -count=1 -timeout 300s ./internal/e2e/multinode/...

test-cell: ## LB-routed cell topology E2E: compose PG + MinIO + ayb1/ayb2 + nginx LB (needs Docker; builds images via compose)
	go test -tags=cell -count=1 -timeout 600s ./internal/e2e/celltopology/...

test-demo-smoke: ## Run demo smoke tests only — schema apply, tables, RLS, CRUD (needs managed Postgres)
	go run ./internal/testutil/cmd/testpg -- go tool gotestsum --format testdox -- -tags=integration -count=1 -run TestDemoSmoke ./internal/e2e/

test-demo-instantsearch: ## Run InstantSearch demo unit/build/lint/node/browser tests against a live runtime
	@bash -lc 'set -euo pipefail; source tests/port_helpers.sh; AYB_API_PORT="$$(pick_free_port 48093 49093 50093 51093 52093)" || { echo "No free isolated AYB API port available for InstantSearch demo" >&2; exit 1; }; export AYB_API_PORT; AYB_APP_PORT="$$(pick_free_port 45196 46196 47196 48196 49196)" || { echo "No free isolated app port available for InstantSearch demo" >&2; exit 1; }; export AYB_APP_PORT; AYB_DATABASE_EMBEDDED_PORT="$$(pick_free_port 45435 46435 47435 48435 49435)" || { echo "No free isolated embedded Postgres port available for InstantSearch demo" >&2; exit 1; }; export AYB_DATABASE_EMBEDDED_PORT; cd examples/instantsearch_demo; npm test; npm run build; npm run lint:browser-tests; npm run test:node-probe; browser_status=0; npm run test:browser-tests || browser_status=$$?; cd ../..; guard_status=0; bash scripts/check-playwright-executed.sh examples/instantsearch_demo/playwright-report/results.json instantsearch_demo || guard_status=$$?; test "$$browser_status" -eq 0 && test "$$guard_status" -eq 0'

test-smoke: build ## Run Playwright smoke tests — 8 critical paths, ~5 min (builds + starts server)
	@bash -lc '$(BROWSER_EXPORT_AUTH_ENV); export AYB_START_COMMAND="$${AYB_START_COMMAND:-$(BROWSER_LOCAL_AYB_START_COMMAND)}"; bash scripts/run-with-ayb.sh "cd ui && npm run test:browser -- --project=smoke"'

test-browser-full: build ## Run Playwright full browser suite, ~15 min (builds + starts server)
	@bash -lc '$(BROWSER_EXPORT_AUTH_ENV); export AYB_START_COMMAND="$${AYB_START_COMMAND:-$(BROWSER_LOCAL_AYB_START_COMMAND)}"; bash scripts/run-with-ayb.sh "cd ui && npm run test:browser -- --project=full"'

test-e2e: build ## Run all Playwright tests — smoke + full (builds + starts server)
	@bash -lc '$(BROWSER_EXPORT_AUTH_ENV); export AYB_START_COMMAND="$${AYB_START_COMMAND:-$(BROWSER_LOCAL_AYB_START_COMMAND)}"; bash scripts/run-with-ayb.sh "cd ui && npm run test:browser"'

test-sdk-integration: build ## Run the SDK integration suite against a live AYB — auth + storage (builds + starts server)
	bash tests/test_sdk_integration_port_contract.sh
	cd sdk && npm ci
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_base_url_is_loopback load_exchange_admin_password_for_token load_resolve_admin_token; source tests/port_helpers.sh; if [[ -z "$${AYB_BASE_URL:-}" && -z "$${AYB_HEALTH_URL:-}" && -z "$${AYB_SERVER_PORT:-}" ]]; then AYB_SERVER_PORT="$$(pick_free_port 48091 49091 50091 51091 52091)" || { echo "No free isolated AYB server port available for SDK integration" >&2; exit 1; }; export AYB_SERVER_PORT; fi; if [[ -z "$${AYB_DATABASE_URL:-}" && -z "$${AYB_DATABASE_EMBEDDED_PORT:-}" ]]; then AYB_DATABASE_EMBEDDED_PORT="$$(pick_free_port 45433 46433 47433 48433 49433)" || { echo "No free isolated embedded Postgres port available for SDK integration" >&2; exit 1; }; export AYB_DATABASE_EMBEDDED_PORT; fi; $(BROWSER_EXPORT_AUTH_ENV); export AYB_STORAGE_ENABLED=true; export AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true; bash scripts/run-with-ayb.sh '"'"'load_resolve_admin_token && bash scripts/sdk_live_proof_seed.sh && cd sdk && npm run test:integration && cd ../sdk_go && export AYB_TEST_URL="$${AYB_BASE_URL}" AYB_TEST_COLLECTION="$${AYB_SDK_LIVE_PROOF_COLLECTION:-sdk_kotlin_search_posts}" AYB_TEST_ADMIN_TOKEN="$${AYB_ADMIN_TOKEN}" && go test -count=1 -run TestE2E ./... -v'"'"''

test-sdk-python-integration: build ## Run the Python SDK edge-invoke live proof against a live AYB (builds + starts server)
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_base_url_is_loopback load_exchange_admin_password_for_token load_resolve_admin_token; source tests/port_helpers.sh; if [[ -z "$${AYB_BASE_URL:-}" && -z "$${AYB_HEALTH_URL:-}" && -z "$${AYB_SERVER_PORT:-}" ]]; then AYB_SERVER_PORT="$$(pick_free_port 48091 49091 50091 51091 52091)" || { echo "No free isolated AYB server port available for Python SDK live proof" >&2; exit 1; }; export AYB_SERVER_PORT; fi; if [[ -z "$${AYB_DATABASE_URL:-}" && -z "$${AYB_DATABASE_EMBEDDED_PORT:-}" ]]; then AYB_DATABASE_EMBEDDED_PORT="$$(pick_free_port 45433 46433 47433 48433 49433)" || { echo "No free isolated embedded Postgres port available for Python SDK live proof" >&2; exit 1; }; export AYB_DATABASE_EMBEDDED_PORT; fi; $(BROWSER_EXPORT_AUTH_ENV); export AYB_STORAGE_ENABLED=true; export AYB_AUTH_ANONYMOUS_AUTH_ENABLED=true; bash scripts/run-with-ayb.sh '"'"'load_resolve_admin_token && bash scripts/sdk_live_proof_seed.sh && export AYB_TEST_URL="$${AYB_BASE_URL}" AYB_TEST_COLLECTION="$${AYB_SDK_LIVE_PROOF_COLLECTION:-sdk_kotlin_search_posts}" AYB_TEST_ADMIN_TOKEN="$${AYB_ADMIN_TOKEN}" && cd sdk_python && python3 -m pip install ".[dev]" -q && python3 -m pytest tests/test_e2e.py -k "edge_invoke_live"'"'"''

load-admin-status: ## Run direct k6 baseline scenario against AYB_BASE_URL (default http://127.0.0.1:8090)
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_resolve_admin_token; $(LOAD_ADMIN_STATUS_K6_COMMAND)'

load-admin-status-local: ## Start local AYB with run-with-ayb and run the baseline k6 scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_base_url_is_loopback load_exchange_admin_password_for_token load_resolve_admin_token; load_export_env; load_export_admin_password_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token && $(LOAD_ADMIN_STATUS_K6_COMMAND)"'

load-auth-request-path: ## Run direct k6 auth register/login/refresh scenario against AYB_BASE_URL
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_resolve_admin_token; $(LOAD_AUTH_REQUEST_PATH_K6_COMMAND)'

load-auth-request-path-local: ## Start local AYB with run-with-ayb and run the auth register/login/refresh scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_base_url_is_loopback load_exchange_admin_password_for_token load_resolve_admin_token; load_export_env; load_export_auth_env; load_export_admin_password_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token && $(LOAD_AUTH_REQUEST_PATH_K6_COMMAND)"'

load-data-path: ## Run direct k6 collection CRUD/batch data-path scenario against AYB_BASE_URL
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_resolve_admin_token; $(LOAD_DATA_PATH_K6_COMMAND)'

load-data-path-local: ## Start local AYB with run-with-ayb and run the collection CRUD/batch data-path scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_base_url_is_loopback load_exchange_admin_password_for_token load_resolve_admin_token; load_export_env; load_export_auth_env; load_export_admin_password_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token && $(LOAD_DATA_PATH_K6_COMMAND)"'

load-data-pool-pressure: ## Run direct k6 admin SQL pool-pressure scenario against AYB_BASE_URL
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_resolve_admin_token; $(LOAD_DATA_POOL_PRESSURE_K6_COMMAND)'

load-data-pool-pressure-local: ## Start local AYB with run-with-ayb and run the admin SQL pool-pressure scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_base_url_is_loopback load_exchange_admin_password_for_token load_resolve_admin_token; load_export_env; load_export_admin_password_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token && $(LOAD_DATA_POOL_PRESSURE_K6_COMMAND)"'

load-http-100: ## Run direct HTTP load scenario suite at 100 VUs/iterations per scenario
	@K6_VUS=100 K6_ITERATIONS=100 $(MAKE) load-admin-status
	@K6_VUS=100 K6_ITERATIONS=100 $(MAKE) load-auth-request-path
	@K6_VUS=100 K6_ITERATIONS=100 $(MAKE) load-data-path
	@K6_VUS=100 K6_ITERATIONS=100 $(MAKE) load-data-pool-pressure

load-http-500: ## Run direct HTTP load scenario suite at 500 VUs/iterations per scenario
	@K6_VUS=500 K6_ITERATIONS=500 $(MAKE) load-admin-status
	@K6_VUS=500 K6_ITERATIONS=500 $(MAKE) load-auth-request-path
	@K6_VUS=500 K6_ITERATIONS=500 $(MAKE) load-data-path
	@K6_VUS=500 K6_ITERATIONS=500 $(MAKE) load-data-pool-pressure

load-http-1000: ## Run direct HTTP load scenario suite at 1000 VUs/iterations per scenario
	@K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-admin-status
	@K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-auth-request-path
	@K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-data-path
	@K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-data-pool-pressure

define LOAD_REQUIRE_UNSAFE_LOCAL_TIER
if [ "$${AYB_LOAD_UNSAFE:-}" != "1" ]; then \
	printf "Refusing dangerous local load tier '%s': set AYB_LOAD_UNSAFE=1 to continue\n" "$(1)" >&2; \
	exit 1; \
fi;
endef

load-http-100-local: ## Start local AYB with pg_cron-free load config and run the HTTP load suite at 100 VUs/iterations
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_export_admin_password_env; export AYB_START_COMMAND="$${AYB_START_COMMAND:-$(LOAD_LOCAL_AYB_START_COMMAND)}"; bash scripts/run-with-ayb.sh "$(MAKE) load-http-100"'

load-http-500-local: ## Start local AYB with pg_cron-free load config and run the HTTP load suite at 500 VUs/iterations
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_export_admin_password_env; export AYB_START_COMMAND="$${AYB_START_COMMAND:-$(LOAD_LOCAL_AYB_START_COMMAND)}"; bash scripts/run-with-ayb.sh "$(MAKE) load-http-500"'

load-http-1000-local: ## DANGEROUS local HTTP load tier (1000 VUs/iterations); requires AYB_LOAD_UNSAFE=1
	@bash -lc '$(call LOAD_REQUIRE_UNSAFE_LOCAL_TIER,load-http-1000-local) $(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_export_admin_password_env; export AYB_START_COMMAND="$${AYB_START_COMMAND:-$(LOAD_LOCAL_AYB_START_COMMAND)}"; bash scripts/run-with-ayb.sh "$(MAKE) load-http-1000"'

load-realtime-ws: ## Run direct k6 realtime websocket subscribe scenario against AYB_BASE_URL
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_resolve_admin_token; $(LOAD_REALTIME_WS_K6_COMMAND)'

load-realtime-ws-local: ## Start local AYB with run-with-ayb and run the realtime websocket subscribe scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_base_url_is_loopback load_exchange_admin_password_for_token load_resolve_admin_token; load_export_env; load_export_auth_env; load_export_admin_password_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token && $(LOAD_REALTIME_WS_K6_COMMAND)"'

load-realtime-ws-1000: ## Run direct realtime websocket scenario at 1000 VUs/iterations
	@K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-realtime-ws

load-realtime-ws-1000-local: ## DANGEROUS local realtime websocket load tier (1000 VUs/iterations); requires AYB_LOAD_UNSAFE=1
	@bash -lc '$(call LOAD_REQUIRE_UNSAFE_LOCAL_TIER,load-realtime-ws-1000-local) $(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_export_admin_password_env; export AYB_START_COMMAND="$${AYB_START_COMMAND:-$(LOAD_LOCAL_AYB_START_COMMAND)}"; bash scripts/run-with-ayb.sh "K6_VUS=1000 K6_ITERATIONS=1000 $(MAKE) load-realtime-ws"'

load-realtime-ws-5000: ## Run direct realtime websocket scenario at 5000 VUs/iterations
	@K6_VUS=5000 K6_ITERATIONS=5000 $(MAKE) load-realtime-ws

load-realtime-ws-5000-local: ## DANGEROUS local realtime websocket load tier (5000 VUs/iterations); requires AYB_LOAD_UNSAFE=1
	@bash -lc '$(call LOAD_REQUIRE_UNSAFE_LOCAL_TIER,load-realtime-ws-5000-local) $(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_export_admin_password_env; export AYB_START_COMMAND="$${AYB_START_COMMAND:-$(LOAD_LOCAL_AYB_START_COMMAND)}"; bash scripts/run-with-ayb.sh "K6_VUS=5000 K6_ITERATIONS=5000 $(MAKE) load-realtime-ws"'

load-realtime-ws-10000: ## Run direct realtime websocket scenario at 10000 VUs/iterations
	@K6_VUS=10000 K6_ITERATIONS=10000 $(MAKE) load-realtime-ws

load-realtime-ws-10000-local: ## DANGEROUS local realtime websocket load tier (10000 VUs/iterations); requires AYB_LOAD_UNSAFE=1
	@bash -lc '$(call LOAD_REQUIRE_UNSAFE_LOCAL_TIER,load-realtime-ws-10000-local) $(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_export_admin_password_env; export AYB_START_COMMAND="$${AYB_START_COMMAND:-$(LOAD_LOCAL_AYB_START_COMMAND)}"; bash scripts/run-with-ayb.sh "K6_VUS=10000 K6_ITERATIONS=10000 $(MAKE) load-realtime-ws"'

load-sustained-soak: ## Run direct k6 sustained mixed-workload soak scenario against AYB_BASE_URL
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); load_export_env; load_export_auth_env; load_resolve_admin_token; $(LOAD_SUSTAINED_SOAK_K6_COMMAND)'

load-sustained-soak-local: ## Start local AYB with run-with-ayb and run the sustained mixed-workload soak scenario
	@bash -lc '$(LOAD_BOOTSTRAP_FUNCTIONS); export -f load_base_url_is_loopback load_exchange_admin_password_for_token load_resolve_admin_token; load_export_env; load_export_auth_env; load_export_admin_password_env; bash scripts/run-with-ayb.sh "load_resolve_admin_token && $(LOAD_SUSTAINED_SOAK_K6_COMMAND)"'

test-all: test test-integration test-sdk test-ui test-demos-unit ## Run all fast tests: Go unit + integration + SDK + UI + demo units

test-full: test-all test-e2e ## Run every automated test: unit + integration + SDK + UI + all browser tests (~1.5 hrs)

# Full per-demo Playwright suites, including live-polls passkey coverage.
test-demo-e2e: build ## Run demo app E2E tests — Playwright suites for kanban + live-polls + movies (starts demo, runs tests, stops)
	@cd _dev/manual_smoke_tests && AYB_BIN=$(CURDIR)/ayb bash 18_demo_e2e.test.sh

# Deliberately excluded from test-all/test-full: this lane pulls the multi-hundred-MB
# nomic-embed-text model and needs exclusive use of the fixed Ollama port 11434, so it
# runs as its own CI job and its own local command rather than in the fast lane.
test-demo-movies-real-provider: build ## Run movies real-provider smoke — real Ollama embeddings end to end (needs ollama + free port 11434)
	@AYB_BIN=$(CURDIR)/ayb bash _dev/manual_smoke_tests/19_movies_real_provider.test.sh

test-demo-launch: build ## Run demo launch smoke — kanban + live-polls + movies CLI launch contracts
	@cd _dev/manual_smoke_tests && AYB_BIN=$(CURDIR)/ayb bash 17_demo_launch.test.sh

test-push-smoke: build ## Run push notification runtime smoke through the log provider
	AYB_BIN=$(CURDIR)/ayb bash _dev/manual_smoke_tests/20_push_smoke.test.sh

# Cross-demo roundtrip smoke only; narrower than the full per-demo suites above.
test-demo-cross-smoke: build ## Run cross-demo Playwright smoke — kanban + live-polls + movies in one suite
	@cd tests/e2e && npm ci --prefer-offline --no-audit && \
		AYB_BIN=$(CURDIR)/ayb npx playwright install chromium >/dev/null && \
		AYB_BIN=$(CURDIR)/ayb npx playwright test --reporter=line cross_demo.spec.ts

test-api-journey: build ## Run full_journey API smoke lifecycle via run-with-ayb
	@AYB_AUTH_ENABLED=true AYB_STORAGE_ENABLED=true bash scripts/run-with-ayb.sh 'cd _dev/manual_smoke_tests && python3 full_journey.test.py'

test-api-smoke: build ## Run API smoke suite via run-with-ayb (starts server, runs run_all_tests.sh, stops server)
	@AYB_AUTH_ENABLED=true AYB_STORAGE_ENABLED=true bash scripts/run-with-ayb.sh 'cd _dev/manual_smoke_tests && ./run_all_tests.sh'

test-everything: build ## Run absolutely everything: unit + integration + SDK + UI + browser + API smoke tests
	@failed=""; passed=""; \
	run_step() { \
		printf "\n\033[1;34m━━━ $$1 ━━━\033[0m\n"; \
		if ( eval "$$2" ); then \
			passed="$$passed\n  ✓ $$1"; \
		else \
			failed="$$failed\n  ✗ $$1"; \
		fi; \
	}; \
	run_step "Go unit tests"      "go tool gotestsum --format testdox -- -count=1 ./..."; \
	run_step "Integration tests"  "bash scripts/run-integration-tests.sh"; \
	run_step "SDK tests"          "cd sdk && npm test"; \
	run_step "All SDK tests"      "$(MAKE) test-sdk-all"; \
	run_step "UI component tests" "cd ui && pnpm test"; \
	run_step "Playwright e2e"     "$(MAKE) test-e2e"; \
	run_step "Demo app E2E"       "cd _dev/manual_smoke_tests && AYB_BIN=$(CURDIR)/ayb bash 18_demo_e2e.test.sh"; \
	run_step "Cross-demo smoke (Playwright)" "$(MAKE) test-demo-cross-smoke"; \
	run_step "API smoke tests"    "$(MAKE) test-api-smoke"; \
	printf "\n\033[1m━━━━━━━━━━━━━━━━━━━━━━\033[0m\n"; \
	printf "\033[1m  TEST SUMMARY\033[0m\n"; \
	printf "\033[1m━━━━━━━━━━━━━━━━━━━━━━\033[0m\n"; \
	if [ -n "$$passed" ]; then printf "\033[32m%b\033[0m\n" "$$passed"; fi; \
	if [ -n "$$failed" ]; then printf "\033[31m%b\033[0m\n" "$$failed"; fi; \
	printf "\033[1m━━━━━━━━━━━━━━━━━━━━━━\033[0m\n"; \
	if [ -n "$$failed" ]; then exit 1; fi

lint: ## Run linters (requires golangci-lint)
	golangci-lint run ./...

govulncheck: ## Run dependency + stdlib vulnerability scan
	GOTOOLCHAIN=auto go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

check-sizes: ## Run source-size guardrail
	bash scripts/check-file-sizes.sh

check-screen-specs: ## Run screen-spec format guard
	bash scripts/check_screen_spec.sh docs/reference/screen_specs

check-demo-specs: ## Run demo screen-spec format guard
	bash scripts/check_screen_spec.sh docs/reference/demo_specs

check-followups: ## Report follow-up ledger pollution without blocking aggregate gates
	@status=0; \
	if ! git show HEAD:chats/icg/_followups.md | bash scripts/check_followups.sh HEAD=-; then status=1; fi; \
	if ! bash scripts/check_followups.sh worktree=chats/icg/_followups.md; then status=1; fi; \
	exit $$status

check-ui-bundle-size: ## Build UI and run entry bundle-size guardrail
	$(MAKE) ui
	bash scripts/check_ui_bundle_size.sh

check-hygiene: ## Run leaked worktree path guard
	go test ./internal/codehealth -run TestNoLeakedWorktreePaths -count=1

hygiene: ## Strip leaked worktree paths and run hygiene guard
	bash scripts/strip-leaked-paths.sh
	$(MAKE) check-hygiene

check-ui-lint: ## Lint admin UI TypeScript source
	cd ui && pnpm install --frozen-lockfile && npx eslint src/

check-browser-tests-lint: ## Lint browser test specs
	cd ui && pnpm install --frozen-lockfile && npm run lint:browser-tests && npm run lint:browser-tests:mocked
	cd examples/kanban && npm ci --prefer-offline --no-audit && npm run lint:browser-tests
	cd examples/live-polls && npm ci --prefer-offline --no-audit && npm run lint:browser-tests
	cd examples/movies && npm ci --prefer-offline --no-audit && npm run lint:browser-tests
	cd examples/instantsearch_demo && npm ci --prefer-offline --no-audit && npm run lint:browser-tests

check-func-sizes: ## Run Go function-size guardrail test
	go test ./internal/codehealth -run TestFunctionSizeAllowlist -count=1

check-closeout-ancestry: ## Verify certifying closeouts credit only ancestor repairs
	bash scripts/check-closeout-repair-ancestry.sh --require-corpus

check: hygiene fmt lint govulncheck check-sizes check-screen-specs check-demo-specs check-ui-bundle-size check-ui-lint check-func-sizes check-closeout-ancestry ## Run local CI-equivalent quality checks

demo-check: build ## Run the complete demo validation aggregate
	@set -euo pipefail; \
	source tests/port_helpers.sh; \
	$(MAKE) check-demo-specs; \
	printf 'DEMO-CHECK ARM check-demo-specs: PASS\n'; \
	$(MAKE) test-demos-unit; \
	printf 'DEMO-CHECK ARM test-demos-unit: PASS\n'; \
	kanban_desktop_port="$$(pick_free_port 45173 46173 47173 48173 49173)" || { printf 'No free isolated app port available for kanban desktop\n' >&2; exit 1; }; \
	(cd examples/kanban && AYB_DEMO_APP_PORT="$$kanban_desktop_port" npm run test:e2e -- --project=kanban --retries=0); \
	bash scripts/check-playwright-executed.sh examples/kanban/playwright-report/results.json kanban; \
	printf 'DEMO-CHECK ARM kanban-desktop: PASS\n'; \
	live_polls_desktop_port="$$(pick_free_port 45175 46175 47175 48175 49175)" || { printf 'No free isolated app port available for live-polls desktop\n' >&2; exit 1; }; \
	(cd examples/live-polls && AYB_DEMO_APP_PORT="$$live_polls_desktop_port" npm run test:e2e -- --project=live-polls --retries=0); \
	bash scripts/check-playwright-executed.sh examples/live-polls/playwright-report/results.json live-polls; \
	printf 'DEMO-CHECK ARM live-polls-desktop: PASS\n'; \
	movies_desktop_port="$$(pick_free_port 45177 46177 47177 48177 49177)" || { printf 'No free isolated app port available for movies desktop\n' >&2; exit 1; }; \
	(cd examples/movies && AYB_DEMO_APP_PORT="$$movies_desktop_port" npm run test:e2e -- --project=movies --retries=0); \
	bash scripts/check-playwright-executed.sh examples/movies/playwright-report/results.json movies; \
	printf 'DEMO-CHECK ARM movies-desktop: PASS\n'; \
	kanban_a11y_port="$$(pick_free_port 45183 46183 47183 48183 49183)" || { printf 'No free isolated app port available for kanban a11y\n' >&2; exit 1; }; \
	(cd examples/kanban && AYB_DEMO_APP_PORT="$$kanban_a11y_port" npm run test:e2e -- tests/a11y.spec.ts --project=kanban --retries=0); \
	bash scripts/check-playwright-executed.sh examples/kanban/playwright-report/results.json kanban; \
	printf 'DEMO-CHECK ARM kanban-a11y: PASS\n'; \
	live_polls_a11y_port="$$(pick_free_port 45185 46185 47185 48185 49185)" || { printf 'No free isolated app port available for live-polls a11y\n' >&2; exit 1; }; \
	(cd examples/live-polls && AYB_DEMO_APP_PORT="$$live_polls_a11y_port" npm run test:e2e -- e2e/a11y.spec.ts --project=live-polls --retries=0); \
	bash scripts/check-playwright-executed.sh examples/live-polls/playwright-report/results.json live-polls; \
	printf 'DEMO-CHECK ARM live-polls-a11y: PASS\n'; \
	movies_a11y_port="$$(pick_free_port 45187 46187 47187 48187 49187)" || { printf 'No free isolated app port available for movies a11y\n' >&2; exit 1; }; \
	(cd examples/movies && AYB_DEMO_APP_PORT="$$movies_a11y_port" npm run test:e2e -- e2e/a11y.spec.ts --project=movies --retries=0); \
	bash scripts/check-playwright-executed.sh examples/movies/playwright-report/results.json movies; \
	printf 'DEMO-CHECK ARM movies-a11y: PASS\n'; \
	instantsearch_a11y_port="$$(pick_free_port 45196 46196 47196 48196 49196)" || { printf 'No free isolated app port available for InstantSearch a11y\n' >&2; exit 1; }; \
	(cd examples/instantsearch_demo && AYB_APP_PORT="$$instantsearch_a11y_port" npm run test:browser-tests -- browser-tests-unmocked/smoke/a11y.spec.ts --project=instantsearch_demo --retries=0); \
	bash scripts/check-playwright-executed.sh examples/instantsearch_demo/playwright-report/results.json instantsearch_demo; \
	printf 'DEMO-CHECK ARM instantsearch-a11y: PASS\n'; \
	kanban_mobile_port="$$(pick_free_port 45193 46193 47193 48193 49193)" || { printf 'No free isolated app port available for kanban mobile\n' >&2; exit 1; }; \
	(cd examples/kanban && AYB_DEMO_APP_PORT="$$kanban_mobile_port" npm run test:e2e -- --project=kanban-mobile --retries=0); \
	bash scripts/check-playwright-executed.sh examples/kanban/playwright-report/results.json kanban-mobile; \
	printf 'DEMO-CHECK ARM kanban-mobile: PASS\n'; \
	live_polls_mobile_port="$$(pick_free_port 45195 46195 47195 48195 49195)" || { printf 'No free isolated app port available for live-polls mobile\n' >&2; exit 1; }; \
	(cd examples/live-polls && AYB_DEMO_APP_PORT="$$live_polls_mobile_port" npm run test:e2e -- --project=live-polls-mobile --retries=0); \
	bash scripts/check-playwright-executed.sh examples/live-polls/playwright-report/results.json live-polls-mobile; \
	printf 'DEMO-CHECK ARM live-polls-mobile: PASS\n'; \
	movies_mobile_port="$$(pick_free_port 45197 46197 47197 48197 49197)" || { printf 'No free isolated app port available for movies mobile\n' >&2; exit 1; }; \
	(cd examples/movies && AYB_DEMO_APP_PORT="$$movies_mobile_port" npm run test:e2e -- --project=movies-mobile --retries=0); \
	bash scripts/check-playwright-executed.sh examples/movies/playwright-report/results.json movies-mobile; \
	printf 'DEMO-CHECK ARM movies-mobile: PASS\n'; \
	instantsearch_mobile_port="$$(pick_free_port 45198 46198 47198 48198 49198)" || { printf 'No free isolated app port available for InstantSearch mobile\n' >&2; exit 1; }; \
	(cd examples/instantsearch_demo && AYB_APP_PORT="$$instantsearch_mobile_port" npm run test:browser-tests -- --project=instantsearch_demo-mobile --retries=0); \
	bash scripts/check-playwright-executed.sh examples/instantsearch_demo/playwright-report/results.json instantsearch_demo-mobile; \
	printf 'DEMO-CHECK ARM instantsearch-mobile: PASS\n'; \
	$(MAKE) test-demo-instantsearch; \
	printf 'DEMO-CHECK ARM test-demo-instantsearch: PASS\n'; \
	$(MAKE) test-push-smoke; \
	printf 'DEMO-CHECK ARM test-push-smoke: PASS\n'; \
	bash scripts/demo_freshness_check.sh; \
	printf 'DEMO-CHECK ARM freshness: PASS\n'; \
	[ "$${DEMO_CHECK_SKIP_LIVE:-}" = "1" ] && printf 'DEMO-CHECK ARM live-workflow: SKIP\n' && exit 0; \
	command -v gh >/dev/null || printf 'gh CLI is required for live workflow dispatch/watch\n' >&2; \
	command -v gh >/dev/null; \
	run_url_file="$$(mktemp -t ayb-demo-check-run-url.XXXXXX)"; \
	gh workflow run cross_demo_live.yml -R AllyourbaseHQ/allyourbase | tee "$$run_url_file"; \
	run_url="$$(cat "$$run_url_file")"; \
	rm -f "$$run_url_file"; \
	run_id="$$(printf '%s\n' "$$run_url" | sed -nE 's@.*[/ ]([0-9]+)[[:space:]]*$$@\1@p' | tail -n 1)"; \
	printf '%s\n' "$$run_id" | grep -Eq '^[0-9]+$$' || printf 'Unable to extract numeric workflow run ID from gh dispatch output: %s\n' "$$run_url" >&2; \
	printf '%s\n' "$$run_id" | grep -Eq '^[0-9]+$$'; \
	gh run watch "$$run_id" -R AllyourbaseHQ/allyourbase --exit-status; \
	printf 'DEMO-CHECK ARM live-workflow: PASS\n'

check-installer: ## Run installer validation suite
	sh tests/test_install.sh

launch-check: ## Validate the real public launch surface
	sh scripts/launch-check.sh

adoption: ## Collect live public adoption signal
	sh scripts/adoption.sh

check-sdk-build: ## Build the JavaScript SDK
	cd sdk && npm run build

release-candidate-check: check check-browser-tests-lint test-all test-multinode test-sdk-integration test-cell test-quickstart-contract ui check-sdk-build check-installer test-demo-launch test-demo-e2e test-smoke test-browser-full ## Run the trusted public release candidate gate

ui: ## Build the admin dashboard SPA
	cd ui && pnpm install && pnpm build

demos: ## Build demo apps (force rebuild, pre-built for go:embed)
	rm -f examples/kanban/dist/.stamp examples/live-polls/dist/.stamp examples/movies/dist/.stamp examples/instantsearch_demo/dist/.stamp
	$(MAKE) examples/kanban/dist/.stamp examples/live-polls/dist/.stamp examples/movies/dist/.stamp examples/instantsearch_demo/dist/.stamp

docker: ## Build Docker image locally
	docker build -t allyourbase/ayb:latest -t allyourbase/ayb:$(VERSION) .

docker-runtime-smoke: ## Run the published-image Docker runtime smoke using /tmp bind mounts
	bash scripts/docker-runtime-smoke.sh

clean: ## Remove build artifacts
	rm -f ayb
	rm -rf dist/
	rm -f ui/dist/.stamp examples/kanban/dist/.stamp examples/live-polls/dist/.stamp examples/movies/dist/.stamp examples/instantsearch_demo/dist/.stamp

release: ## Build release binaries via goreleaser (dry run)
	goreleaser release --snapshot --clean

vet: ## Run go vet
	go vet ./...

fmt: ## Check formatting
	@FILES_NEEDING_FMT="$$(find . -name '*.go' -type f ! -path './vendor/*' ! -path './_dev/*' -print0 | xargs -0 gofmt -l)"; \
	test -z "$$FILES_NEEDING_FMT" || (echo "Files need formatting:" && echo "$$FILES_NEEDING_FMT" && exit 1)

sync-openapi: ## Copy OpenAPI spec to docs-site public dir
	cp openapi/openapi.yaml docs-site/public/openapi.yaml
