CXX = g++
CXXFLAGS = -O2 -W -Wall -fPIC -std=c++17 -Iinclude -Itemplate -I$(PROTO_GEN_DIR) -I/usr/local/include
LDFLAGS = -L/usr/local/lib `pkg-config --libs grpc++ grpc` -lgrpc++_reflection -lprotobuf -lpthread -ldl

PROTOC = protoc
GRPC_CPP_PLUGIN = grpc_cpp_plugin
GRPC_CPP_PLUGIN_PATH ?= $(shell which $(GRPC_CPP_PLUGIN))

PROTO_SRC_DIR = ../robot/proto/v1
BUILD_DIR = build
PROTO_GEN_DIR = $(BUILD_DIR)/proto
PROTOS = $(PROTO_SRC_DIR)/management.proto

# Find all .cpp files in root, src, and template directories
SRCS := main.cpp $(shell find src -name "*.cpp") $(shell find template -name "*.cpp")
OBJS := $(SRCS:%.cpp=$(BUILD_DIR)/%.o)
ENGINE_SRCS := $(filter-out main.cpp, $(SRCS))

# Generated Proto files logic
PROTO_SRCS = $(PROTO_GEN_DIR)/management.pb.cc $(PROTO_GEN_DIR)/management.grpc.pb.cc
PROTO_OBJS = $(PROTO_SRCS:$(PROTO_GEN_DIR)/%.cc=$(BUILD_DIR)/proto/%.o)

# Test configuration
TEST_DIR = tests
TEST_SRCS := $(shell find $(TEST_DIR) -name "*.cpp")
TEST_BINARIES := $(TEST_SRCS:$(TEST_DIR)/%.cpp=$(BUILD_DIR)/tests/%)
GTEST_LIBS = -lgtest -lgtest_main -pthread

# Coverage flags for GCC/Clang
COVERAGE_FLAGS = --coverage -fprofile-arcs -ftest-coverage

# Objects for testing (excludes main.o to avoid double main definition)
ENGINE_OBJS = $(filter-out $(BUILD_DIR)/main.o, $(OBJS))

TARGET = $(BUILD_DIR)/engine

.PHONY: all run clean test rebuild proto install-deps help coverage pre-commit

all: proto $(TARGET) ## Build the engine executable

help: ## Show this help message
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

$(TARGET): $(PROTO_OBJS) $(OBJS)
	@mkdir -p $(dir $@)
	$(CXX) $^ $(LDFLAGS) -o $@

$(BUILD_DIR)/%.o: %.cpp
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) -c $< -o $@

run: all ## Run the engine
	./$(TARGET)

test: proto $(TEST_BINARIES) ## Run all unit tests independently
	@for bin in $(TEST_BINARIES); do echo "Running $$bin..."; ./$$bin || exit 1; done

coverage: proto ## Run tests with coverage reporting
	@echo "--> Running tests with coverage..."
	@rm -f *.gcno *.gcda coverage.info coverage.xml
	@mkdir -p $(BUILD_DIR)
	# Build each test binary with coverage enabled
	@for src in $(TEST_SRCS); do \
		bin=$${src%.cpp}; \
		bin=$(BUILD_DIR)/$${bin#tests/}_cov; \
		$(CXX) $(filter-out -O2,$(CXXFLAGS)) -O0 -g --coverage $(PROTO_SRCS) $(ENGINE_SRCS) $$src -o $$bin $(LDFLAGS) -lgtest -lgtest_main; \
		./$$bin || true; \
	done
	@lcov --quiet --capture --directory . --output-file coverage.info --no-external --ignore-errors mismatch,mismatch --rc geninfo_ignore_errors=mismatch --rc geninfo_unexecuted_blocks=1
	@lcov --quiet --remove coverage.info '$(shell pwd)/build/*' '$(shell pwd)/tests/*' -o coverage.info --ignore-errors mismatch,mismatch --rc geninfo_ignore_errors=mismatch --rc geninfo_unexecuted_blocks=1
	@gcovr --xml-pretty -o coverage.xml -r . --exclude 'tests/.*' --exclude 'build/proto/.*'
	@gcovr -r . --exclude 'tests/.*' --exclude 'build/proto/.*'
	@rm -f *.gcno *.gcda coverage.info $(BUILD_DIR)/*_cov coverage_runner
	@echo "--> Coverage report generated at coverage.xml"

$(BUILD_DIR)/tests/%: $(TEST_DIR)/%.cpp $(PROTO_OBJS) $(ENGINE_OBJS)
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) $^ $(LDFLAGS) -lgtest -lgtest_main -o $@

$(BUILD_DIR)/proto/%.o: $(PROTO_GEN_DIR)/%.cc
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) -c $< -o $@

# Explicit rule for generating C++ files from proto definitions
$(PROTO_GEN_DIR)/%.pb.cc $(PROTO_GEN_DIR)/%.grpc.pb.cc: $(PROTO_SRC_DIR)/%.proto
	@mkdir -p $(PROTO_GEN_DIR)
	$(PROTOC) -I=$(PROTO_SRC_DIR) \
		--cpp_out=$(PROTO_GEN_DIR) \
		--grpc_out=$(PROTO_GEN_DIR) \
		--plugin=protoc-gen-grpc=$(GRPC_CPP_PLUGIN_PATH) \
		$<

$(BUILD_DIR)/tests/%.o: $(TEST_DIR)/%.cpp
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) -c $< -o $@

proto: $(PROTO_SRCS) ## Generate gRPC code from proto files

install-deps: ## Install system dependencies
	sudo apt-get update && sudo apt-get install -y libgrpc++-dev protobuf-compiler-grpc pkg-config

pre-commit: ## Run pre-commit hooks
	@echo "--> Running pre-commit hooks..."
	@cd .. && pre-commit run --all-files

clean: ## Remove build artifacts and coverage files
	rm -rf $(BUILD_DIR)
	rm -f coverage* *_cov
	rm -f *.gcno *.gcda

rebuild: clean all ## Rebuild the engine
