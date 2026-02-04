# Makefile for the Go Trading Bot
MAIN_FILE := ./cmd/server/main.go
CONFIG_FILE := ../config.toml

# Use the modern 'docker compose' command. The -f flag points to the root docker-compose.yml
COMPOSE_CMD = docker compose -f ../../docker-compose.yml

.PHONY: help test test-integration test-all db-up db-down integration-e2e

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
	@echo "  integration-e2e  Run end-to-end integration tests (Go <-> Python gRPC, requires Docker)."

run: 
	@echo "==> Starting Go Trading Bot..."
	@go run $(MAIN_FILE) --config $(CONFIG_FILE)

proto:
	@echo "==> Generating protobuf files..."
	@protoc -I=../proto --go_out=gen/go --go_opt=paths=source_relative --go-grpc_out=gen/go --go-grpc_opt=paths=source_relative ../proto/v1/exchange.proto

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

# integration-e2e: Start python-gateway and test-db, run Go integration tests, then stop containers
integration-e2e:
	@echo "==> Starting python-gateway and test database for integration tests..."
	@$(COMPOSE_CMD) up -d python-gateway test-db
	@echo "==> Waiting for python-gateway to be ready..."
	@sleep 3
	@echo "==> Running Go integration tests (gRPC E2E)..."
	@go test -v -tags=integration ./...
	@echo "==> Stopping containers..."
	@$(COMPOSE_CMD) down
