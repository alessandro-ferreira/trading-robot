# Makefile for the Python Exchange Gateway
MAIN_FILE := main.py
CONFIG_FILE ?= ../config.toml

PYTHON := python
VENV_CHECK := $(shell python -c 'import sys; print(1 if sys.prefix != sys.base_prefix else 0)')

.PHONY: help run test
 
help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

# ==============================================================================
# Development
# ==============================================================================

# This is a "private" command not intended for direct use.
define CHECK_VENV
	@if [ "$(VENV_CHECK)" = "0" ]; then \
		echo "ERROR: Python virtual environment not activated. Please run 'source <path_to_venv>/bin/activate' first."; \
		exit 1; \
	fi
endef

run: ## Run the gRPC server
	$(call CHECK_VENV)
	@echo "--> Running Python gRPC gateway..."
	@$(PYTHON) $(MAIN_FILE) --config $(CONFIG_FILE)

# ==============================================================================
# Testing
# ==============================================================================

test: ## Run unit tests
	$(call CHECK_VENV)
	@echo "--> Running unit tests..."
	@$(PYTHON) -m unittest discover -s tests -t .