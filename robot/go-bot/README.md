# Go Bot

This service is the core of the trading robot, containing the primary business logic, strategy execution, and state management. It acts as a gRPC client, sending commands to the `python-gateway`.

## Summary

- [Go Bot](#go-bot)
- [Summary](#summary)
- [Folder Structure](#folder-structure)
- [Getting Started](#getting-started)
  - [1. Prerequisites](#1-prerequisites)
  - [2. Installation](#2-installation)
  - [3. Generate gRPC Code](#3-generate-grpc-code)
  - [4. Run the Service](#4-run-the-service)
  - [5. Testing](#5-testing)
  - [6. Code Quality](#6-code-quality)

## Folder Structure

```
.
├── go-bot/                       # The core Go application
│   ├── go.mod
│   ├── Makefile                  # Automates common tasks (testing, etc.)
│   ├── cmd/server/
│   │   └── main.go               # Initializes and runs the Go components
│   ├── gen/go/v1/                # Auto-generated Go gRPC code
│   ├── internal/                 # All internal Go packages
|   │   ├── api                   # gRPC services
|   │   ├── background/           # Background tasks
│   │   ├── components/
│   │   │   ├── execution/        # Logic for trade execution via gRPC
│   │   │   ├── health/           # Periodic health checks
│   │   │   ├── portfolio/        # Portfolio management
│   │   │   ├── reconciliation/   # Reconciliation tasks
│   │   │   ├── risk/             # Risk management
│   │   │   └── signal_generator/ # Drives the strategy to generate signals
│   │   ├── config/               # Configuration loading
│   │   ├── database/             # Database connection and access logic
│   │       └── repository        # Data access layer (Repository Pattern)
│   │   ├── logger/               # Structured logging setup
│   │   ├── orchestrator/         # Trading loop orchestrator
│   │   └── strategy/             # Trading strategy logic
│   └── migrations                # Database migrations
```

## Getting Started

This guide provides the necessary steps to set up and run the `go-bot` service.

### 1. Prerequisites

-   Go 1.24 (as defined in `go.mod`)
-   PostgreSQL 16 + TimescaleDB (Local installation)
-   Protobuf Compiler (protoc)
-   Docker & Docker Compose (for database and integration tests)

### 2. Installation

Navigate to this directory (`robot/go-bot`) and run `go mod tidy` to download and install the required Go packages.

```bash
# From the go-bot/ directory
go mod tidy
```

### 3. Generate gRPC Code

The gRPC client code must be generated from the `.proto` definitions.

First, ensure the Go gRPC plugins for `protoc` are installed:

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
```

Now, use the provided `Makefile` to generate the code:

```bash
# From the go-bot/ directory
make proto
```

### 4. Database Setup

Ensure your local PostgreSQL 16 instance is running. You need to create the database and user expected by the default configuration (`config.toml`).

```sql
-- Run these commands in psql
CREATE DATABASE trading_db;
```

**Environment Variables:**

The `Makefile` uses default credentials (`postgres`/`postgres`) to run migrations. If your local database uses different credentials, create a `.env` file in this directory (`robot/go-bot/.env`) to override them.

Example `.env`:
```dotenv
DB_USER=your_user
DB_PASSWORD=your_password
```

Once the database is ready and configured, apply the schema migrations:

```bash
make migrate-up
```


### 5. Run the Service

Use the provided `Makefile` to start the application. This will attempt to connect to the `python-gateway`, so ensure it is running first.

```bash
# From the go-bot/ directory
make run
```

This will start the application, which will then attempt to connect to the `python-gateway`.


### 6. Testing

The project includes both unit and integration tests. The `Makefile` provides convenient targets for running them.

#### Running All Tests (Recommended)

The `test-all` target automates the entire process: it starts the test database, runs all unit and integration tests, and then tears down the database. This is the simplest way to run the full test suite.

```bash
# From the go-bot/ directory
make test-all
```

### 6. Code Quality

This project uses `pre-commit` with `golangci-lint` and `go-fmt` to enforce coding standards. The configuration is located at the repository root.

To set up the automatic git hooks:

```bash
pip install pre-commit
pre-commit install --hook-type pre-commit --hook-type commit-msg
```

To run it manually:

```bash
pre-commit run --all-files
```
