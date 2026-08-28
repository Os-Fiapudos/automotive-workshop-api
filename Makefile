# =============================================================================
# Makefile — automotive-workshop-api
#
# Cross-platform (Windows + Linux/macOS) targets using docker compose v2.
# On Windows, run via Git Bash, WSL, or any POSIX-compatible shell with Make.
# =============================================================================

.DEFAULT_GOAL := help

# ─── Variables ────────────────────────────────────────────────────────────────

COMPOSE      := docker compose
GO           := go
ENV_FILE     := .env

# Extract DB credentials from .env (with defaults)
DB_USER      := $(shell grep -E '^POSTGRES_USER='   $(ENV_FILE) 2>/dev/null | cut -d= -f2)
DB_PASS      := $(shell grep -E '^POSTGRES_PASSWORD=' $(ENV_FILE) 2>/dev/null | cut -d= -f2)
DB_NAME      := $(shell grep -E '^POSTGRES_DB='     $(ENV_FILE) 2>/dev/null | cut -d= -f2)
DB_PORT      := $(shell grep -E '^POSTGRES_PORT='   $(ENV_FILE) 2>/dev/null | cut -d= -f2)
API_PORT     := $(shell grep -E '^API_PORT='        $(ENV_FILE) 2>/dev/null | cut -d= -f2)

# Fallback defaults if .env doesn't exist or vars are empty
DB_USER      ?= workshop
DB_PASS      ?= workshop
DB_NAME      ?= automotive_workshop
DB_PORT      ?= 5432
API_PORT     ?= 8080

DATABASE_URL := postgres://$(DB_USER):$(DB_PASS)@localhost:$(DB_PORT)/$(DB_NAME)?sslmode=disable

# ─── Help ─────────────────────────────────────────────────────────────────────

help: ## Mostra esta ajuda
	@echo ''
	@echo '╔══════════════════════════════════════════════════════════════╗'
	@echo '║   automotive-workshop-api — Comandos disponíveis            ║'
	@echo '╚══════════════════════════════════════════════════════════════╝'
	@echo ''
	@echo '=== Opção 1: Tudo via Docker (recomendado) ==='
	@echo '  make setup        — Cria .env a partir de .env.example'
	@echo '  make start        — Sobe todos os serviços (db+adminer+api+swagger)'
	@echo '  make start-db     — Sobe apenas o banco Postgres'
	@echo '  make logs         — Mostra logs dos containers'
	@echo '  make stop         — Para todos os serviços'
	@echo '  make reset        — Recria banco do zero (apaga dados) + seed'
	@echo ''
	@echo '=== Opção 2: Banco no Docker + API local ==='
	@echo '  make start-db     — Sobe apenas o Postgres (docker)'
	@echo '  make run          — Roda a API localmente (go run ./cmd/api)'
	@echo ''
	@echo '=== Comuns a ambas ==='
	@echo '  make seed         — Popula o banco com dados de exemplo'
	@echo '  make test         — Roda todos os testes (go test ./...)'
	@echo '  make test-race    — Roda testes com detector de data race'
	@echo '  make build        — Compila o binário da API'
	@echo '  make login        — Faz login e obtém JWT (admin@workshop.local)'
	@echo '  make curl         — Exemplo: testa health check com curl'
	@echo '  make psql         — Abre console interativo no Postgres'
	@echo '  make clean        — Para e remove containers + volumes'
	@echo '  make help         — Mostra esta ajuda'
	@echo ''
	@echo '=== URLs ==='
	@echo '  API:       http://localhost:$(API_PORT)'
	@echo '  Swagger:   http://localhost:$(shell grep -E '^SWAGGER_UI_PORT=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo 8082)'
	@echo '  Adminer:   http://localhost:$(shell grep -E '^ADMINER_PORT=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo 8081)'
	@echo ''

# ─── Setup ────────────────────────────────────────────────────────────────────

.PHONY: setup
setup: ## Cria .env a partir de .env.example (não sobrescreve se já existe)
	@if [ ! -f $(ENV_FILE) ]; then \
		cp .env.example $(ENV_FILE); \
		echo "✓ Arquivo $(ENV_FILE) criado a partir de .env.example"; \
		echo "  Edite JWT_SECRET em $(ENV_FILE) antes de usar em produção!"; \
	else \
		echo "• $(ENV_FILE) já existe — nada a fazer."; \
	fi

# ─── Docker ───────────────────────────────────────────────────────────────────

