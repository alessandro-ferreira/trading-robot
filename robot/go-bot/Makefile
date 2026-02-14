# Makefile for the Go Trading Bot
-include .env

MAIN_FILE := ./cmd/server/main.go
CONFIG_FILE := ../config.toml

# Use the modern 'docker compose' command. The -f flag points to the root docker-compose.yml
COMPOSE_CMD = docker compose -f ../../docker-compose.yml
MIGRATE_VERSION = v4.15.2

# Database migration settings
DB_USER ?= postgres
DB_PASSWORD ?= postgres
DB_HOST ?= localhost
DB_PORT ?= 5432
DB_NAME ?= trading_db
DB_SSLMODE ?= disable
DB_URL ?= postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)
MIGRATIONS_PATH := ./migrations

.PHONY: help run proto install-tools pre-commit
.PHONY: test test-integration test-all
.PHONY: db-up db-down
.PHONY: install-migrate migrate-up migrate-down migrate-drop migrate-create migrate-version debug-db

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  help             Show this help message."
	@echo "  run              Start the Go Trading Bot."
	@echo "  proto            Generate protobuf files from .proto definitions."
	@echo "  install-tools    Install development tools."
	@echo "  pre-commit       Run pre-commit hooks (requires pre-commit installed)."
	@echo "  test             Run all unit tests (excluding integration tests)."
	@echo "  test-integration Run integration tests (requires running DB and Gateway)."
	@echo "  test-all         Run all unit and integration tests, managing the test environment."
	@echo "  db-up            Start the test database in the background."
	@echo "  db-down          Stop and remove the test database."
	@echo "  install-migrate  Install the golang-migrate CLI tool."
	@echo "  migrate-up       Apply all database migrations."
	@echo "  migrate-down     Revert the last database migration."
	@echo "  migrate-create   Create a new migration file."
	@echo "  migrate-version  Show the current migration version."
	@echo "  db-drop-all      Drop all database tables (clean slate)."
	@echo "  debug-db         Show database connection info (for debugging)."

# Run the Go Trading Bot
run:
	@echo "==> Starting Go Trading Bot..."
	@go run $(MAIN_FILE) --config $(CONFIG_FILE)

# Protobuf generation
proto:
	@echo "==> Generating protobuf files..."
	@protoc -I=../proto --go_out=gen/go --go_opt=paths=source_relative --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative ../proto/v1/exchange.proto


# Pre-commit hooks
install-tools:
	@echo "==> Installing goimports..."
	@go install golang.org/x/tools/cmd/goimports@latest

pre-commit: install-tools
	@echo "==> Running pre-commit hooks..."
	@cd ../.. && pre-commit run --all-files

# Testing targets
test:
	@echo "==> Running unit tests..."
	@go test -v ./...

test-integration:
	@echo "==> Running integration tests (requires running DB and Gateway)..."
	@go test -v -tags=integration -count=1 ./...

test-all:
	@$(MAKE) test
	@echo "\n==> Starting test environment (DB + Gateway)..."
	@$(COMPOSE_CMD) up --build -d python-gateway test-db
	@echo "==> Waiting for services..."
	@sleep 5
	@$(MAKE) test-integration
	@echo "==> Tearing down environment..."
	@$(COMPOSE_CMD) down

db-up:
	@echo "==> Starting test database..."
	@$(COMPOSE_CMD) up -d test-db

db-down:
	@echo "==> Tearing down test database..."
	@$(COMPOSE_CMD) down

# Database Migrations
install-migrate:
	@echo "==> Installing golang-migrate..."
	@go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

migrate-up:
	@echo "==> Applying database migrations..."
	@migrate -database "$(DB_URL)" -path $(MIGRATIONS_PATH) up

migrate-down:
	@echo "==> Reverting last database migration..."
	@migrate -database "$(DB_URL)" -path $(MIGRATIONS_PATH) down 1

migrate-create:
	@echo "==> Creating new migration files..."
	@read -p "Enter migration name: " name; \
	migrate create -ext sql -dir $(MIGRATIONS_PATH) -seq $$name

migrate-version:
	@echo "==> Checking current migration version..."
	@migrate -database "$(DB_URL)" -path $(MIGRATIONS_PATH) version

db-drop-all:
	@echo "==> Dropping all database tables..."
	@psql "$(DB_URL)" -c "DROP SCHEMA IF EXISTS trading CASCADE; DROP TABLE IF EXISTS schema_migrations;"

debug-db:
	@echo "DB_HOST: $(DB_HOST)"
	@echo "DB_USER: $(DB_USER)"
	@echo "DB_NAME: $(DB_NAME)"
	@echo "DB_URL: $(DB_URL)"
