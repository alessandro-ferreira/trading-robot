# Live Trading Robot

This directory contains the live trading bot, designed with a modular architecture for performance and scalability.

## Architecture

The system is composed of three main components.

1.  **Go Bot (`go-bot`)**: The core application responsible for orchestration, risk management, and execution. It acts as a gRPC client to the `python-gateway` and integrates with the `strategy-core` via `cgo` for high-performance signal generation.

2.  **Python Gateway (`python-gateway`)**: A gRPC server that acts as a bridge to the cryptocurrency exchange. It translates requests from the `go-bot` into API calls for the exchange.

3.  **C++ Strategy Core (`strategy-core`)**: A high-performance static library containing the core trading logic. It processes market data and generates trading signals (buy/sell/hold), which are then consumed by the `go-bot`.

The `go-bot` and `python-gateway` communicate via gRPC, with the API contract defined in `proto/v1/exchange.proto`.

For a more detailed diagram, see `ARCHITECTURE.md`.

## Project Directory Structure

```
robot/
├── ARCHITECTURE.md                 # This file
├── config.toml.example             # Example configuration for the services
├── docker-compose.yml              # Docker Compose for integration tests
│
├── go-bot/                         # The core Go application
│   ├── go.mod
│   ├── Makefile                    # Automates common tasks (testing, etc.)
│   ├── cmd/server/
│   │   └── main.go                 # Initializes and runs the Go components
│   ├── gen/go/v1/                  # Auto-generated Go gRPC code
│   ├── internal/                   # All internal Go packages
|   │   ├── background/             # Background tasks
│   │   ├── components/
│   │   │   ├── execution/          # Logic for trade execution via gRPC
│   │   │   ├── health/             # Periodic health checks
│   │   │   ├── portfolio/          # Portfolio management
│   │   │   ├── risk/               # Risk management
│   │   │   └── signal_generator/   # Drives the strategy to generate signals
│   │   ├── config/                 # Configuration loading
│   │   ├── database/               # Database connection and access logic
│   │       └── repository          # Data access layer (Repository Pattern)
│   │   ├── logger/                 # Structured logging setup
│   │   ├── orchestrator/           # Trading loop orchestrator
│   │   └── strategy/               # Trading strategy logic
│   └── migrations                  # Database migrations
│
├── python-gateway/                 # The Python Exchange Gateway service
│   ├── Dockerfile                  # Docker build instructions
│   ├── Makefile                    # Automates common tasks
│   ├── main.py                     # Starts the Python gRPC server
│   ├── requirements.txt            # Python dependencies
│   ├── core/                       # Core application helpers
│   └── exchange/
│       ├── exchanges/              # Supported exchanges
│       ├── factory.py              # Logic to select exchange based on config
│       └── service.py              # Implements the gRPC service
│   ├── tests/                      # Service tests
│   └── v1/                         # Auto-generated Python gRPC code
│
├── strategy-core/                  # The C++ Strategy Engine
│   ├── Makefile                    # Automates build tasks
│   ├── include/                    # Public header files
│   │   └── trading/
│   │       └── interfaces/         # Abstract component interfaces
│   └── src/
│   │   ├── api.cpp                 # C-API implementation
│   │   ├── strategy.cpp            # Core strategy orchestration logic
│   │   ├── rules/                  # Rule implementation sources
│   │   └── state/                  # State implementation sources
|   └── tests/                      # Unit tests
│
└── proto/                          # Shared gRPC definitions
    └── v1/
        └── exchange.proto          # Defines services and messages
```

## Development Environment

To ensure consistency all services are built and run against specific versions.

-   **Python:** 3.12 (as defined in `python-gateway/Dockerfile`)
-   **Go:** 1.24 (as defined in `go-bot/go.mod`)
-   **C++:** GCC or Clang with C++17 support (as defined in `strategy-core/Makefile`)

It is highly recommended to use these versions for local development. The `python-gateway/Dockerfile` should be pinned to a `python:3.12-slim` base image.

## Setup and Running

A global configuration file is shared between the services. Create it from the template:

```bash
cp config.toml.example config.toml
```

Now, edit `config.toml` and fill in your database credentials and exchange API keys.

To set up and run the trading bot, please follow the instructions in the `README.md` file for each service. You will need to start both services in separate terminals.


1.  **Build Strategy Core Library:**
    -   Instructions in `strategy-core/README.md`
2.  **Start the Python Gateway:**
    -   Instructions in `python-gateway/README.md`
3.  **Then, start the Go Bot:**
    -   Instructions in `go-bot/README.md`
