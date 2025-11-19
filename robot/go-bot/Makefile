# Makefile for the Go Trading Bot

# Use the modern 'docker compose' command. The -f flag points to the root docker-compose.yml
COMPOSE_CMD = docker compose -f ../../docker-compose.yml

.PHONY: help
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  help             Show this help message."
	@echo "  test             Run all unit tests (excluding integration tests)."
	@echo "  test-integration Run all tests, including integration tests (requires a running database)."
	@echo "  test-all         Start the test database, run all tests, and then stop the database."
	@echo "  db-up            Start the test database in the background."
	@echo "  db-down          Stop and remove the test database."

.PHONY: test
test:
	@echo "==> Running unit tests..."
	@go test ./...

.PHONY: test-integration
test-integration:
	@echo "==> Running all tests (including integration)..."
	@go test -v -tags=integration ./...

.PHONY: test-all
test-all: db-up
	@echo "==> Waiting for database to be ready..."
	@sleep 5
	@$(MAKE) test-integration
	@$(MAKE) db-down

.PHONY: db-up
db-up:
	@echo "==> Starting test database..."
	@$(COMPOSE_CMD) up -d test-db

.PHONY: db-down
db-down:
	@echo "==> Tearing down test database..."
	@$(COMPOSE_CMD) down
