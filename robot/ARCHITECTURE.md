# Trading Bot Project Architecture

This document outlines the architecture for the trading bot. It serves as a live reference that will be updated as the project evolves.

## 1. Core Philosophy

The system is designed as a set of decoupled microservices to leverage the best technologies for each specific task, prioritizing performance, reliability, and maintainability.

- **Go (`go-bot`)**: For the core, high-concurrency application logic.
- **Python (`python-gateway`)**: To leverage the `ccxt` library's vast exchange support.
- **C++ (`core`)**: For maximum performance in the strategy's mathematical calculations.

## 2. Technology Stack

-   **Primary Language (Core Bot):** Go
-   **Exchange Gateway & Analysis:** Python
-   **Performance-Critical Strategy Logic:** C++ (compiled as a shared library, called from Go via `cgo`)
-   **Inter-Service Communication:** gRPC
-   **Database:** PostgreSQL with the TimescaleDB extension.
-   **Database Access:** Raw SQL queries via the `pgx` library in Go (No ORM).
-   **Configuration:** TOML

## 3. High-Level Architecture

The system is composed of two main microservices communicating via gRPC:

1.  **`go-bot`**: The main application. It is the "brain" of the operation, containing all high-performance, concurrent logic. It handles portfolio management, risk management, and orchestrates the trading strategy. It is completely exchange-agnostic.

2.  **`python-gateway`**: A specialized Python server whose only job is to be an exchange gateway. It receives generic commands from the `go-bot` (e.g., "Create Order", "Stream Ticker") and translates them into specific API calls for the configured exchange using the `ccxt` library.

### Component Interaction Flow

```
+-------------------------------------------------+      (gRPC Request: "Place BUY Order")      +--------------------------------+
|                Go Application                   | ------------------------------------------> |      Python Exchange Gateway   |
|               (The Core Bot)                    |                                             |                                |
| +-----------------+   +-----------------------+ |      (gRPC Response: "Order Placed")        | +----------------------------+ |
| |  Risk Manager   |-->|   Execution Engine    | | <------------------------------------------ | |         ccxt Library       | |
| +-----------------+   |      (gRPC Client)    | |                                             | +-------------+--------------+ |
|                       +-----------------------+ |                                             |               |                |
|                                                 |                                             |               v                |
| +---------------------------------------------+ |                                             | +-------------+--------------+ |
| |           Signal Generator (Go)             | |                                             | |Exchange API(REST/WebSocket)| |
| | (Calls C++ core, decides to buy/sell)       | |                                             | +----------------------------+ |
| +---------------------------------------------+ |                                             |                                |
+-------------------------------------------------+                                             +--------------------------------+
```

## 4. Project Directory Structure

```
robot/
├── ARCHITECTURE.md             # This file
├── .gitignore
├── config.toml.example         # Example configuration for the services
│
├── go-bot/                     # The core Go application
│   ├── go.mod
│   ├── Makefile                # Automates common tasks (testing, etc.)
│   ├── cmd/server/
│   │   └── main.go             # Initializes and runs the Go components
│   ├── internal/               # All internal Go packages
│   │   ├── components/
│   │   │   ├── execution/      # Logic for trade execution via gRPC
│   │   │   ├── portfolio/      # Portfolio management
│   │   │   └── risk/           # Risk management
│   │   ├── config/             # Configuration loading
│   │   ├── database/           # Database connection and access logic
│   │   ├── logger/             # Structured logging setup
│   │   └── strategy/           # Trading strategy logic
│   │       └── core/           # C++ logic called via cgo
│   ├── gen/go/v1/              # Auto-generated Go gRPC code
│   └── migrations              # Database migrations
│
├── python-gateway/             # The Python Exchange Gateway service
│   ├── Makefile                # Automates common tasks
│   ├── requirements.txt        # Python dependencies
│   ├── main.py                 # Starts the Python gRPC server
│   ├── core/                   # Core application helpers
│   └── exchange/
│       ├── factory.py          # Logic to select exchange based on config
│       └── service.py          # Implements the gRPC service
│   ├── tests/                  # Service tests
│   └── v1/                     # Auto-generated Python gRPC code
│
└── proto/                      # Shared gRPC definitions
    └── v1/
        └── exchange.proto      # Defines services and messages
```
