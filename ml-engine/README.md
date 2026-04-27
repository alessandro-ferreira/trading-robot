# ML Engine

This component is the Machine Learning (ML) engine of the trading bot. It acts as a service responsible for running intelligence models and communicating strategic decisions to the robot via the Management gRPC API.

Note that the actual Machine Learning implementation is proprietary and is not available in this public repository; only the reference template is provided.

## Summary

- [ML Engine](#ml-engine)
- [Summary](#summary)
- [Folder Structure](#folder-structure)
- [Getting Started](#getting-started)
  - [1. Prerequisites](#1-prerequisites)
  - [2. Generate gRPC Code](#2-generate-grpc-code)
  - [3. Build the Service](#3-build-the-service)
  - [4. Design Philosophy](#4-design-philosophy)

## Folder Structure

```
.
├── ml-engine/                      # The C++ Machine Learning Engine
│   ├── Makefile                    # Automates build tasks
│   ├── main.cpp                    # Application entry point
│   ├── build/                      # Build artifacts and generated proto code
│   ├── include/                    # Public header files
│   │   └── IEngine.hpp             # Abstract engine interface
│   ├── src/                        # Infrastructure implementation
│   │   └── management_client.cpp   # gRPC client logic
│   ├── template/                   # Public example engine implementation
│   └── tests/                      # Unit tests
```

## Getting Started

### 1. Prerequisites

-   **C++ Compiler:** GCC or Clang with C++17 support.
-   **Make:** For build automation
-   **Coverage Tools:** `lcov` and `gcovr` (required for `make coverage`)
-   **gRPC & Protobuf:** Development libraries for C++.
    ```bash
    make install-deps
    ```

### 2. Generate gRPC Code

Before building the engine, generate the C++ stubs from the shared `.proto` definitions:

```bash
make proto
```

### 3. Build the Service

Compile the source code into the `engine` executable:

```bash
make
```

### 4. Design Philosophy

The engine is built around the `IEngine` interface to support a hybrid public/private development model:

-   **Public Infrastructure:** The gRPC client and interface are public, allowing anyone to understand how signals are communicated.
-   **Pluggable Brains:** The engine can load the built-in `template` implementation or more sophisticated custom logic.