.PHONY: start
start: setup ## Sobe todos os serviços com Docker Compose
	$(COMPOSE) up -d
	@echo ''
	@echo '✓ Todos os serviços estão rodando!'
	@echo '  API:   http://localhost:$(API_PORT)'
	@echo '  Adminer: http://localhost:$(shell grep -E '^ADMINER_PORT=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo 8081)'
	@echo '  Swagger: http://localhost:$(shell grep -E '^SWAGGER_UI_PORT=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo 8082)'

.PHONY: start-db
start-db: setup ## Sobe apenas o banco Postgres via Docker
	$(COMPOSE) up -d db
	@echo ''
	@echo '✓ Postgres rodando em localhost:$(DB_PORT)'
	@echo '  DATABASE_URL=$(DATABASE_URL)'

.PHONY: logs
logs: ## Mostra logs dos containers
	$(COMPOSE) logs -f

.PHONY: stop
stop: ## Para todos os serviços (mantém volumes)
	$(COMPOSE) stop
	@echo '✓ Serviços parados.'

.PHONY: down
down: ## Para e remove containers (mantém volumes)
	$(COMPOSE) down
	@echo '✓ Containers removidos.'

.PHONY: clean
clean: ## Para e remove containers + volumes (APAGA DADOS)
	$(COMPOSE) down -v
	@echo '✓ Containers e volumes removidos — dados apagados.'

.PHONY: reset
reset: setup clean start ## Recria banco do zero (apaga dados) + popula com seed
	@sleep 3
	$(MAKE) seed
	@echo ''
	@echo '✓ Reset completo! Banco recriado e populado com dados de exemplo.'
	@echo '  Usuário admin: admin@workshop.local / admin123'
	@echo '  Faça login com: make login'

# ─── API local ────────────────────────────────────────────────────────────────

.PHONY: run
run: ## Roda a API localmente (requer Postgres rodando via start-db)
	DATABASE_URL="$(DATABASE_URL)" \
	JWT_SECRET="$(shell grep -E '^JWT_SECRET=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo 'change-me-dev-only')" \
	JWT_TTL="$(shell grep -E '^JWT_TTL=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo '1h')" \
	$(GO) run ./cmd/api

.PHONY: build
build: ## Compila o binário da API
	$(GO) build -o bin/api ./cmd/api

# ─── Seed ─────────────────────────────────────────────────────────────────────

.PHONY: seed
seed: ## Aplica dados de exemplo (seed.sql) no banco via Docker
	@echo 'Copiando seed.sql para o container...'
	$(COMPOSE) cp docs/seed.sql db:/tmp/seed.sql
	$(COMPOSE) exec -T db psql -U $(DB_USER) -d $(DB_NAME) -f /tmp/seed.sql
	@echo '✓ Seed aplicado com sucesso!'
	@echo '  Usuário admin: admin@workshop.local / admin123'

# ─── Tests ────────────────────────────────────────────────────────────────────

.PHONY: test
test: ## Roda todos os testes (unitários + integração)
	$(GO) test ./...

.PHONY: test-race
test-race: ## Roda testes com detector de data race
	$(GO) test -race ./...

.PHONY: test-v
test-v: ## Roda testes em modo verboso
	$(GO) test -v ./...

# ─── Utilitários ──────────────────────────────────────────────────────────────

.PHONY: login
login: ## Faz login com o usuário admin e exibe o JWT
	@echo '>>> Login como admin@workshop.local / admin123'
	@echo '>>> Resposta:'
	@curl -s -X POST http://localhost:$(API_PORT)/api/v1/auth/login \
		-H 'Content-Type: application/json' \
		-d '{"email":"admin@workshop.local","password":"admin123"}' | jq .

.PHONY: curl
curl: ## Testa o health check da API
	@echo '>>> Health check:'
	@curl -s http://localhost:$(API_PORT)/health | jq .

.PHONY: psql
psql: ## Abre console psql interativo no banco via Docker
	$(COMPOSE) exec -e PGPASSWORD=$(DB_PASS) db psql -U $(DB_USER) -d $(DB_NAME)

# ─── Schema ───────────────────────────────────────────────────────────────────

.PHONY: schema
schema: ## Recria o esquema do banco (apaga e recria tudo via schema.sql)
	@echo 'ATENÇÃO: Isso vai apagar TODOS os dados existentes!'
	$(COMPOSE) exec -T db psql -U $(DB_USER) -d $(DB_NAME) -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
	$(COMPOSE) exec -T db psql -U $(DB_USER) -d $(DB_NAME) -f /docker-entrypoint-initdb.d/01-schema.sql
	@echo '✓ Schema recriado a partir de docs/schema.sql'