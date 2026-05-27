.DEFAULT_GOAL := help

# ── Paths ─────────────────────────────────────────────────────────────────────
COMPOSE_FILE  := deploy/docker-compose.yml
BIN_DIR       := bin
GATEWAY_MAIN  := ./cmd/gateway
ADMIN_MAIN    := ./cmd/admin
TOKEN_MAIN    := ./cmd/token
TESTBACKEND   := ./cmd/testbackend
WORKER_MAIN   := ./cmd/worker
UI_DIR        := web/admin-ui

# ── Local dev env (Docker Compose exposes postgres on 5433, redis on 6379) ───
LOCAL_POSTGRES_DSN  ?= postgres://postgres:9539Abdu@localhost:5433/elitegate_db?sslmode=disable
LOCAL_REDIS_ADDR    ?= localhost:6379
LOCAL_REDIS_PASSWORD ?= redis_secret
LOCAL_JWT_SECRET    ?= supersecretjwtkey_32byteslongkey!
LOCAL_GATEWAY_PORT  ?= 8080
LOCAL_ADMIN_PORT    ?= 9090
LOCAL_ADMIN_API_URL ?= http://localhost:9090

LOCAL_ENV = POSTGRES_DSN="$(LOCAL_POSTGRES_DSN)" \
	REDIS_ADDR="$(LOCAL_REDIS_ADDR)" \
	REDIS_PASSWORD="$(LOCAL_REDIS_PASSWORD)" \
	JWT_SECRET="$(LOCAL_JWT_SECRET)" \
	GATEWAY_PORT="$(LOCAL_GATEWAY_PORT)" \
	ADMIN_PORT="$(LOCAL_ADMIN_PORT)" \
	ADMIN_API_URL="$(LOCAL_ADMIN_API_URL)" \
	APP_ENV=development

# ── Help ──────────────────────────────────────────────────────────────────────
.PHONY: help
help: ## Show available commands
	@echo EliteGate / CoreGuard Gateway
	@echo.
	@echo Usage: make [target]
	@echo.
	@echo Build:
	@echo   build-all          Build all service binaries into $(BIN_DIR)/
	@echo   build-gateway      Build gateway binary
	@echo   build-admin        Build admin binary
	@echo   build-token        Build JWT token CLI
	@echo   build-testbackend  Build test backend binary
	@echo.
	@echo Run (local, uses localhost DB/Redis):
	@echo   run-gateway        Run gateway API
	@echo   run-admin          Run admin API
	@echo   run-testbackend    Run mock upstream on :9090
	@echo   run-worker         Run worker stub
	@echo.
	@echo Quality:
	@echo   test               Run all Go tests
	@echo   fmt                Format Go source
	@echo   vet                Run go vet
	@echo   tidy               Run go mod tidy
	@echo   lint               Run vet (+ golangci-lint if installed)
	@echo   clean              Remove build artifacts
	@echo.
	@echo Docker:
	@echo   docker-up          Start full stack (detached)
	@echo   docker-down        Stop stack
	@echo   docker-rebuild     Rebuild and restart gateway + admin
	@echo   docker-logs        Tail gateway + admin logs
	@echo   docker-ps          Show container status
	@echo   infra-up           Start only postgres + redis
	@echo   infra-down         Stop postgres + redis
	@echo.
	@echo Utilities:
	@echo   deps               Download Go module dependencies
	@echo   token              Generate a test client JWT
	@echo   ui-dev             Start admin UI dev server
	@echo   ui-build           Build admin UI for production

# ── Build ─────────────────────────────────────────────────────────────────────
.PHONY: build-all build-gateway build-admin build-token build-testbackend
build-all: build-gateway build-admin build-token build-testbackend ## Build all binaries

build-gateway: ## Build gateway binary
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/gateway $(GATEWAY_MAIN)

build-admin: ## Build admin binary
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/admin $(ADMIN_MAIN)

build-token: ## Build token CLI
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/token $(TOKEN_MAIN)

build-testbackend: ## Build test backend binary
	@mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/testbackend $(TESTBACKEND)

# ── Run (local) ───────────────────────────────────────────────────────────────
.PHONY: run-gateway run-admin run-testbackend run-worker
run-gateway: ## Run gateway locally against localhost postgres/redis
	$(LOCAL_ENV) go run $(GATEWAY_MAIN)

run-admin: ## Run admin API locally against localhost postgres/redis
	$(LOCAL_ENV) go run $(ADMIN_MAIN)

run-testbackend: ## Run mock upstream server on :9090
	go run $(TESTBACKEND)

run-worker: ## Run worker stub
	go run $(WORKER_MAIN)

# ── Quality ───────────────────────────────────────────────────────────────────
.PHONY: test fmt vet tidy lint clean deps
test: ## Run all Go tests
	go test ./...

fmt: ## Format Go source files
	gofmt -w .
	go fmt ./...

vet: ## Run go vet
	go vet ./...

tidy: ## Tidy go.mod / go.sum
	go mod tidy

lint: vet ## Run static analysis (golangci-lint if available)
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed — skipped"

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)

deps: ## Download Go module dependencies
	go mod download

# ── Docker ────────────────────────────────────────────────────────────────────
.PHONY: docker-up docker-down docker-rebuild docker-logs docker-ps infra-up infra-down
docker-up: ## Start full Docker stack (detached)
	docker compose -f $(COMPOSE_FILE) up -d

docker-down: ## Stop Docker stack
	docker compose -f $(COMPOSE_FILE) down

docker-rebuild: ## Rebuild and restart gateway + admin services
	docker compose -f $(COMPOSE_FILE) up -d --build gateway admin

docker-logs: ## Tail gateway and admin container logs
	docker compose -f $(COMPOSE_FILE) logs -f gateway admin

docker-ps: ## Show running container status
	docker compose -f $(COMPOSE_FILE) ps

infra-up: ## Start only postgres and redis
	docker compose -f $(COMPOSE_FILE) up -d postgres redis

infra-down: ## Stop postgres and redis
	docker compose -f $(COMPOSE_FILE) stop postgres redis

# ── Utilities ─────────────────────────────────────────────────────────────────
.PHONY: token ui-dev ui-build
token: ## Generate a test client JWT (override: make token SECRET=... CLIENT=...)
	$(LOCAL_ENV) go run $(TOKEN_MAIN) -secret "$(LOCAL_JWT_SECRET)" $(if $(CLIENT),-client "$(CLIENT)",) $(if $(ROLE),-role "$(ROLE)",)

ui-dev: ## Start admin UI dev server
	cd $(UI_DIR) && npm run dev

ui-build: ## Build admin UI for production
	cd $(UI_DIR) && npm run build
