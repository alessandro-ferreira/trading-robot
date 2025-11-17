# Trading Analysis and Automation Suite

This repository contains a comprehensive suite of tools for backtesting trading strategies and deploying them in a live, automated environment.

## Project Structure

-   **/data_analysis**: A Python application for running historical simulations (backtests) of trading strategies.
-   **/robot**: A live trading bot built with a microservices architecture. It consists of a Go application for the core logic and a Python gateway to communicate with exchange APIs via gRPC.

---

## Prerequisites

Before you begin, ensure you have the following installed on your system:

-   [Python 3.8+](https://www.python.org/)
-   [Go 1.18+](https://go.dev/)
-   [Protobuf Compiler (protoc)](https://grpc.io/docs/protoc-installation/)

## Getting Started

This guide will walk you through setting up the development environment from a clean state. All commands should be run from the project's root directory (`~/Documents/Codes/trading`) unless specified otherwise.

### 1. Clone the Repository

```bash
git clone <your-repository-url>
cd trading
```

### 2. Python Virtual Environment

Create and activate a Python virtual environment. This keeps your project dependencies isolated.

```bash
# Create the virtual environment
python3 -m venv <path_to_your_venvs>/venv_trading

# Activate the environment
source <path_to_your_venvs>/venv_trading/bin/activate
```

### 3. Setup Python Dependencies

This project uses `pip-tools` to manage Python dependencies.

#### Step 3.1: Install pip-tools

```bash
pip install pip-tools
```

#### Step 3.2: Compile `requirements.txt` Files

Use `pip-compile` to generate the `requirements.txt` files from the `requirements.in` files. This locks the versions of all dependencies, creating a reproducible environment.

```bash
# Compile dependencies for the data_analysis component
pip-compile data_analysis/requirements.in

# Compile dependencies for the robot's python-gateway
pip-compile robot/python-gateway/requirements.in
```

#### Step 3.3: Install All Python Dependencies

```bash
pip install -r data_analysis/requirements.txt
pip install -r robot/python-gateway/requirements.txt
```

### 4. Setup Go Service

Initialize the Go module for the `go-bot` service.

```bash
# Navigate to the go-bot directory
cd robot/go-bot/

# Initialize the module.
go mod init trading/robot/go-bot

# Download and clean up dependencies
go mod tidy

# Return to the project root
cd ../..
```

### 5. Generate gRPC Code

The Go and Python services communicate via gRPC. We will generate the code for each service separately, which makes the process clearer and easier to debug.

#### Step 5.1: Generate Python Code

This command generates the necessary server-side code for the `python-gateway`.

```bash
# From the project root directory
python -m grpc_tools.protoc -I=robot/proto \
    --python_out=robot/python-gateway \
    --pyi_out=robot/python-gateway \
    --grpc_python_out=robot/python-gateway \
    robot/proto/v1/exchange.proto
```

#### Step 5.2: Generate Go Code

First, ensure the Go gRPC plugins for `protoc` are installed. These plugins are used by the compiler to generate Go-specific client code.

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
```

This command generates the necessary client-side code for the `go-bot`.

```bash
# From the project root directory
mkdir -p robot/go-bot/gen/go

python -m grpc_tools.protoc -I=robot/proto \
    --go_out=robot/go-bot/gen/go --go_opt=paths=source_relative \
    --go-grpc_out=robot/go-bot/gen/go --go-grpc_opt=paths=source_relative \
    robot/proto/v1/exchange.proto
```

### 6. Create Python Package Initializers

To ensure Python's import system can find all modules correctly, create the necessary `__init__.py` files.

```bash
# From the project root directory
touch robot/python-gateway/__init__.py
touch robot/python-gateway/v1/__init__.py
touch robot/python-gateway/exchange/__init__.py
```

### 7. Verifying the Setup

To confirm that everything is working, run the Python server and the Go client. This requires two separate terminals.

**Terminal 1: Start the Python gRPC Server**

To ensure all imports work correctly, run the Python application as a module from the project root.

```bash
# From the project root directory
python -robot/python-gateway/main.py
```

**Terminal 2: Run the Go gRPC Client**

Navigate to the Go module's root directory to run the client.

```bash
# From the project root directory (~/Documents/Codes/trading)
cd robot/go-bot/

# Run the main application
go run ./cmd/server/main.go
```

If successful, the Go client will print a "✅ Success!" message, and the Python server will log that it received a request. This confirms the communication channel is working.
