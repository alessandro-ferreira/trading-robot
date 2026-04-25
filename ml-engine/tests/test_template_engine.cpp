#include <gtest/gtest.h>

#include "TemplateEngine.hpp"

using std::string;

namespace trading::ml {

class TemplateEngineTest : public ::testing::Test {
   protected:
    TemplateEngine engine_;
    string exchange_ = "binance";
};

TEST_F(TemplateEngineTest, BTCConfigurationMatchesMigration) {
    auto update = engine_.GenerateStrategyUpdate(exchange_, "BTC/USDT");
    EXPECT_EQ(update.strategy_type, "momentum_trailing");
    EXPECT_EQ(update.momentum_params.window_seconds, 10);
    EXPECT_FALSE(update.momentum_params.require_all);
    EXPECT_DOUBLE_EQ(update.momentum_params.stop_loss_pct, 0.1);

    ASSERT_TRUE(update.momentum_params.activation_pct.has_value());
    EXPECT_DOUBLE_EQ(*update.momentum_params.activation_pct, 0.05);
    ASSERT_TRUE(update.momentum_params.trailing_stop_pct.has_value());
    EXPECT_DOUBLE_EQ(*update.momentum_params.trailing_stop_pct, 0.02);

    auto risk = engine_.GenerateRiskUpdate(exchange_, "BTC/USDT");
    EXPECT_DOUBLE_EQ(risk.risk_per_trade, 100.0);
    EXPECT_DOUBLE_EQ(risk.max_position_size, 1.0);
}

TEST_F(TemplateEngineTest, ETHConfigurationMatchesMigration) {
    auto update = engine_.GenerateStrategyUpdate(exchange_, "ETH/USDT");
    EXPECT_EQ(update.strategy_type, "momentum_profit");
    EXPECT_EQ(update.momentum_params.window_seconds, 10);
    EXPECT_TRUE(update.momentum_params.require_all);

    ASSERT_TRUE(update.momentum_params.profit_target_pct.has_value());
    EXPECT_DOUBLE_EQ(*update.momentum_params.profit_target_pct, 0.05);
    EXPECT_EQ(update.momentum_params.windows.size(), 3);

    auto risk = engine_.GenerateRiskUpdate(exchange_, "ETH/USDT");
    EXPECT_DOUBLE_EQ(risk.risk_per_trade, 50.0);
    EXPECT_DOUBLE_EQ(risk.max_position_size, 10.0);
}

TEST_F(TemplateEngineTest, LTCConfigurationMatchesMigration) {
    auto update = engine_.GenerateStrategyUpdate(exchange_, "LTC/USDT");
    EXPECT_EQ(update.strategy_type, "dummy");
    EXPECT_TRUE(update.enabled);

    auto risk = engine_.GenerateRiskUpdate(exchange_, "LTC/USDT");
    EXPECT_DOUBLE_EQ(risk.risk_per_trade, 25.0);
    EXPECT_DOUBLE_EQ(risk.max_position_size, 5.0);
}

TEST_F(TemplateEngineTest, UnknownSymbolFallback) {
    auto update = engine_.GenerateStrategyUpdate(exchange_, "UNKNOWN/USDT");
    EXPECT_FALSE(update.enabled);
    EXPECT_EQ(update.strategy_type, "dummy");

    auto risk = engine_.GenerateRiskUpdate(exchange_, "UNKNOWN/USDT");
    EXPECT_DOUBLE_EQ(risk.risk_per_trade, 0.0);
    EXPECT_DOUBLE_EQ(risk.max_position_size, 0.0);
}

}  // namespace trading::ml
