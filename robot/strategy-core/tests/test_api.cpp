#include <gtest/gtest.h>

#include <vector>

#include "trading/api.h"

namespace trading {

class ApiTest : public ::testing::Test {
   protected:
    StrategyConfig config_;
    StrategyHandle handle_ = nullptr;

    void SetUp() override {
        // Default configuration for momentum strategy
        config_.type = STRATEGY_MOMENTUM;
        config_.window_size = 2;  // Set to minimum required for a lookback of 1
        config_.num_momentum_windows = 1;
        config_.momentum_windows[0].lookback = 1;
        config_.momentum_windows[0].threshold = 0.01;  // 1%
        config_.momentum_require_all = 0;              // OR logic
        config_.stop_loss_pct = 0.05;
        config_.activation_pct = 0.10;
        config_.trailing_stop_pct = 0.05;
    }

    void TearDown() override {
        if (handle_) {
            Strategy_Destroy(handle_);
        }
    }
};

TEST_F(ApiTest, CreateAndDestroy) {
    handle_ = Strategy_Create(config_);
    EXPECT_NE(handle_, nullptr);
}

TEST_F(ApiTest, ReturnsNullForInvalidType) {
    config_.type = static_cast<StrategyType>(999);
    handle_ = Strategy_Create(config_);
    EXPECT_EQ(handle_, nullptr);
}

TEST_F(ApiTest, ReturnsNullForTooManyMomentumWindows) {
    config_.num_momentum_windows = MAX_MOMENTUM_WINDOWS + 1;
    handle_ = Strategy_Create(config_);
    EXPECT_EQ(handle_, nullptr);
}

TEST_F(ApiTest, ReturnsNullForInsufficientWindowSize) {
    // Lookback is 1, so window size must be > 1 (at least 2)
    config_.momentum_windows[0].lookback = 1;
    config_.window_size = 1;
    handle_ = Strategy_Create(config_);
    EXPECT_EQ(handle_, nullptr);
}

TEST_F(ApiTest, ReturnsNullForInvalidStopLoss) {
    config_.stop_loss_pct = 0.0;
    handle_ = Strategy_Create(config_);
    EXPECT_EQ(handle_, nullptr);
}

TEST_F(ApiTest, ReturnsNullForInvalidActivationPct) {
    config_.activation_pct = 0.0;
    handle_ = Strategy_Create(config_);
    EXPECT_EQ(handle_, nullptr);
}

TEST_F(ApiTest, ReturnsNullForInvalidTrailingStopPct) {
    config_.trailing_stop_pct = 0.0;
    handle_ = Strategy_Create(config_);
    EXPECT_EQ(handle_, nullptr);
}

TEST_F(ApiTest, InitDoesNotTriggerSignal) {
    handle_ = Strategy_Create(config_);

    // Load history that does NOT meet the signal threshold.
    // 100 -> 100.5 is a 0.5% jump, which is less than the 1% threshold.
    double prices[] = {100.0, 100.5};
    Strategy_Init(handle_, prices, 2, 0, 0.0, 0.0);

    // Should be 0.0 because the threshold is not met.
    EXPECT_DOUBLE_EQ(Strategy_GetSignal(handle_), 0.0);
}

TEST_F(ApiTest, UpdatePriceTriggersSignal) {
    handle_ = Strategy_Create(config_);

    // Warm up with initial price
    Strategy_UpdatePrice(handle_, 100.0);
    EXPECT_DOUBLE_EQ(Strategy_GetSignal(handle_), 0.0);

    // Update with price jump > 1%
    Strategy_UpdatePrice(handle_, 102.0);
    EXPECT_DOUBLE_EQ(Strategy_GetSignal(handle_), 1.0);  // BUY
}

TEST_F(ApiTest, MatchAllConfigurationRespected) {
    // Configure 2 windows: 1 tick > 1%, 2 ticks > 5%
    config_.num_momentum_windows = 2;
    config_.momentum_windows[1].lookback = 2;
    config_.momentum_windows[1].threshold = 0.05;
    config_.momentum_require_all = 1;  // AND logic

    handle_ = Strategy_Create(config_);

    // 100 -> 100 -> 102.
    // 1-tick change: 2% (Passes). 2-tick change: 2% (Fails > 5%).
    Strategy_UpdatePrice(handle_, 100.0);
    Strategy_UpdatePrice(handle_, 100.0);
    Strategy_UpdatePrice(handle_, 102.0);

    // Should be 0.0 because AND logic requires both to pass
    EXPECT_DOUBLE_EQ(Strategy_GetSignal(handle_), 0.0);
}

TEST_F(ApiTest, InitPositionEnablesExitRules) {
    handle_ = Strategy_Create(config_);

    // Restore state: in position at 100, peak also 100 (never went above entry), current price is 90 (10% loss).
    // The configured stop loss is 5%, so this should trigger a sell via Phase 1.
    Strategy_Init(handle_, nullptr, 0, 1, 100.0, 100.0);
    Strategy_UpdatePrice(handle_, 90.0);

    // Check that the exit rule was evaluated and triggered a SELL signal.
    EXPECT_DOUBLE_EQ(Strategy_GetSignal(handle_), -1.0);
}

}  // namespace trading
