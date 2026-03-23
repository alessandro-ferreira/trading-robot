# Trading Automation Suite

This repository contains a suite of tools for automated trading.

## Project Structure

The project is organized into a main component:
- `robot`: A live trading bot designed with a modular architecture for performance and scalability. It consists of a Go application for the core logic, a Python gateway to communicate with exchange APIs via gRPC, and a C++ core for high-performance strategy execution.

For more information and setup instructions, please see the `README.md` file within each respective directory.

## Code Quality

This project uses `pre-commit` to enforce coding standards and automatically fix issues (linting, formatting) across both Python and Go codebases.

### Setup

Follow these steps after setting up your Go, Python and C++ environments:

1.  Install `pre-commit`:
    ```bash
    pip install pre-commit
    ```
2.  Install the git hooks (run from the repository root):
    ```bash
    pre-commit install --hook-type pre-commit --hook-type commit-msg
    ```
