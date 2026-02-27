# Trading Bot Project Architecture

This document outlines the architecture for the trading bot. It serves as a live reference that will be updated as the project evolves.

## 1. Core Philosophy

The system is designed as a set of decoupled projects to leverage the best technologies for each specific task, prioritizing performance, reliability, and maintainability.

-   **Go (`go-bot`)**: For the core, high-concurrency application logic, orchestration, and state management.
-   **Python (`python-gateway`)**: To provide a flexible bridge to various exchanges, leveraging a factory pattern to select the appropriate connector (e.g., `ccxt` or custom implementations).
-   **C++ (`strategy-core`)**: For a high-performance, extensible strategy engine, designed with a component-based architecture that can be easily adapted and tested.

## 2. Technology Stack

-   **Primary Language (Core Bot):** Go
-   **Exchange Gateway:** Python
-   **Performance-Critical Strategy Logic:** C++ (compiled as a static library, called from Go via `cgo`)
-   **Inter-Service Communication:** gRPC
-   **Database:** PostgreSQL with the TimescaleDB extension.
-   **Database Access:** Raw SQL queries via the `pgx` library in Go (No ORM).
-   **Configuration:** TOML

## 3. High-Level Architecture

The system is composed of three main projects:

1.  **`go-bot`**: The main application. It is the "brain" of the operation, containing all high-performance, concurrent logic. It handles portfolio management, risk management, and orchestrates the trading strategy. It is completely exchange-agnostic.

2.  **`python-gateway`**: A specialized Python server whose only job is to be an exchange gateway. It receives generic commands from the `go-bot` (e.g., "Create Order", "Stream Ticker") and translates them into specific API calls for the configured exchange.

3.  **`strategy-core`**: A standalone C++ library that contains the strategy logic. It is designed with a component-based architecture (MarketState, EntryRule, ExitRule) to be highly modular and testable. The Go bot uses a C-API factory to construct a specific strategy (e.g., "Momentum") by assembling these components.

### Component Interaction Flow

```
+----------------------------------------------+      (gRPC Request: "Place BUY Order")      +-----------------------------------+
|              Go Application                  | ------------------------------------------> |    Python Exchange Gateway        |
|              (The Core Bot)                  |                                             |                                   |
| +----------------+   +---------------------+ |      (gRPC Response: "Order Placed")        | +-------------------------------+ |
| |  Risk Manager  |-->|   Execution Engine  | | <------------------------------------------ | |    Exchange Connectors        | |
| +----------------+   |    (gRPC Client)    | |                                             | |  (Factory: ccxt, custom)      | |
|                      +---------------------+ |                                             | +----------------+--------------+ |
|                                              |                                             |                  |                |
| +------------------------------------------+ |                                             |                  v                |
| |         Signal Generator (Go)            | |                                             | +----------------+--------------+ |
| +------------------------------------------+ |                                             | | Exchange API (REST/WebSocket) | |
|                    |                         |                                             | +-------------------------------+ |
|                    | (cgo call)              |                                             |                                   |
|                    v                         |                                             |                                   |
+--------------------|-------------------------+                                             +-----------------------------------+
                     |
+--------------------|-------------------------+
|        C++ Strategy Engine (Library)         |
| +------------------------------------------+ |
| |  Strategy (State, Entry/Exit Rules)      | |
| +------------------------------------------+ |
+----------------------------------------------+
```
