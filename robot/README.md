# Live Trading Robot

This directory contains the live trading bot, designed with a microservices architecture for modularity and scalability.

## Architecture

The system is composed of two main services that communicate via gRPC. The API contract is defined in `proto/v1/exchange.proto`.

1.  **Python Gateway (`python-gateway`)**: This service acts as a bridge to the cryptocurrency exchange. It receives gRPC requests from the Go bot, translates them into REST API or WebSocket calls for the exchange (using the `ccxt` library), and sends the responses back.

2.  **Go Bot (`go-bot`)**: This is the core of the system. It contains the trading logic, strategy execution, risk management, and state persistence. It connects to a TimescaleDB database to store historical data and trade information. It acts as the gRPC client, sending commands to the Python Gateway.

For a more detailed diagram, see `ARCHITECTURE.md`.

## Components

- `go-bot/`: Source code for the core trading logic application.
- `python-gateway/`: Source code for the exchange communication gateway.
- `proto/`: Contains the Protocol Buffer (`.proto`) definitions for the gRPC services.
- `gen/`: **(Generated)** Will contain the gRPC code generated from the `.proto` files for both Go and Python.
- `config.toml.example`: An example configuration file.

## Setup

### 1. Configuration

Before running the services, you must create your own configuration file.

```bash
cp config.toml.example config.toml
```

Now, edit `config.toml` and fill in your database credentials and exchange API keys.

**Important**: The `config.toml` file contains secrets and should be added to your `.gitignore` to prevent it from being committed to version control.

### 2. Generate gRPC Code

The Go and Python code for the gRPC services must be generated from the `.proto` file. This is a required step before compiling or running the services.
