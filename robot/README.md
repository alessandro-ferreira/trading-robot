# Live Trading Robot

This directory contains the live trading bot, designed with a microservices architecture for modularity, performance, and scalability.

## Architecture

The system is composed of two main services that communicate via gRPC. The API contract is defined in `proto/v1/exchange.proto`.

1.  **Go Bot (`go-bot`)**: This is the core of the system, containing the trading logic, strategy execution, risk management, and state persistence. It connects to a TimescaleDB database to store historical data and trade information. It acts as the gRPC client, sending commands to the Python Gateway.

2.  **Python Gateway (`python-gateway`)**: This service acts as a bridge to the cryptocurrency exchange. It receives gRPC requests from the Go bot, translates them into API calls for the exchange (using the `ccxt` library), and sends the responses back.

For a more detailed diagram, see `ARCHITECTURE.md`.

## Project Directory Structure

```
robot/
├── ARCHITECTURE.md             # This file
├── config.toml.example         # Example configuration for the services
├── docker-compose.yml          # Docker Compose for integration tests
│
├── go-bot/                     # The core Go application
│   ├── go.mod
│   ├── Makefile                # Automates common tasks (testing, etc.)
│   ├── cmd/server/
│   │   └── main.go             # Initializes and runs the Go components
│   ├── gen/go/v1/              # Auto-generated Go gRPC code
│   ├── internal/               # All internal Go packages
|   │   ├── background/         # Background tasks
│   │   ├── components/
│   │   │   ├── execution/      # Logic for trade execution via gRPC
│   │   │   ├── monitor/        # Periodic health checks
│   │   │   ├── portfolio/      # Portfolio management
│   │   │   └── risk/           # Risk management
│   │   ├── config/             # Configuration loading
│   │   ├── database/           # Database connection and access logic
│   │       └── repository      # Data access layer (Repository Pattern)
│   │   ├── logger/             # Structured logging setup
│   │   └── strategy/           # Trading strategy logic
│   │       └── core/           # C++ logic called via cgo
│   └── migrations              # Database migrations
│
├── python-gateway/             # The Python Exchange Gateway service
│   ├── Dockerfile              # Docker build instructions
│   ├── Makefile                # Automates common tasks
│   ├── main.py                 # Starts the Python gRPC server
│   ├── requirements.txt        # Python dependencies
│   ├── core/                   # Core application helpers
│   └── exchange/
│       ├── exchanges/          # Supported exchanges
│       ├── factory.py          # Logic to select exchange based on config
│       └── service.py          # Implements the gRPC service
│   ├── tests/                  # Service tests
│   └── v1/                     # Auto-generated Python gRPC code
│
└── proto/                      # Shared gRPC definitions
    └── v1/
        └── exchange.proto      # Defines services and messages
```

## Development Environment

To ensure consistency all services are built and run against specific versions.

-   **Python:** 3.12 (as defined in `python-gateway/Dockerfile`)
-   **Go:** 1.24 (as defined in `go-bot/go.mod`)

It is highly recommended to use these versions for local development. The `python-gateway/Dockerfile` should be pinned to a `python:3.12-slim` base image.

## Setup and Running

A global configuration file is shared between the services. Create it from the template:

```bash
cp config.toml.example config.toml
```

Now, edit `config.toml` and fill in your database credentials and exchange API keys.

To set up and run the trading bot, please follow the instructions in the `README.md` file for each service. You will need to start both services in separate terminals.


1.  **Start the Python Gateway first:**
    -   Instructions in `python-gateway/README.md`
2.  **Then, start the Go Bot:**
    -   Instructions in `go-bot/README.md`
