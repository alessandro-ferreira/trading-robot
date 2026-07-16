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
    // Simulates a missed update: no entry at t=3600.
    SlidingWindowPriceState state(7200);
    state.UpdatePrice({0, 100.0});
    state.UpdatePrice({3450, 101.0});  // Add a point that is not stale
    state.UpdatePrice({7200, 102.0});  // gap: no update at t=3600

    // 3600s ago from t=7200 => look for t<=3600.
    // Last entry is {3450, 101.0}.
    // Gap is 3600 - 3450 = 150, which is <= 300s (lookback staleness). This is a valid match.
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(3600), 101.0);

    // Now test a case where the gap is too large.
    // 6800s ago from t=7200 => look for t<=400.
    // Last entry is {0, 100.0}.
    // Gap is 400 - 0 = 400, which is > 300s. This is stale and should be rejected.
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(6800), 0.0);
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
    vector<PricePoint> history = {{100, 10.0}, {150, 10.1}};
    state.Init(history);

    // If the first point was dropped, looking back to t=100 would fail.
    // Current=150. 50s ago is 100.
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(50), 10.0);
}

TEST(SlidingWindowTest, InitToleratesPriceJumpWithLargeGap) {
    SlidingWindowPriceState state(100);
    // 3600s gap between first and second point
    vector<PricePoint> history = {{100, 100.0}, {3700, 200.0}};
    EXPECT_TRUE(state.Init(history));
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 200.0);
}

TEST(SlidingWindowTest, ReadyStateRevertsOnGap) {
    SlidingWindowPriceState state(100);
    state.UpdatePrice({100, 10.0});
    state.UpdatePrice({500, 10.0});  // Ready
    EXPECT_TRUE(state.IsReady());

    state.UpdatePrice({901, 10.0});  // Gap. Old entries evicted.
    // 901 - 500 - 300 (Cutoff tolerance) = 101 > 100. Not ready anymore.
    EXPECT_FALSE(state.IsReady());
}

TEST(SlidingWindowTest, InitFailsWithUnsortedHistory) {
    SlidingWindowPriceState state(100);
    vector<PricePoint> history = {{200, 10.0}, {100, 11.0}};  // Unsorted
    EXPECT_FALSE(state.Init(history));
}

TEST(SlidingWindowTest, InitFailsWithNonPositivePrice) {
    SlidingWindowPriceState state(100);
    vector<PricePoint> history = {{100, 10.0}, {101, -1.0}};
    EXPECT_FALSE(state.Init(history));
}

TEST(SlidingWindowTest, InitSkipsUnrealisticPriceJumps) {
    SlidingWindowPriceState state(100);
    // MAX_TICK_PRICE_CHANGE is 0.015 (0.15%). 10.0 -> 10.2 is a 0.2% jump.
    // The point {101, 10.2} should be skipped.
    vector<PricePoint> history = {{100, 10.0}, {101, 10.2}, {102, 10.1}};
    EXPECT_TRUE(state.Init(history));
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 10.1);
    // Verify the jumpy point was skipped and the lookback still works against the first point.
    EXPECT_DOUBLE_EQ(state.GetPriceSecondsAgo(2), 10.0);
}

TEST(SlidingWindowTest, UpdatePriceFailsWithStaleTick) {
    SlidingWindowPriceState state(100);
    state.UpdatePrice({200, 10.0});
    // This tick is older than the current state, so it should be rejected.
    EXPECT_FALSE(state.UpdatePrice({199, 11.0}));
}

TEST(SlidingWindowTest, RejectsUnrealisticPriceJumps) {
    SlidingWindowPriceState state(100);
    state.UpdatePrice({100, 100.0});
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 100.0);

    // Attempt to update with a >1.5% price jump (100 -> 102). Should be rejected.
    EXPECT_FALSE(state.UpdatePrice({101, 102.0}));
    // Verify that the state was not updated.
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 100.0);

    // Attempt to update with a valid <1.5% price jump (100 -> 101). Should be accepted.
    EXPECT_TRUE(state.UpdatePrice({102, 101.0}));
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 101.0);
}

TEST(SlidingWindowTest, ToleratesUnrealisticPriceJumpWithLargeGap) {
    SlidingWindowPriceState state(100);
    state.UpdatePrice({100, 100.0});

    // Case 1: Small gap, large price jump -> should be rejected
    // Gap = 10s (< 300s), Jump = 10% (> 5%)
    EXPECT_FALSE(state.UpdatePrice({110, 110.0}));
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 100.0);

    // Case 2: Large gap, large price jump -> should be accepted
    // Gap = 301s (> 300s), Jump = 10% (> 5%)
    EXPECT_TRUE(state.UpdatePrice({411, 110.0}));
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 110.0);
}

TEST(SlidingWindowTest, RejectsZeroOrNegativePrice) {
    SlidingWindowPriceState state(100);
    state.UpdatePrice({100, 100.0});

    // Attempt to update with a zero price. Should be rejected.
    EXPECT_FALSE(state.UpdatePrice({101, 0.0}));
    // Attempt to update with a negative price. Should be rejected.
    EXPECT_FALSE(state.UpdatePrice({102, -50.0}));
    // Verify that the state was not updated.
    EXPECT_DOUBLE_EQ(state.GetCurrentPrice(), 100.0);
}

}  // namespace trading
