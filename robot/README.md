# Live Trading Robot

This directory contains the live trading bot, designed with a microservices architecture for modularity, performance, and scalability.

## Architecture

The system is composed of two main services that communicate via gRPC. The API contract is defined in `proto/v1/exchange.proto`.

1.  **Go Bot (`go-bot`)**: This is the core of the system, containing the trading logic, strategy execution, risk management, and state persistence. It connects to a TimescaleDB database to store historical data and trade information. It acts as the gRPC client, sending commands to the Python Gateway.

2.  **Python Gateway (`python-gateway`)**: This service acts as a bridge to the cryptocurrency exchange. It receives gRPC requests from the Go bot, translates them into API calls for the exchange (using the `ccxt` library), and sends the responses back.

For a more detailed diagram, see `ARCHITECTURE.md`.

## Development Environment

To ensure consistency all services are built and run against specific versions.

-   **Python:** 3.12 (as defined in `python-gateway/Dockerfile`)
-   **Go:** 1.18+ (as defined in `go-bot/go.mod`)

It is highly recommended to use these versions for local development. The `python-gateway/Dockerfile` should be pinned to a `python:3.12-slim` base image.

## Setup and Running

To set up and run the trading bot, please follow the instructions in the `README.md` file for each service. You will need to start both services in separate terminals.

1.  **Start the Python Gateway first:**
    -   Instructions in `python-gateway/README.md`
2.  **Then, start the Go Bot:**
    -   Instructions in `go-bot/README.md`

A global configuration file is shared between the services. Create it from the template:

```bash
cp config.toml.example config.toml
```

Now, edit `config.toml` and fill in your database credentials and exchange API keys.

**Important**: The `config.toml` file contains secrets and should be added to your `.gitignore` to prevent it from being committed to version control.

### 2. Generate gRPC Code

The Go and Python code for the gRPC services must be generated from the `.proto` file. This is a required step before compiling or running the services.
