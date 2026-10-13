COMPOSE      := docker compose -f deploy/docker-compose.yml
TEST_COMPOSE := docker compose -f deploy/docker-compose.test.yml

# Throwaway database started by `make test-db`; not a real credential.
TEST_DATABASE_URL ?= postgres://onsite:onsite-test@127.0.0.1:55432/postgres?sslmode=disable

SERVER_URL ?= https://localhost:8443
CA_FILE    ?= .dev/ca.pem
FIXTURE    ?= gpu-server-a100

# Named explicitly so Go never wanders into web/node_modules.
GO_PKGS := ./cmd/... ./internal/...

.PHONY: dev down logs build test test-go test-web test-db test-db-down lint agent-fake help

help: ## Show the available commands
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F ':.*## ' '{printf "  make %-13s %s\n", $$1, $$2}'

dev: ## Run Postgres, the server and the web app locally
	$(COMPOSE) up --build -d --wait
	@mkdir -p .dev
	@$(COMPOSE) cp server:/public/ca.pem $(CA_FILE) >/dev/null 2>&1
	@echo ""
	@echo "  Web app:  http://localhost:8080"
	@echo "  Server:   $(SERVER_URL)  (certificate authority saved to $(CA_FILE))"
	@echo "  Logs:     make logs      Stop: make down"

down: ## Stop the local stack (data is kept)
	$(COMPOSE) down

logs: ## Follow the local stack's logs
	$(COMPOSE) logs -f

build: ## Build the server and agent into ./bin
	go build -o bin/server ./cmd/server
	go build -o bin/agent ./cmd/agent

test: test-go test-web ## Run all Go and web tests

test-db: ## Start the throwaway test database
	$(TEST_COMPOSE) up -d --wait

test-db-down: ## Stop the throwaway test database
	$(TEST_COMPOSE) down

test-go: test-db
	TEST_DATABASE_URL='$(TEST_DATABASE_URL)' go test -race $(GO_PKGS)

test-web: web/node_modules
	cd web && npm test

lint: web/node_modules ## Run golangci-lint and the web linter
	golangci-lint run $(GO_PKGS)
	cd web && npm run lint && npm run typecheck

agent-fake: ## Run an agent in fake mode from a fixture (FIXTURE=name)
	go run ./cmd/agent run --fake testdata/fixtures/$(FIXTURE).json --server $(SERVER_URL) --ca-file $(CA_FILE)

web/node_modules: web/package.json web/package-lock.json
	cd web && npm ci
	@touch web/node_modules
