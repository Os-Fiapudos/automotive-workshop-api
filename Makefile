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
	@echo '  make reset        — Recria banco do zero (apaga dados) + migrations + seed'
	@echo '  make smoke        — Smoke test: health, login, abre e consulta uma OS'
	@echo ''
	@echo '=== Infraestrutura (Terraform, infra/) e Kubernetes (k8s/) ==='
	@echo '  make infra-init / infra-plan / infra-apply — kind + metrics-server + Postgres (TF_VAR_postgres_password)'
	@echo '  make infra-output   — Outputs (kubeconfig_path, postgres_database_url_template)'
	@echo '  make infra-destroy INFRA_CONFIRM_DESTROY=yes — Derruba cluster e banco (só ambiente descartável)'
	@echo '  make iac-validate   — Validação estática de infra/ e k8s/ (igual à CI)'
	@echo '  make k8s-status / k8s-hpa-load / k8s-pod-recovery / k8s-db-persistence — Evidências (evidence/)'
	@echo ''
	@echo '=== Migrations do banco (golang-migrate) ==='
	@echo '  make migrate-up       — Aplica as migrations pendentes'
	@echo '  make migrate-version  — Mostra a versão atual do schema'
	@echo '  make migrate-create name=<nome> — Cria o próximo par NNNN_<nome>.up/.down.sql'
	@echo '  make migrate-baseline — Adota um banco existente (Fase 1/dirty): backup + comprovação'
	@echo '  make migrate-verify   — Verifica a cadeia inteira em containers descartáveis (igual à CI)'
	@echo '  make migrate-down MIGRATE_CONFIRM_DOWN=yes — Reverte 1 migration (só banco descartável)'
	@echo '  make bootstrap-admin  — Cria o admin a partir de ADMIN_EMAIL/ADMIN_PASSWORD'
	@echo ''
	@echo '=== Deploy em Kubernetes — ambiente de demonstração (specs/ci-cd-pipeline/) ==='
	@echo '  make deploy           — Terraform + migrations + bootstrap + rollout + smoke (IMAGE e segredos no ambiente)'
	@echo '  make deploy-destroy DEPLOY_CONFIRM_DESTROY=yes — Derruba o cluster e o banco'
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

# ─── Infraestrutura e Kubernetes (specs/infrastructure-provisioning/, specs/kubernetes-deployment/) ─
# Terraform owns the kind cluster, metrics-server and Postgres; the API is deployed from
# k8s/ (k8s/README.md). TF_VAR_postgres_password must be in the environment (or in the
# git-ignored infra/terraform.tfvars).

TERRAFORM := terraform -chdir=infra

.PHONY: infra-init
infra-init: ## Inicializa o Terraform (providers pinados)
	$(TERRAFORM) init -input=false

.PHONY: infra-plan
infra-plan: ## Mostra o plano do Terraform (cluster kind, metrics-server, Postgres)
	$(TERRAFORM) plan -input=false

.PHONY: infra-apply
infra-apply: ## Provisiona/atualiza cluster kind, metrics-server e Postgres
	$(TERRAFORM) apply -input=false

.PHONY: infra-output
infra-output: ## Mostra os outputs (nenhum contém credencial)
	$(TERRAFORM) output

.PHONY: infra-destroy
infra-destroy: ## Derruba cluster e banco — só ambiente descartável (INFRA_CONFIRM_DESTROY=yes)
	@test "$(INFRA_CONFIRM_DESTROY)" = yes || { echo "Destrói o cluster e o banco. Rode: make infra-destroy INFRA_CONFIRM_DESTROY=yes"; exit 1; }
	$(TERRAFORM) destroy -input=false

.PHONY: iac-validate
iac-validate: ## Validação estática de infra/ e k8s/ (terraform validate, kubeconform, regras)
	scripts/iac-validate.sh all

.PHONY: k8s-status
k8s-status: ## Estado do namespace: pods, HPA, métricas
	scripts/k8s-evidence.sh status

.PHONY: k8s-hpa-load
k8s-hpa-load: ## Carga dentro do cluster e linha do tempo de réplicas do HPA
	scripts/k8s-evidence.sh hpa-load

.PHONY: k8s-pod-recovery
k8s-pod-recovery: ## Apaga um pod da API e mede a reposição
	scripts/k8s-evidence.sh pod-recovery

