# Trading Automation Suite

This repository contains a suite of tools for automated trading.

## Project Structure

The repository is structured around a central trading system supported by specialized auxiliary tools:

### Main Component
- `robot`: The primary live trading system. It integrates a high-concurrency Go engine for orchestration and risk enforcement, a Python-based exchange gateway, and a high-performance C++ strategy core.

### Auxiliary Tools
- `ml-engine`: A C++ service for machine learning inference, optimizing trading models and communicating strategic insights to the robot via the Management gRPC API.

For more information and setup instructions, please see the `README.md` file within each respective directory.

## Code Quality

This project uses `pre-commit` to enforce coding standards and automatically fix issues (linting, formatting) across Go, Python and C++ codebases.

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
