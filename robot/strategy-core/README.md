# C++ Strategy Core

This component is the high-performance strategy engine of the trading bot. It is implemented as a standalone C++ library that encapsulates trading logic, signal generation, and market state management. It is designed to be linked into the `go-bot` application via `cgo`.

## Summary

- C++ Strategy Core
- Summary
- Folder Structure
- Getting Started
  - 1. Prerequisites
  - 2. Build the Library
  - 3. Integration
  - 4. Code Quality

## Folder Structure

```
.
├── strategy-core/              # The C++ Strategy Engine
│   ├── Makefile                # Automates build tasks
│   ├── include/                # Public header files
│   │   └── trading/
│   │       └── interfaces/     # Abstract component interfaces
│   └── src/
│   │   ├── api.cpp             # C-API implementation
│   │   ├── strategy.cpp        # Core strategy orchestration logic
│   │   ├── rules/              # Rule implementation sources
│   │   └── state/              # State implementation sources
|   └── tests/                  # Unit tests
```

## Getting Started

This guide provides the steps to build the strategy core library for development.

### 1. Prerequisites

-   **C++ Compiler:** GCC or Clang with C++20 support.
-   **Make:** For build automation.

### 2. Build the Library

Use the provided `Makefile` to compile the source code into a static library (`libstrategy.a`) or shared object, depending on the build configuration.
Make sure you have gtest installed `sudo apt-get install libgtest-dev`

```bash
# From the strategy-core/ directory
make
```

### 3. Integration

This library is integrated into the `go-bot` using `cgo`.

-   **API Definition:** The `api.cpp` file exposes a C-compatible interface.
-   **Linking:** The Go application includes the `include/` directory and links against the compiled library found in `strategy-core/`.

### 4. Code Quality

Code should adhere to modern C++ standards (C++20).

-   **Memory Management:** Use smart pointers (`std::unique_ptr`, `std::shared_ptr`) instead of raw pointers where possible.
-   **Modularity:** Implement new strategies by extending the `Strategy`, `EntryRule`, and `ExitRule` interfaces.