.PHONY: k8s-db-persistence
k8s-db-persistence: ## Recria o pod do Postgres e confirma que os dados persistem
	scripts/k8s-evidence.sh db-persistence

# ─── Setup ────────────────────────────────────────────────────────────────────

.PHONY: setup
setup: ## Cria .env a partir de .env.example (não sobrescreve se já existe)
	@if [ ! -f $(ENV_FILE) ]; then \
		cp .env.example $(ENV_FILE); \
		echo "Arquivo $(ENV_FILE) criado a partir de .env.example"; \
		echo "  Edite JWT_SECRET em $(ENV_FILE) antes de usar em produção!"; \
	else \
		echo "• $(ENV_FILE) já existe — nada a fazer."; \
	fi

# ─── Docker ───────────────────────────────────────────────────────────────────

.PHONY: start
start: setup ## Sobe todos os serviços com Docker Compose
	$(COMPOSE) up -d
	@echo ''
	@echo 'Todos os serviços estão rodando!'
	@echo '  API:   http://localhost:$(API_PORT)'
	@echo '  Adminer: http://localhost:$(shell grep -E '^ADMINER_PORT=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo 8081)'
	@echo '  Swagger: http://localhost:$(shell grep -E '^SWAGGER_UI_PORT=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo 8082)'

.PHONY: start-db
start-db: setup ## Sobe apenas o banco Postgres via Docker
	$(COMPOSE) up -d db
	@echo ''
	@echo 'Postgres rodando em localhost:$(DB_PORT)'
	@echo '  DATABASE_URL=$(DATABASE_URL)'

.PHONY: logs
logs: ## Mostra logs dos containers
	$(COMPOSE) logs -f

.PHONY: stop
stop: ## Para todos os serviços (mantém volumes)
	$(COMPOSE) stop
	@echo 'Serviços parados.'

.PHONY: down
down: ## Para e remove containers (mantém volumes)
	$(COMPOSE) down
	@echo 'Containers removidos.'

.PHONY: clean
clean: ## Para e remove containers + volumes (APAGA DADOS)
	$(COMPOSE) down -v
	@echo 'Containers e volumes removidos — dados apagados.'

.PHONY: reset
reset: setup clean start ## Recria banco do zero (apaga dados) + migrations + seed
	@echo ''
	@echo 'Reset completo! Banco recriado pelas migrations e populado pelo serviço seed do Compose.'
	@echo '  Usuário admin: admin@workshop.local / admin123'
	@echo '  Faça login com: make login'

# ─── API local ────────────────────────────────────────────────────────────────

.PHONY: run
run: ## Roda a API localmente (requer Postgres rodando via start-db)
	DATABASE_URL="$(DATABASE_URL)" \
	JWT_SECRET="$(shell grep -E '^JWT_SECRET=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo 'change-me-dev-only')" \
	JWT_TTL="$(shell grep -E '^JWT_TTL=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo '1h')" \
	EXTERNAL_NOTIFICATION_SECRET="$(shell grep -E '^EXTERNAL_NOTIFICATION_SECRET=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo 'change-me-dev-only')" \
	EXTERNAL_STATUS_UPDATE_SECRET="$(shell grep -E '^EXTERNAL_STATUS_UPDATE_SECRET=' $(ENV_FILE) 2>/dev/null | cut -d= -f2 || echo 'change-me-dev-only-status')" \
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
	@echo 'Seed aplicado com sucesso!'
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

.PHONY: smoke
smoke: ## Smoke test da stack local: health, login, abre e consulta uma OS (SMOKE_EXPECT_ORDER_ID após restart)
	@API_PORT=$(API_PORT) scripts/smoke.sh

.PHONY: psql
psql: ## Abre console psql interativo no banco via Docker
	$(COMPOSE) exec -e PGPASSWORD=$(DB_PASS) db psql -U $(DB_USER) -d $(DB_NAME)

# ─── Schema ───────────────────────────────────────────────────────────────────

