# Makefile for the Go Trading Bot
-include .env

MAIN_PACKAGE := ./cmd/server
CONFIG_FILE := ../config.toml
BACKTEST_CONFIG_FILE := ../config.backtest.toml
STRATEGY_CORE_DIR := ../strategy-core

# Use the modern 'docker compose' command. The -f flag points to the root docker-compose.yml
COMPOSE_CMD = docker compose -f ../docker-compose.yml
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

# Protobuf definitions
PROTO_DIR := ../proto
PROTO_FILES := $(PROTO_DIR)/v1/exchange.proto $(PROTO_DIR)/v1/management.proto

.PHONY: help run proto install-tools pre-commit
.PHONY: strategy-core clean-strategy
.PHONY: coverage test test-integration test-all backtest
.PHONY: install-migrate migrate-up migrate-down migrate-create migrate-force migrate-version
.PHONY: db-drop-all debug-db

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  help             Show this help message."
	@echo "  run              Start the Go Trading Bot."
	@echo "  strategy-core    Build the C++ Strategy Core static library."
	@echo "  clean-strategy   Clean the C++ Strategy Core static library."
	@echo "  proto            Generate protobuf files from .proto definitions."
	@echo "  install-tools    Install development tools."
	@echo "  pre-commit       Run pre-commit hooks (requires pre-commit installed)."
	@echo "  coverage         Run code coverage tests."
	@echo "  test             Run all unit tests (excluding integration tests)."
	@echo "  test-integration Run integration tests (requires running DB and Gateway)."
	@echo "  test-all         Run all unit and integration tests, managing the test environment."
	@echo "  backtest         Run a backtest using the Go Trading Bot with the backtest configuration."
	@echo "  install-migrate  Install the golang-migrate CLI tool."
	@echo "  migrate-up       Apply all database migrations."
	@echo "  migrate-down     Revert the last database migration."
	@echo "  migrate-create   Create a new migration file."
	@echo "  migrate-force    Force the database to a specific migration version (fixes dirty state)."
	@echo "  migrate-version  Show the current migration version."
	@echo "  db-drop-all      Drop all database tables (clean slate)."
	@echo "  debug-db         Show database connection info (for debugging)."

# Run the Go Trading Bot
run: strategy-core
	@echo "==> Starting Go Trading Bot..."
# use go run -a to force recompilation of all packages, ensuring the cgo C++ library is up-to-date
	@go run $(MAIN_PACKAGE) --config $(CONFIG_FILE)

# Strategy Core
strategy-core:
	@echo "==> Building C++ Strategy Core..."
	@$(MAKE) -C $(STRATEGY_CORE_DIR)

clean-strategy:
	@echo "==> Cleaning C++ Strategy Core..."
	@$(MAKE) -C $(STRATEGY_CORE_DIR) clean

# Protobuf generation
proto:
	@echo "==> Generating protobuf files..."
	@protoc -I=$(PROTO_DIR) --go_out=gen/go --go_opt=paths=source_relative --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative $(PROTO_FILES)


# Pre-commit hooks
install-tools:
	@echo "==> Installing goimports..."
	@go install golang.org/x/tools/cmd/goimports@latest

pre-commit: install-tools
	@echo "==> Running pre-commit hooks..."
	@cd ../.. && pre-commit run --all-files

# Testing targets
coverage:
	@echo "==> Running tests with coverage..."
	@go test -tags=unit -timeout 30s -coverprofile=coverage.out ./...
	@rm -f coverage.out

test: strategy-core
	@echo "==> Running unit tests..."
	@go test -v -tags=unit ./...

test-integration: strategy-core
	@echo "\n==> Starting test environment (DB + Gateway)..."
	@$(COMPOSE_CMD) up --build -d python-gateway test-db
	@echo "==> Waiting for services..."
	@sleep 3
	@echo "==> Waiting for database to be ready..."
	@echo "==> Applying migrations to test database..."
	@$(COMPOSE_CMD) exec -T test-db psql -U testuser -d testdb -c "DROP SCHEMA IF EXISTS trading CASCADE;" > /dev/null 2>&1
	@for migration_file in $$(find $(MIGRATIONS_PATH) -name "*.up.sql" | sort); do \
 		$(COMPOSE_CMD) exec -T test-db psql -U testuser -d testdb -f "$$migration_file" > /dev/null; \
 	done
	@echo "==> Running integration tests (requires running DB and Gateway)..."
# use -count=1 to disable test caching and -p 1 to prevent concurrent database conflicts (orders and balance)
	@go test -v -tags=integration -count=1 -p 1 ./...
	@echo "==> Tearing down environment..."
	@$(COMPOSE_CMD) down

test-all: strategy-core
	@echo "==> Running unit tests..."
	@$(MAKE) test
	@echo "==> Running integration tests..."
	@$(MAKE) test-integration

backtest: strategy-core
	@echo "==> Running backtest..."
	@go run $(MAIN_PACKAGE) --config $(BACKTEST_CONFIG_FILE)

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

migrate-force:
	@echo "==> Forcing database version..."
	@read -p "Enter version to force: " version; \
	migrate -database "$(DB_URL)" -path $(MIGRATIONS_PATH) force $$version

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
