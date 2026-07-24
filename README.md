# Trading Automation Suite

This repository contains a suite of tools for automated trading.

> **Status:** Active development.

The overall system architecture is documented in `robot/ARCHITECTURE.md`.

## Project Structure

The repository is structured around a central trading system supported by specialized auxiliary tools:

### Main Component
- `robot`: The primary live trading system. It integrates a high-concurrency Go engine for orchestration and risk enforcement, a Python-based exchange gateway, and a high-performance C++ strategy core.

### Auxiliary Tools
- `ml-engine`: A C++ service for machine learning inference, optimizing trading models and communicating strategic insights to the robot via the Management gRPC API.
- `simulator`: A standalone C++ backtesting tool that re-implements the production momentum strategy for historical validation and profit estimation using configurable parameters.

## Documentation

Each project contains its own documentation and setup instructions.

- `robot/README.md`
- `robot/ARCHITECTURE.md`
- `robot/go-bot/README.md`
- `robot/python-gateway/README.md`
- `robot/strategy-core/README.md`
- `ml-engine/README.md`
- `simulator/README.md`

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

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Disclaimer

This software is for educational, research, and portfolio purposes only. It is not financial advice, and it does not guarantee profitable trading. Cryptocurrency trading involves substantial risk of financial loss. The author accepts no responsibility or liability for any financial losses or damages incurred from using or modifying this software.
