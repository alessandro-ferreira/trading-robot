# Trading Analysis and Automation Suite

This repository contains a suite of tools for financial market analysis and automated trading.

## Project Structure

The project is organized into two main components:

-   `./data_analysis/`: A Python application for running historical simulations (backtests) of trading strategies.
-   `./robot/`: A live trading bot built with a microservices architecture. It consists of a Go application for the core logic and a Python gateway to communicate with exchange APIs via gRPC.

For more information and setup instructions, please see the `README.md` file within each respective directory.

## Code Quality

This project uses `pre-commit` to enforce coding standards and automatically fix issues (linting, formatting) across both Python and Go codebases.

### Setup

1.  Install `pre-commit`:
    ```bash
    pip install pre-commit
    ```
2.  Install the git hooks (run from the repository root):
    ```bash
    pre-commit install
    ```
