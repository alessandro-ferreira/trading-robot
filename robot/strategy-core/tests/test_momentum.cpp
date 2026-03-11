#include <gtest/gtest.h>

#include <memory>
#include <vector>

#include "trading/rules/momentum.hpp"
#include "trading/state/sliding_window.hpp"

using std::unique_ptr;
using std::vector;

namespace trading {

class MomentumTest : public ::testing::Test {
   protected:
    // Builds a SlidingWindowPriceState from a price list using hourly (3600s) intervals.
    // window_duration_seconds must be provided to accommodate the intended lookbacks.
    unique_ptr<SlidingWindowPriceState> CreateState(const vector<PricePoint>& ticks,
                                                    long long window_duration_seconds) {
        auto state = std::make_unique<SlidingWindowPriceState>(window_duration_seconds);
        for (const auto& tick : ticks) {
            bool result = state->UpdatePrice(tick);
            EXPECT_TRUE(result) << "UpdatePrice failed for tick with timestamp " << tick.timestamp << " and price "
                                << tick.price;
        }
        return state;
    }
};

TEST_F(MomentumTest, FailsIfNotEnoughData) {
    // Lookback is 5h, but we only provide 3 hourly prices (span=2h < 5h window). Not ready.
    vector<MomentumWindow> config = {{5 * 3600, 0.01}};
    MomentumEntryRule rule(config, false);
    auto state = CreateState({{0, 100.0}, {3600, 101.0}, {7200, 102.0}}, 6 * 3600);

    EXPECT_FALSE(rule.Check(*state));
}

TEST_F(MomentumTest, OrLogic_HandlesInvalidPastPrice) {
    // Window 1: 1h lookback -> hits a data gap (stale data), so it's skipped in OR mode.
    // Window 2: 2h lookback -> hits 100.0, change is 2% >= 1% threshold -> passes.
    vector<MomentumWindow> config = {{3600, 0.01}, {7200, 0.01}};
    MomentumEntryRule rule(config, false);  // OR logic

    // Prices at t=0, t=3600, and t=10800, with a gap at t=7200.
    // At t=10800: 1h back (target 7200) is stale -> skip; 2h back (target 3600) is 100 -> 2%>=1% -> true.
    auto state = CreateState({{0, 100.0}, {3600, 100.0}, {10800, 102.0}}, 3 * 3600);

    EXPECT_TRUE(rule.Check(*state));
}

TEST_F(MomentumTest, AndLogic_FailsOnInvalidPastPrice) {
    vector<MomentumWindow> config = {{3600, 0.01}, {7200, 0.01}};
    MomentumEntryRule rule(config, true);  // AND logic

    // Same data: 1h back is stale -> AND fails immediately.
    auto state = CreateState({{0, 100.0}, {3600, 100.0}, {10800, 102.0}}, 3 * 3600);

    EXPECT_FALSE(rule.Check(*state));
}

TEST_F(MomentumTest, OrLogic_TriggersIfAnyWindowPasses) {
    // Window 1: 1h lookback, 1% threshold -> 2% change passes.
    // Window 2: 2h lookback, 5% threshold -> 2% change fails.
    // OR: one passing window is enough.
    vector<MomentumWindow> config = {{3600, 0.01}, {7200, 0.05}};
    MomentumEntryRule rule(config, false);

    // Prices at t=[0,3600,7200,10800]: 100,100,100,102
    // 1h back={7200,100}->2%>=1%->pass. 2h back={3600,100}->2%<5%->fail.
    auto state = CreateState({{0, 100.0}, {3600, 100.0}, {7200, 100.0}, {10800, 102.0}}, 3 * 3600);

    EXPECT_TRUE(rule.Check(*state));
}

TEST_F(MomentumTest, AndLogic_FailsIfAnyWindowFails) {
    vector<MomentumWindow> config = {{3600, 0.01}, {7200, 0.05}};
    MomentumEntryRule rule(config, true);  // AND logic

    // Same data: 2h window fails (2% < 5%) -> AND returns false.
    auto state = CreateState({{0, 100.0}, {3600, 100.0}, {7200, 100.0}, {10800, 102.0}}, 3 * 3600);

    EXPECT_FALSE(rule.Check(*state));
}

TEST_F(MomentumTest, AndLogic_TriggersIfAllWindowsPass) {
    // Both windows have 1% threshold; both see a 2% gain.
    vector<MomentumWindow> config = {{3600, 0.01}, {7200, 0.01}};
    MomentumEntryRule rule(config, true);  // AND logic

    auto state = CreateState({{0, 100.0}, {3600, 100.0}, {7200, 100.0}, {10800, 102.0}}, 3 * 3600);

    EXPECT_TRUE(rule.Check(*state));
}

}  // namespace trading
