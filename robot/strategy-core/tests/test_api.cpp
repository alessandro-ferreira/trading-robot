#include <gtest/gtest.h>

#include <vector>

#include "trading/api.h"

namespace trading {

class ApiTest : public ::testing::Test {
   protected:
    StrategyConfig config_;
    StrategyHandle handle_ = nullptr;

    void SetUp() override {
        config_.type = STRATEGY_MOMENTUM_TRAILING;
        config_.window_seconds = 7200;  // 2h window, accommodates 1h lookback
        config_.num_momentum_windows = 1;
        config_.momentum_windows[0].lookback_seconds = 3600;  // 1h
        config_.momentum_windows[0].threshold = 0.01;         // 1%
        config_.momentum_require_all = 0;                     // OR logic
        config_.stop_loss_pct = 0.05;
        config_.activation_pct = 0.10;
        config_.trailing_stop_pct = 0.05;
        config_.profit_target_pct = 0.0;  // Not used for trailing
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

TEST_F(ApiTest, CreateDummyStrategy) {
    config_.type = STRATEGY_DUMMY;
    handle_ = Strategy_Create(config_);
    EXPECT_NE(handle_, nullptr);
}

TEST_F(ApiTest, CreateFixedProfitStrategy) {
    config_.type = STRATEGY_MOMENTUM_PROFIT;
    config_.profit_target_pct = 0.10;
    handle_ = Strategy_Create(config_);
    EXPECT_NE(handle_, nullptr);
}

TEST_F(ApiTest, ReturnsNullForInvalidType) {
    config_.type = static_cast<StrategyType>(-1);
    handle_ = Strategy_Create(config_);
    EXPECT_EQ(handle_, nullptr);
}

TEST_F(ApiTest, ReturnsNullForTooManyMomentumWindows) {
    config_.num_momentum_windows = MAX_MOMENTUM_WINDOWS + 1;
    handle_ = Strategy_Create(config_);
    EXPECT_EQ(handle_, nullptr);
}

TEST_F(ApiTest, ReturnsNullForInsufficientWindowSize) {
    // window_seconds must be strictly greater than max lookback_seconds.
    config_.momentum_windows[0].lookback_seconds = 3600;
    config_.window_seconds = config_.momentum_windows[0].lookback_seconds;  // equal -> invalid
    handle_ = Strategy_Create(config_);
    EXPECT_EQ(handle_, nullptr);

    config_.window_seconds = config_.momentum_windows[0].lookback_seconds - 1;
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

TEST_F(ApiTest, ReturnsNullForInvalidProfitTarget) {
    config_.type = STRATEGY_MOMENTUM_PROFIT;
    config_.profit_target_pct = 0.0;
    handle_ = Strategy_Create(config_);
    EXPECT_EQ(handle_, nullptr);
}

TEST_F(ApiTest, InitDoesNotTriggerSignal) {
    handle_ = Strategy_Create(config_);

    // Load history that does NOT meet the signal threshold.
    // 0.5% gain (100 -> 100.5) over 1h is below the 1% threshold.
    PricePoint ticks[] = {{0, 100.0}, {7200, 100.5}};
    Strategy_Init_Trailing(handle_, ticks, 2, 0, 0.0, 0.0);

    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_SEARCHING_BUY_ENTRY);
}

TEST_F(ApiTest, UpdateConfigChangesParametersWithoutClearingHistory) {
    handle_ = Strategy_Create(config_);

    // Warm up the engine
    Strategy_UpdatePrice(handle_, 100.0, 0);
    Strategy_UpdatePrice(handle_, 100.0, 7200);  // 2h span, engine should be ready

    // Update config (e.g., change stop loss)
    config_.stop_loss_pct = 0.02;
    EXPECT_EQ(Strategy_UpdateConfig(handle_, config_), STRATEGY_SUCCESS);

    // If history was cleared, engine wouldn't be ready.
    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_SEARCHING_BUY_ENTRY);
}

TEST_F(ApiTest, UpdatePriceTriggersSignal) {
    handle_ = Strategy_Create(config_);

    // Warm up: window not ready yet.
    Strategy_UpdatePrice(handle_, 100.0, 0);
    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_SEARCHING_BUY_ENTRY);

    Strategy_UpdatePrice(handle_, 100.0, 3600);
    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_SEARCHING_BUY_ENTRY);

