#include <gtest/gtest.h>

#include "trading/interfaces/exit_rule.hpp"
#include "trading/interfaces/market_state.hpp"
#include "trading/rules/risk_management.hpp"

namespace trading {

// A minimal mock for MarketState, providing only what the ExitRule interface requires.
struct MockMarketState : public MarketState {
    double current_price_ = 0.0;

    void SetCurrentPrice(double price) { current_price_ = price; }

    // --- MarketState Interface Implementation ---
    double GetCurrentPrice() const override { return current_price_; }
    void UpdatePrice(double price) override { current_price_ = price; }
};

class RiskManagementTest : public ::testing::Test {
   protected:
    // Stop-loss at 10% (0.10), activation at 5% (0.05), trailing stop at 3% (0.03)
    RiskManagementExitRule rule_{0.10, 0.05, 0.03};
    MockMarketState state_;
};

TEST_F(RiskManagementTest, DoesNotTriggerOnFavorablePriceMove) {
    // +3% gain, highest never reached activation (5%) → Phase 1 flat stop check.
    state_.SetCurrentPrice(103.0);
    EXPECT_FALSE(rule_.Check(state_, 100.0, 103.0));
}

TEST_F(RiskManagementTest, TriggersOnStopLoss) {
    // Price drops 11% from entry price of 100, exceeding 10% stop-loss.
    // Highest price never reached activation → Phase 1.
    state_.SetCurrentPrice(89.0);
    EXPECT_TRUE(rule_.Check(state_, 100.0, 100.0));
}

TEST_F(RiskManagementTest, DoesNotTriggerWhenPriceIsAboveStopLoss) {
    // Price drops 9% from entry price of 100, which is less than 10% stop-loss.
    state_.SetCurrentPrice(91.0);
    EXPECT_FALSE(rule_.Check(state_, 100.0, 100.0));
}

TEST_F(RiskManagementTest, TriggersOnTrailingStop) {
    // Highest price is 120 (+20% from entry), exceeding activation (5%) → Phase 2.
    // Trailing stop level = 120 * (1 - 0.03) = 116.4. Current at 116 <= 116.4 → exit.
    state_.SetCurrentPrice(116.0);
    EXPECT_TRUE(rule_.Check(state_, 100.0, 120.0));
}

TEST_F(RiskManagementTest, DoesNotTriggerWhenAboveTrailingStop) {
    // Highest price is 120 (+20%) → Phase 2. Trail level = 116.4. Current at 118 > 116.4 → hold.
    state_.SetCurrentPrice(118.0);
    EXPECT_FALSE(rule_.Check(state_, 100.0, 120.0));
}

TEST_F(RiskManagementTest, ActivationPreventsRevertingToFlatStop) {
    // Highest was 108 (+8% from entry) → activation (5%) was reached → Phase 2 stays active.
    // Current drops to 91. Trail level = 108 * (1 - 0.03) = 104.76. 91 <= 104.76 → exit.
    // Note: flat stop (Phase 1) would give 100 * (1 - 0.10) = 90; 91 > 90 would NOT trigger.
    // This confirms Phase 2 is retained even though current price fell back below activation.
    state_.SetCurrentPrice(91.0);
    EXPECT_TRUE(rule_.Check(state_, 100.0, 108.0));
}

}  // namespace trading
