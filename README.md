# Trading Analysis and Automation Suite

This repository contains a comprehensive suite of tools for backtesting trading strategies and deploying them in a live environment. The project is split into two main components: `data_analysis` and `robot`.

## Project Structure

- **/data_analysis**: Contains tools for running historical simulations (backtesting). It uses high-performance C++ for optimization and Python for data processing and visualization.
- **/robot**: A live trading bot built with a microservices architecture. It consists of a Go application for the core logic and a Python gateway to communicate with exchange APIs.
