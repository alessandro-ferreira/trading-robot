# Strategy Simulator

This component is a standalone, sequential C++ backtesting tool that independently re-implements the momentum trading strategy used by `robot/strategy-core`. It serves two primary purposes:

1. **Production Logic Validation:** Validates the production logic against historical data by providing a "ground truth" to detect divergences from the production bot.
2. **Profit Estimation:** Provides a simple tool for estimating potential gains and losses for specific cryptocurrencies over defined periods using custom configurations.

Since the production strategy expects tick-level data and enforces lookback staleness limits, the simulator interpolates intermediate prices using an arithmetic progression to avoid false staleness failures when replaying hourly CSV data.

## Summary

- [Strategy Simulator](#strategy-simulator)
- [Summary](#summary)
- [Folder Structure](#folder-structure)
- [Getting Started](#getting-started)
  - [1. Prerequisites](#1-prerequisites)
  - [2. Build the Simulator](#2-build-the-simulator)
  - [3. Usage](#3-usage)
- [Design Philosophy](#design-philosophy)

## Folder Structure

```text
.
├── simulator/                      # The C++ Strategy Simulator
│   ├── Makefile                    # Automates build tasks
│   ├── simulator.cpp               # Application entry point
│   ├── build/                      # Build artifacts
│   └── prices/                     # Historical prices data
```

## Getting Started

### 1. Prerequisites

- **C++ Compiler:** GCC or Clang with C++17 support.
- **Make:** For build automation.

### 2. Build the Simulator

Compile the source code into the `simulator` executable:

```bash
make
```

### 3. Usage

The simulator is configured through command-line flags:

```bash
./build/simulator \
    --symbol BTC \
    --begin 2021-01 \
    --end 2021-12 \
    --type profit \
    --window 43201 \
    --momentum 3600:0.03,21600:0.04,43200:0.12 \
    --loss 0.22 \
    --profit 0.15
```

Run `./build/simulator --help` for a complete list of available options.

#### Input

By default, historical prices are read from:

```text
prices/<symbol>_prices.csv
```

During simulation, intermediate price points are generated automatically to emulate tick-level updates expected by the production strategy.

## Output

The simulator produces one CSV row for every completed trade.

Each row contains:

- entry timestamp
- entry date
- entry price
- exit timestamp
- exit date
- exit price
- realized profit/loss
- exit reason

The final row summarizes the compounded return over the entire simulation period.

Example:

```csv
entry_timestamp,entry_date,entry_price,exit_timestamp,exit_date,exit_price,pnl_pct,exit_reason
1578023400,2020-01-03 03:50,7176.8067,1578439200,2020-01-07 23:20,8253.8100,0.1501,profit_target
...
1577836800,2020-01-01 00:00,7174.3900,1782860400,2026-06-30 23:00,58624.7100,18.4048,accumulated
```

## Design Philosophy

The simulator intentionally follows a different design from the production strategy.

- **Independent implementation**: The simulator does **not** reuse the production implementation to provide an independent reference implementation capable of detecting behavioral regressions in `strategy-core`.
- **Procedural, not OOP:** Plain structs and free functions rather than the component-based class hierarchy used in `strategy-core`.
- **Sequential Processing:** Historical prices are processed strictly in chronological order. Only information available at the current timestamp is used, preventing look-ahead bias.
- **Instant Fills:** There is no live exchange in the loop, so BUY/SELL signals are assumed to execute immediately at the signal price. This models the theoretical strategy behavior rather than execution latency or slippage.
