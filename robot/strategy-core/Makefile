CXX = g++
CXXFLAGS = -O2 -W -Wall -fPIC -std=c++17 -Iinclude
AR = ar
ARFLAGS = rcs

BUILD_DIR = build
TARGET = $(BUILD_DIR)/libstrategy.a

# Find all .cpp files in src directory and subdirectories
SRCS := $(shell find src -name "*.cpp")
OBJS := $(SRCS:src/%.cpp=$(BUILD_DIR)/%.o)

# Test configuration
TEST_DIR = tests
TEST_SRCS := $(shell find $(TEST_DIR) -name "*.cpp")
TEST_BINARIES := $(TEST_SRCS:$(TEST_DIR)/%.cpp=$(BUILD_DIR)/tests/%)

GTEST_LIBS = -lgtest -lgtest_main -pthread

# Coverage flags for GCC/Clang
COVERAGE_FLAGS = --coverage -fprofile-arcs -ftest-coverage

.PHONY: all clean test rebuild help coverage pre-commit

all: $(TARGET) ## Build the static library

help: ## Show this help message
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

$(TARGET): $(OBJS)
	@mkdir -p $(dir $@)
	$(AR) $(ARFLAGS) $@ $^

$(BUILD_DIR)/%.o: src/%.cpp
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) -c $< -o $@

test: $(TEST_BINARIES) ## Run all unit tests independently
	@for bin in $(TEST_BINARIES); do echo "Running $$bin..."; ./$$bin || exit 1; done

coverage: ## Run tests with coverage reporting
	@echo "--> Running tests with coverage..."
	@rm -f *.gcno *.gcda coverage.info
	@mkdir -p $(BUILD_DIR)
	# Build each test binary with coverage enabled
	@for src in $(TEST_SRCS); do \
		bin=$${src%.cpp}; \
		bin=$(BUILD_DIR)/$${bin#tests/}_cov; \
		$(CXX) $(filter-out -O2,$(CXXFLAGS)) -O0 -g --coverage $(SRCS) $$src -o $$bin $(GTEST_LIBS); \
		./$$bin || true; \
	done
	@lcov --quiet --capture --directory . --output-file coverage.info --no-external --ignore-errors mismatch,mismatch --rc geninfo_ignore_errors=mismatch
	@lcov --quiet --remove coverage.info '$(shell pwd)/tests/*' -o coverage.info --ignore-errors mismatch,mismatch --rc geninfo_ignore_errors=mismatch
	@gcovr --xml-pretty -o coverage.xml -r . --exclude 'tests/.*'
	@gcovr -r . --exclude 'tests/.*'
	@rm -f *.gcno *.gcda coverage.info $(BUILD_DIR)/*_cov coverage_runner *_cov
	@echo "--> Coverage report generated at coverage.xml"

$(BUILD_DIR)/tests/%: $(TEST_DIR)/%.cpp $(TARGET)
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) $< $(TARGET) -o $@ $(GTEST_LIBS)

pre-commit: ## Run pre-commit hooks
	@echo "--> Running pre-commit hooks..."
	@cd ../.. && pre-commit run --all-files

clean: ## Remove build artifacts and coverage files
	rm -rf $(BUILD_DIR)
	rm -f coverage* *_cov
	rm -f *.gcno *.gcda

rebuild: clean all ## Rebuild the library
