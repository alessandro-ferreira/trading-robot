#include <gtest/gtest.h>

#include "trading/state/sliding_window.hpp"

namespace trading {

TEST(SlidingWindowTest, InitializesCorrectly) {
    SlidingWindowPriceState state(3);
    EXPECT_FALSE(state.IsReady());
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 0.0);
}

TEST(SlidingWindowTest, UpdatesPriceAndReadyState) {
    SlidingWindowPriceState state(3);

    state.UpdatePrice(100.0);
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 100.0);
    EXPECT_FALSE(state.IsReady());

    state.UpdatePrice(101.0);
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 101.0);
    EXPECT_FALSE(state.IsReady());

    state.UpdatePrice(102.0);
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 102.0);
    EXPECT_TRUE(state.IsReady());  // Now full
}

TEST(SlidingWindowTest, MaintainsWindowSize) {
    SlidingWindowPriceState state(2);

    state.UpdatePrice(10.0);
    state.UpdatePrice(20.0);
    EXPECT_TRUE(state.IsReady());

    // Add a 3rd item, should push out 10.0
    state.UpdatePrice(30.0);
    EXPECT_TRUE(state.IsReady());
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 30.0);

    // Check history
    // GetPriceAgo(0) -> current -> 30.0
    // GetPriceAgo(1) -> previous -> 20.0
    // GetPriceAgo(2) -> too old -> 0.0 (since window size is 2)

    EXPECT_DOUBLE_EQ(state.GetPriceAgo(0), 30.0);
    EXPECT_DOUBLE_EQ(state.GetPriceAgo(1), 20.0);
    EXPECT_DOUBLE_EQ(state.GetPriceAgo(2), 0.0);
}

TEST(SlidingWindowTest, GetPriceAgoHandlesInsufficientHistory) {
    SlidingWindowPriceState state(5);
    state.UpdatePrice(50.0);

    EXPECT_DOUBLE_EQ(state.GetPriceAgo(0), 50.0);
    EXPECT_DOUBLE_EQ(state.GetPriceAgo(1), 0.0);  // Not enough history yet
}

}  // namespace trading
