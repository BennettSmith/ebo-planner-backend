# East Bay Overland — Local Dev Helpers
#
# Prereqs:
#   - Docker Desktop (or compatible)
#   - psql client (optional; also available via docker exec)
#
# Usage:
#   make up            # start postgres
#   make db-create     # create database (if not already created)
#   make db-migrate    # apply migrations (schema)
#   make db-seed       # seed sample data
#   make db-reset      # drop/recreate/apply/seed (destructive)
#
# Environment:
#   - You can override defaults by creating a .env file (see .env.example)
#   - Or pass variables inline: make db-reset POSTGRES_DB=ebo_dev

SHELL := /bin/bash

POSTGRES_USER ?= ebo
POSTGRES_PASSWORD ?= ebo_password
POSTGRES_DB ?= ebo_dev
POSTGRES_PORT ?= 5432

COMPOSE ?= docker compose
DB_SERVICE ?= db

# Connection string for local host access (psql on host)
DATABASE_URL ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:$(POSTGRES_PORT)/$(POSTGRES_DB)?sslmode=disable

# Dev-only seed script (must NOT be included in migrations)
SEED_FILE ?= ./db/seed/seed_dev_optional.sql

PSQL ?= psql
PSQL_FLAGS ?= -v ON_ERROR_STOP=1

.PHONY: help
help:
	@echo "Targets:"
	@echo "  up             Start postgres container"
	@echo "  down           Stop postgres container"
	@echo "  logs           Tail postgres logs"
	@echo "  psql           Open interactive psql shell (inside container)"
	@echo "  db-create      Create database (if missing)"
	@echo "  db-drop        Drop database (destructive)"
	@echo "  db-migrate     Apply schema migrations (golang-migrate)"
	@echo "  db-seed        Apply dev seed (optional)"
	@echo "  db-reset       Drop + create + migrate + seed (destructive)"
	@echo ""
	@echo "Vars (override like: make up POSTGRES_PORT=5433):"
	@echo "  POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB POSTGRES_PORT SEED_FILE DATABASE_URL"

.PHONY: up
up:
	$(COMPOSE) up -d --build

.PHONY: down
down:
	$(COMPOSE) down

.PHONY: logs
logs:
	$(COMPOSE) logs -f $(DB_SERVICE)

.PHONY: migrate
migrate:
	$(COMPOSE) run --rm migrate

.PHONY: psql
psql:
	$(COMPOSE) exec -it $(DB_SERVICE) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

# --- Database lifecycle helpers (run via container to avoid host tooling dependencies) ---

.PHONY: db-create
db-create: up
	@$(COMPOSE) exec -T $(DB_SERVICE) bash -lc '\
		psql -v ON_ERROR_STOP=1 -U "$(POSTGRES_USER)" -d postgres \
			-c "SELECT 1 FROM pg_database WHERE datname = '"'"'$(POSTGRES_DB)'"'"';" | grep -q 1 \
		|| psql -v ON_ERROR_STOP=1 -U "$(POSTGRES_USER)" -d postgres \
			-c "CREATE DATABASE \"$(POSTGRES_DB)\";" \
	'

.PHONY: db-drop
db-drop: up
	@$(COMPOSE) exec -T $(DB_SERVICE) bash -lc '\
		psql -v ON_ERROR_STOP=1 -U "$(POSTGRES_USER)" -d postgres \
			-c "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '"'"'$(POSTGRES_DB)'"'"' AND pid <> pg_backend_pid();" \
		&& psql -v ON_ERROR_STOP=1 -U "$(POSTGRES_USER)" -d postgres \
			-c "DROP DATABASE IF EXISTS \"$(POSTGRES_DB)\";" \
	'

.PHONY: db-migrate
db-migrate: db-create
	@$(COMPOSE) run --rm migrate

.PHONY: db-seed
db-seed: db-migrate
	@test -f "$(SEED_FILE)" || (echo "SEED_FILE not found: $(SEED_FILE)."; exit 1)
	@$(COMPOSE) exec -T $(DB_SERVICE) bash -lc '\
		psql $(PSQL_FLAGS) -U "$(POSTGRES_USER)" -d "$(POSTGRES_DB)" \
	' < "$(SEED_FILE)"

.PHONY: db-reset
db-reset: db-drop db-create db-migrate db-seed
	@echo "Reset complete: $(POSTGRES_DB) on localhost:$(POSTGRES_PORT)"