.PHONY: schema
schema: migrate-check-db ## Recria o esquema do banco (APAGA TUDO) reaplicando as migrations
	@echo 'ATENÇÃO: Isso vai apagar TODOS os dados existentes!'
	$(COMPOSE) exec -T db psql -U $(DB_USER) -d $(DB_NAME) -c 'DROP SCHEMA public CASCADE; CREATE SCHEMA public;'
	@$(MIGRATE_ENV) $(MIGRATE) up
	@echo 'Schema recriado pelas migrations (make seed para os dados de exemplo).'

# ─── Migrations (specs/database-migrations/) ──────────────────────────────────
# The tool containers join the Compose database's network and reach it as db:5432,
# so these targets work the same on Linux, macOS and Windows (no --network host).

MIGRATE         := scripts/migrate.sh
COMPOSE_DB_URL   = postgres://$(DB_USER):$(DB_PASS)@db:5432/$(DB_NAME)?sslmode=disable
COMPOSE_NETWORK  = $(shell docker inspect -f '{{range $$k, $$v := .NetworkSettings.Networks}}{{$$k}}{{end}}' $$($(COMPOSE) ps -q db 2>/dev/null) 2>/dev/null)
MIGRATE_ENV      = DATABASE_URL='$(COMPOSE_DB_URL)' MIGRATE_NETWORK='$(COMPOSE_NETWORK)'

# Passed to `docker compose run -e` by bootstrap-admin, whether set in the
# environment or on the make command line.
export ADMIN_EMAIL ADMIN_PASSWORD ADMIN_NAME

.PHONY: migrate-check-db
migrate-check-db:
	@test -n "$(COMPOSE_NETWORK)" || { echo "O banco do Compose não está rodando — suba com: make start-db"; exit 1; }

.PHONY: migrate-up
migrate-up: migrate-check-db ## Aplica as migrations pendentes no banco do Compose
	@$(MIGRATE_ENV) $(MIGRATE) up

.PHONY: migrate-version
migrate-version: migrate-check-db ## Mostra a versão atual do schema
	@$(MIGRATE_ENV) $(MIGRATE) version

.PHONY: migrate-create
migrate-create: ## Cria o próximo par de migration (make migrate-create name=add_x)
	@test -n "$(name)" || { echo "Uso: make migrate-create name=<nome_em_snake_case>"; exit 1; }
	$(MIGRATE) create $(name)

.PHONY: migrate-down
migrate-down: migrate-check-db ## Reverte 1 migration — só em banco descartável (MIGRATE_CONFIRM_DOWN=yes)
	@$(MIGRATE_ENV) MIGRATE_CONFIRM_DOWN='$(MIGRATE_CONFIRM_DOWN)' $(MIGRATE) down 1

.PHONY: migrate-baseline
migrate-baseline: migrate-check-db ## Adota um banco existente (Fase 1/dirty): backup + comprovação + versão
	@$(MIGRATE_ENV) $(MIGRATE) baseline

.PHONY: migrate-verify
migrate-verify: ## Verifica a cadeia de migrations em containers descartáveis (igual à CI)
	$(MIGRATE) verify

.PHONY: bootstrap-admin
bootstrap-admin: ## Cria o usuário admin a partir de ADMIN_EMAIL/ADMIN_PASSWORD (idempotente)
	@test -n "$$ADMIN_EMAIL" && test -n "$$ADMIN_PASSWORD" || { echo "Uso: ADMIN_EMAIL=... ADMIN_PASSWORD=... make bootstrap-admin"; exit 1; }
	$(COMPOSE) run --build --rm --no-deps -e ADMIN_EMAIL -e ADMIN_PASSWORD -e ADMIN_NAME api /bootstrap-admin

# ─── Deploy em Kubernetes (specs/ci-cd-pipeline/) ─────────────────────────────
# The same staged script the CI deploy job runs; locally it builds the demonstration
# environment and leaves it up. Needs IMAGE and the credentials in the environment
# (README, "Ambiente de demonstração").

.PHONY: deploy
deploy: ## Sobe/atualiza o ambiente de demonstração no Kubernetes (não destrói nada)
	scripts/deploy.sh up

.PHONY: deploy-destroy
deploy-destroy: ## Derruba o ambiente de demonstração (exige DEPLOY_CONFIRM_DESTROY=yes)
	@DEPLOY_CONFIRM_DESTROY='$(DEPLOY_CONFIRM_DESTROY)' scripts/deploy.sh destroy
