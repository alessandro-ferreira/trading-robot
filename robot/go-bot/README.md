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

## Folder Structure

```
.
├── Makefile                # Automates common tasks like running and testing.
├── cmd/                    # Application entry points.
│   └── server/
│       └── main.go         # Initializes and runs the Go components.
├── gen/                    # Auto-generated Go gRPC code.
│   └── go/v1/
├── go.mod                  # Go module file for managing dependencies.
├── go.sum                  # Go checksum file for dependencies.
├── internal/               # All internal application code.
│   ├── components/         # Core business logic components.
│   │   └── execution/      # Logic for trade execution via gRPC.
│   ├── config/             # Configuration loading and parsing.
│   ├── database/           # Database connection and access logic.
│   └── logger/             # Structured logging setup.
```

## Getting Started

This guide provides the necessary steps to set up and run the `go-bot` service.

### 1. Prerequisites

-   Go 1.18+
-   Protobuf Compiler (protoc)

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

Now, run the following command from the project's **root directory** (`trading/`) to generate the code:

```bash
protoc -I=robot/proto --go_out=robot/go-bot/gen/go --go_opt=paths=source_relative --go-grpc_out=robot/go-bot/gen/go --go-grpc_opt=paths=source_relative robot/proto/v1/exchange.proto
```

### 4. Run the Service

Use the provided `Makefile` to start the application. This will attempt to connect to the `python-gateway`, so ensure it is running first.

```bash
# From the go-bot/ directory
make run
```

This will start the application, which will then attempt to connect to the `python-gateway`.


### 5. Testing

The project includes both unit and integration tests. The `Makefile` provides convenient targets for running them.

#### Running All Tests (Recommended)

The `test-all` target automates the entire process: it starts the test database, runs all unit and integration tests, and then tears down the database. This is the simplest way to run the full test suite.

```bash
# From the go-bot/ directory
make test-all
```
