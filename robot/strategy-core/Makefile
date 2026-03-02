CXX = g++
CXXFLAGS = -O3 -Wall -fPIC -std=c++17 -Iinclude
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
TEST_OBJS := $(TEST_SRCS:$(TEST_DIR)/%.cpp=$(BUILD_DIR)/tests/%.o)
TEST_TARGET = $(BUILD_DIR)/run_tests

all: $(TARGET)

$(TARGET): $(OBJS)
	@mkdir -p $(dir $@)
	$(AR) $(ARFLAGS) $@ $^

$(BUILD_DIR)/%.o: src/%.cpp
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) -c $< -o $@

test: $(TEST_TARGET)
	./$(TEST_TARGET)

$(TEST_TARGET): $(TEST_OBJS) $(TARGET)
	$(CXX) $(CXXFLAGS) $^ -o $@ -lgtest -lgtest_main -pthread

$(BUILD_DIR)/tests/%.o: $(TEST_DIR)/%.cpp
	@mkdir -p $(dir $@)
	$(CXX) $(CXXFLAGS) -c $< -o $@

clean:
	rm -rf $(BUILD_DIR)

rebuild: clean all

.PHONY: all clean test rebuild
