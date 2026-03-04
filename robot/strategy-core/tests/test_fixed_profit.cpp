#include <gtest/gtest.h>

#include "trading/interfaces/exit_rule.hpp"
#include "trading/interfaces/market_state.hpp"
#include "trading/rules/fixed_profit.hpp"

using std::vector;

namespace trading {

// A minimal mock for MarketState
struct MockMarketState : public MarketState {
    double current_price_ = 0.0;
    void SetCurrentPrice(double price) { current_price_ = price; }
    double GetCurrentPrice() const override { return current_price_; }
    bool Init([[maybe_unused]] const vector<PricePoint>& history) override { return true; }
    bool UpdatePrice(const PricePoint& tick) override {
        current_price_ = tick.price;
        // timestamp is unused in this mock
        return true;
    }
};

class FixedProfitTest : public ::testing::Test {
   protected:
    // Stop-loss at 5% (0.05), Profit Target at 10% (0.10)
    FixedProfitExitRule rule_{0.05, 0.10};
    MockMarketState state_;
};

TEST_F(FixedProfitTest, DoesNotTriggerInBetween) {
    // Entry at 100. Stop at 95. Target at 110.
    // Current at 105 (Hold)
    state_.SetCurrentPrice(105.0);
    EXPECT_FALSE(rule_.Check(state_, 100.0, 105.0));
}

TEST_F(FixedProfitTest, TriggersOnStopLoss) {
    // Drop to 94 (below 95)
    state_.SetCurrentPrice(94.0);
    EXPECT_TRUE(rule_.Check(state_, 100.0, 100.0));
}

TEST_F(FixedProfitTest, TriggersOnTakeProfit) {
    // Rise to 111 (above 110)
    state_.SetCurrentPrice(111.0);
    EXPECT_TRUE(rule_.Check(state_, 100.0, 111.0));
}

}  // namespace trading