    // Window ready; 1h-ago price is 100, current is 102 -> 2% gain >= 1% threshold.
    Strategy_UpdatePrice(handle_, 102.0, 7200);
    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_BUY);  // BUY
}

TEST_F(ApiTest, MatchAllConfigurationRespected) {
    // Configure 2 windows: 1h > 1%, 2h > 5%.
    config_.num_momentum_windows = 2;
    config_.momentum_windows[1].lookback_seconds = 7200;  // 2h
    config_.momentum_windows[1].threshold = 0.05;
    config_.momentum_require_all = 1;  // AND logic
    config_.window_seconds = 10800;    // must be > max lookback (7200)

    handle_ = Strategy_Create(config_);

    // Feed 4 hourly prices: 100, 100, 100, 102.
    // 1h back: (102-100)/100 = 2% >= 1% -> passes.
    // 2h back: (102-100)/100 = 2% <  5% -> fails.
    Strategy_UpdatePrice(handle_, 100.0, 0);
    Strategy_UpdatePrice(handle_, 100.0, 3600);
    Strategy_UpdatePrice(handle_, 100.0, 7200);
    Strategy_UpdatePrice(handle_, 102.0, 10800);

    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_SEARCHING_BUY_ENTRY);
}

TEST_F(ApiTest, InitPositionEnablesExitRules) {
    handle_ = Strategy_Create(config_);

    // Restore state: in position at 100, peak also 100. Current price is 90 (10% loss).
    // Stop loss is 5%, so this triggers a Phase 1 exit.
    Strategy_Init_Trailing(handle_, nullptr, 0, 1, 100.0, 100.0);
    Strategy_UpdatePrice(handle_, 90.0, 1);

    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_SELL);
}

TEST_F(ApiTest, InitProfitPositionEnablesExitRules) {
    config_.type = STRATEGY_MOMENTUM_PROFIT;
    config_.profit_target_pct = 0.10;
    handle_ = Strategy_Create(config_);

    // Restore state: in position at 100. Current price is 111 (11% gain > 10% target).
    Strategy_Init_Profit(handle_, nullptr, 0, 1, 100.0);
    Strategy_UpdatePrice(handle_, 111.0, 1);

    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_SELL);
}

TEST_F(ApiTest, ConfirmBuyTransitionsState) {
    handle_ = Strategy_Create(config_);
    // Trigger BUY
    Strategy_UpdatePrice(handle_, 100.0, 0);
    Strategy_UpdatePrice(handle_, 100.0, 3600);
    Strategy_UpdatePrice(handle_, 102.0, 7200);
    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_BUY);

    // Confirm BUY at 102.0
    Strategy_SetInPosition(handle_, 1, 102.0, 102.0);

    // Next tick should be tracking exit
    Strategy_UpdatePrice(handle_, 103.0, 7300);
    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_TRACKING_SELL_EXIT);
}

TEST_F(ApiTest, RetrySignalRevertsState) {
    handle_ = Strategy_Create(config_);
    // Trigger BUY
    Strategy_UpdatePrice(handle_, 100.0, 0);
    Strategy_UpdatePrice(handle_, 100.0, 3600);
    Strategy_UpdatePrice(handle_, 102.0, 7200);
    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_BUY);

    // Cancel BUY: Use RetrySignal to return to IDLE
    Strategy_RetrySignal(handle_, SIGNAL_BUY);

    // Supply neutral price
    Strategy_UpdatePrice(handle_, 100.5, 7201);
    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_SEARCHING_BUY_ENTRY);
}

TEST_F(ApiTest, ResetSignalTransitionsState) {
    handle_ = Strategy_Create(config_);
    // Trigger BUY
    Strategy_UpdatePrice(handle_, 100.0, 0);
    Strategy_UpdatePrice(handle_, 100.0, 3600);
    Strategy_UpdatePrice(handle_, 102.0, 7200);
    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_BUY);

    // Reset: Use RetrySignal to return to IDLE
    Strategy_RetrySignal(handle_, SIGNAL_BUY);

    // Supply a neutral price so it doesn't immediately re-trigger
    Strategy_UpdatePrice(handle_, 100.5, 7201);
    EXPECT_EQ(Strategy_GetSignal(handle_), SIGNAL_SEARCHING_BUY_ENTRY);
}

}  // namespace trading
