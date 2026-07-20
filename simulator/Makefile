CXX = g++
CXXFLAGS = -O2 -W -Wall -fPIC -std=c++17
LDFLAGS = -ldl

BUILD_DIR = build
TARGET = $(BUILD_DIR)/simulator
SRC = simulator.cpp

.PHONY: all run clean help rebuild

all: $(TARGET) ## Build the simulator executable

help: ## Show this help message
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

$(TARGET): $(SRC)
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) $< $(LDFLAGS) -o $@

simulator-help: all ## Show the help message for the simulator
	./$(TARGET) --help

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)

rebuild: clean all ## Rebuild the application from scratch
