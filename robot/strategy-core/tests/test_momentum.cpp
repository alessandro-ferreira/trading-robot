#include <gtest/gtest.h>

#include <memory>
#include <vector>

#include "trading/rules/momentum.hpp"
#include "trading/state/sliding_window.hpp"

namespace trading {

class MomentumTest : public ::testing::Test {
   protected:
    // Helper to create a state with a specific price history
    std::unique_ptr<SlidingWindowPriceState> CreateState(const std::vector<double>& prices) {
        // The window size must be at least as large as the number of prices to hold them.
        auto state = std::make_unique<SlidingWindowPriceState>(prices.size());
        for (double p : prices) {
            state->UpdatePrice(p);
        }
        return state;
    }
};

TEST_F(MomentumTest, FailsIfNotEnoughData) {
    // Lookback is 5, but we only provide 3 prices. The state is not ready.
    std::vector<std::pair<int, double>> config = {{5, 0.01}};
    MomentumEntryRule rule(config, false);
    auto state = CreateState({100.0, 101.0, 102.0});

    // The rule should check if the state is ready and return false.
    EXPECT_FALSE(rule.Check(*state));
}

TEST_F(MomentumTest, OrLogic_HandlesInvalidPastPrice) {
    // Window 1: Lookback 1, Threshold 1% -> will use invalid price (0) and be skipped.
    // Window 2: Lookback 2, Threshold 1% -> will pass.
    std::vector<std::pair<int, double>> config = {{1, 0.01}, {2, 0.01}};
    MomentumEntryRule rule(config, false);  // OR logic

    // Prices: 100, 0, 102.
    auto state = CreateState({100.0, 0.0, 102.0});

    // Should return TRUE because the second, valid window passes.
    EXPECT_TRUE(rule.Check(*state));
}

TEST_F(MomentumTest, AndLogic_FailsOnInvalidPastPrice) {
    std::vector<std::pair<int, double>> config = {{1, 0.01}, {2, 0.01}};
    MomentumEntryRule rule(config, true);  // AND logic
    auto state = CreateState({100.0, 0.0, 102.0});
    EXPECT_FALSE(rule.Check(*state));
}

TEST_F(MomentumTest, OrLogic_TriggersIfAnyWindowPasses) {
    // Setup: 2 windows.
    // Window 1: Lookback 1, Threshold 1% (0.01)
    // Window 2: Lookback 2, Threshold 5% (0.05)
    std::vector<std::pair<int, double>> config = {{1, 0.01}, {2, 0.05}};

    // OR Logic (require_all = false)
    MomentumEntryRule rule(config, false);

    // Prices: 100, 100, 102
    // Change 1 tick ago: (102-100)/100 = 0.02 (2%) -> PASSES (> 1%)
    // Change 2 ticks ago: (102-100)/100 = 0.02 (2%) -> FAILS (< 5%)

    auto state = CreateState({100.0, 100.0, 102.0});

    // Should return TRUE because one window passed
    EXPECT_TRUE(rule.Check(*state));
}

TEST_F(MomentumTest, AndLogic_FailsIfAnyWindowFails) {
    std::vector<std::pair<int, double>> config = {{1, 0.01}, {2, 0.05}};

    // AND Logic (require_all = true)
    MomentumEntryRule rule(config, true);

    // Same data as above: One passes, one fails.
    auto state = CreateState({100.0, 100.0, 102.0});

    // Should return FALSE because not ALL windows passed
    EXPECT_FALSE(rule.Check(*state));
}

TEST_F(MomentumTest, AndLogic_TriggersIfAllWindowsPass) {
    std::vector<std::pair<int, double>> config = {{1, 0.01}, {2, 0.01}};
    MomentumEntryRule rule(config, true);  // AND Logic
    auto state = CreateState({100.0, 100.0, 102.0});
    EXPECT_TRUE(rule.Check(*state));
}

}  // namespace trading
