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

# Generated Proto files logic
PROTO_SRCS = $(PROTO_GEN_DIR)/management.pb.cc $(PROTO_GEN_DIR)/management.grpc.pb.cc
PROTO_OBJS = $(PROTO_SRCS:$(PROTO_GEN_DIR)/%.cc=$(BUILD_DIR)/proto/%.o)

# Test configuration
TEST_DIR = tests
TEST_SRCS := $(shell find $(TEST_DIR) -name "*.cpp")
TEST_OBJS := $(TEST_SRCS:$(TEST_DIR)/%.cpp=$(BUILD_DIR)/tests/%.o)
TEST_TARGET = $(BUILD_DIR)/run_tests

# Objects for testing (excludes main.o to avoid double main definition)
ENGINE_OBJS = $(filter-out $(BUILD_DIR)/main.o, $(OBJS))

TARGET = $(BUILD_DIR)/engine

all: proto $(TARGET)

$(TARGET): $(PROTO_OBJS) $(OBJS)
	@mkdir -p $(dir $@)
	$(CXX) $^ $(LDFLAGS) -o $@

$(BUILD_DIR)/%.o: %.cpp
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) -c $< -o $@

run: all
	./$(TARGET)

test: proto $(TEST_TARGET)
	./$(TEST_TARGET)

$(TEST_TARGET): $(PROTO_OBJS) $(ENGINE_OBJS) $(TEST_OBJS)
	@mkdir -p $(dir $@)
	$(CXX) $^ $(LDFLAGS) -lgtest -lgtest_main -o $@

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

proto: $(PROTO_SRCS)

install-deps:
	sudo apt-get update && sudo apt-get install -y libgrpc++-dev protobuf-compiler-grpc pkg-config

clean:
	rm -rf $(BUILD_DIR)

rebuild: clean all

.PHONY: all run clean test rebuild proto install-deps
