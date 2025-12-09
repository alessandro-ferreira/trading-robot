# Makefile for the Go Trading Bot
MAIN_FILE := ./cmd/server/main.go
CONFIG_FILE := ../config.toml

# Use the modern 'docker compose' command. The -f flag points to the root docker-compose.yml
COMPOSE_CMD = docker compose -f ../../docker-compose.yml

.PHONY: help test test-integration test-all db-up db-down

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  help             Show this help message."
	@echo "  run              Start the Go Trading Bot."
	@echo "  test             Run all unit tests (excluding integration tests)."
	@echo "  test-integration Run all tests, including integration tests (requires a running database)."
	@echo "  test-all         Start the test database, run all tests, and then stop the database."
	@echo "  db-up            Start the test database in the background."
	@echo "  db-down          Stop and remove the test database."

run: 
	@echo "==> Starting Go Trading Bot..."
	@go run $(MAIN_FILE) --config $(CONFIG_FILE)

test:
	@echo "==> Running unit tests..."
	@go test -v ./...

test-integration:
	@echo "==> Running all tests (including integration)..."
	@go test -v -tags=integration ./...

test-all: db-up
	@echo "==> Waiting for database to be ready..."
	@sleep 5
	@$(MAKE) test-integration
	@$(MAKE) db-down

db-up:
	@echo "==> Starting test database..."
	@$(COMPOSE_CMD) up -d test-db

db-down:
	@echo "==> Tearing down test database..."
	@$(COMPOSE_CMD) down
