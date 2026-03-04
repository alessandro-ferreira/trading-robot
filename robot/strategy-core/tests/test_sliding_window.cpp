#include <gtest/gtest.h>

#include "trading/state/sliding_window.hpp"

using std::vector;
namespace trading {

// All tests use hourly intervals (3600s) to match the timescale of real strategy configuration.

TEST(SlidingWindowTest, InitializesCorrectly) {
    SlidingWindowPriceState state(7200);
    EXPECT_FALSE(state.IsReady());
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 0.0);
}

TEST(SlidingWindowTest, ReadyOnceHistorySpansWindowDuration) {
    SlidingWindowPriceState state(7200);  // 2h window

    state.UpdatePrice({0, 100.0});
    EXPECT_FALSE(state.IsReady());  // span = 0

    state.UpdatePrice({3600, 101.0});
    EXPECT_FALSE(state.IsReady());  // span = 3600 < 7200

    state.UpdatePrice({7200, 102.0});
    EXPECT_TRUE(state.IsReady());  // span = 7200 >= 7200
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 102.0);
}

TEST(SlidingWindowTest, GetPriceSecondsAgoReturnsLastKnownPrice) {
    SlidingWindowPriceState state(7200);
    state.UpdatePrice({0, 100.0});
    state.UpdatePrice({3600, 101.0});
    state.UpdatePrice({7200, 102.0});

    // 3600s ago from t=7200 is t=3600 -> exact match
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(3600), 101.0);
    // 7200s ago from t=7200 is t=0 -> exact match
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(7200), 100.0);
}

TEST(SlidingWindowTest, GetPriceSecondsAgoHandlesGap) {
    // Simulates a missed update: no entry at t=3600, so the last known price before that is at t=0.
    SlidingWindowPriceState state(7200);
    state.UpdatePrice({0, 100.0});
    state.UpdatePrice({7200, 102.0});  // gap: no update at t=3600

    // 3600s ago from t=7200 => look for t<=3600. Last entry there is {0, 100.0}.
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(3600), 100.0);
}

TEST(SlidingWindowTest, OldEntriesEvictedAsWindowAdvances) {
    SlidingWindowPriceState state(7200);
    state.UpdatePrice({0, 100.0});
    state.UpdatePrice({3600, 101.0});
    state.UpdatePrice({7200, 102.0});   // ready; cutoff=0, {0,100} retained
    state.UpdatePrice({10800, 103.0});  // cutoff=3600, {0,100} evicted

    // 7200s ago from t=10800 is t=3600 -> first retained entry
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(7200), 101.0);
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 103.0);
}

TEST(SlidingWindowTest, GetPriceSecondsAgoReturnsZeroWithNoHistory) {
    SlidingWindowPriceState state(7200);
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(3600), 0.0);
}

TEST(SlidingWindowTest, GetPriceSecondsAgoReturnsZeroIfLookbackExceedsHistory) {
    SlidingWindowPriceState state(7200);
    state.UpdatePrice({3600, 100.0});

    // Looking back 7200s from t=3600 means target=t<=-3600. No entry exists there.
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(7200), 0.0);
}

TEST(SlidingWindowTest, InitWithSmallHistory) {
    SlidingWindowPriceState state(100);
    // History size 2: Ensure both are loaded, not just the last one.
    vector<PricePoint> history = {{100, 10.0}, {150, 11.0}};
    state.Init(history);

    // If the first point was dropped, looking back to t=100 would fail.
    // Current=150. 50s ago is 100.
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(50), 10.0);
}

TEST(SlidingWindowTest, ReadyStateRevertsOnGap) {
    SlidingWindowPriceState state(100);
    state.UpdatePrice({100, 10.0});
    state.UpdatePrice({200, 10.0});  // Ready (span 100 >= 100)
    EXPECT_TRUE(state.IsReady());

    state.UpdatePrice({500, 10.0});  // Gap. Old entries evicted.
    // 500 - 100 = 400 cutoff. 100 and 200 evicted. Only 500 remains.
    EXPECT_FALSE(state.IsReady());
}

TEST(SlidingWindowTest, InitFailsWithUnsortedHistory) {
    SlidingWindowPriceState state(100);
    vector<PricePoint> history = {{200, 10.0}, {100, 11.0}};  // Unsorted
    EXPECT_FALSE(state.Init(history));
}

TEST(SlidingWindowTest, UpdatePriceFailsWithStaleTick) {
    SlidingWindowPriceState state(100);
    state.UpdatePrice({200, 10.0});
    // This tick is older than the current state, so it should be rejected.
    EXPECT_FALSE(state.UpdatePrice({199, 11.0}));
}

}  // namespace trading
