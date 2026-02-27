#include <gtest/gtest.h>

#include <memory>
#include <vector>

#include "trading/interfaces/entry_rule.hpp"
#include "trading/interfaces/exit_rule.hpp"
#include "trading/interfaces/market_state.hpp"
#include "trading/strategy.hpp"

namespace trading {

// --- Mocks ---

class MockMarketState : public MarketState {
   public:
    double current_price_ = 0.0;
    void UpdatePrice(double price) override { current_price_ = price; }
    double GetCurrentPrice() const override { return current_price_; }
};

class MockEntryRule : public EntryRule {
   public:
    bool should_trigger_ = false;
    bool Check(const MarketState& state) override { return should_trigger_; }
};

class MockExitRule : public ExitRule {
   public:
    bool should_trigger_ = false;
    mutable double last_highest_price_ = 0.0;  // mutable to allow modification in const Check
    bool Check(const MarketState& state, double entry_price, double highest_price) override {
        last_highest_price_ = highest_price;
        return should_trigger_;
    }
};

// --- Test Fixture ---

class StrategyTest : public ::testing::Test {
   protected:
    void SetUp() override {
        auto state = std::make_unique<MockMarketState>();
        mock_state_ = state.get();

        auto entry = std::make_unique<MockEntryRule>();
        mock_entry_ = entry.get();
        std::vector<std::unique_ptr<EntryRule>> entry_rules;
        entry_rules.push_back(std::move(entry));

        auto exit = std::make_unique<MockExitRule>();
        mock_exit_ = exit.get();
        std::vector<std::unique_ptr<ExitRule>> exit_rules;
        exit_rules.push_back(std::move(exit));

        strategy_ = std::make_unique<Strategy>(std::move(state), std::move(entry_rules), std::move(exit_rules));
    }

    MockMarketState* mock_state_;
    MockEntryRule* mock_entry_;
    MockExitRule* mock_exit_;
    std::unique_ptr<Strategy> strategy_;
};

// --- Tests ---

TEST_F(StrategyTest, StartsWithNoSignal) {
    strategy_->UpdatePrice(100.0);
    EXPECT_DOUBLE_EQ(strategy_->GetSignal(), 0.0);
}

TEST_F(StrategyTest, EntersPositionWhenEntryRuleTriggers) {
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice(100.0);

    // Signal should be 1.0 (BUY)
    EXPECT_DOUBLE_EQ(strategy_->GetSignal(), 1.0);
}

TEST_F(StrategyTest, TracksHighestPriceWhileInPosition) {
    // Enter Position at 100
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice(100.0);
    strategy_->GetSignal();  // Consumes the buy signal, sets in_position_ = true

    // Price moves up to 110
    strategy_->UpdatePrice(110.0);

    // Trigger Exit Rule
    mock_exit_->should_trigger_ = true;

    // Check Signal
    // Verify the result is SELL (-1.0).
    EXPECT_DOUBLE_EQ(strategy_->GetSignal(), -1.0);
    // Verify that the highest price was correctly passed to the rule
    EXPECT_DOUBLE_EQ(mock_exit_->last_highest_price_, 110.0);
}

TEST_F(StrategyTest, ExitsPositionWhenExitRuleTriggers) {
    // Enter Position
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice(100.0);
    EXPECT_DOUBLE_EQ(strategy_->GetSignal(), 1.0);  // BUY

    // Update Price (Holding)
    mock_entry_->should_trigger_ = false;  // Entry rule doesn't matter anymore
    strategy_->UpdatePrice(105.0);
    EXPECT_DOUBLE_EQ(strategy_->GetSignal(), 0.0);  // HOLD

    // Exit Trigger
    mock_exit_->should_trigger_ = true;
    EXPECT_DOUBLE_EQ(strategy_->GetSignal(), -1.0);  // SELL
}

TEST_F(StrategyTest, ResetsAfterExit) {
    // Buy
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice(100.0);
    strategy_->GetSignal();

    // Sell to exit the position
    mock_exit_->should_trigger_ = true;
    strategy_->GetSignal();

    // Next tick: Ensure no new entry or exit is triggered. The strategy should be reset.
    mock_entry_->should_trigger_ = false;
    mock_exit_->should_trigger_ = false;
    EXPECT_DOUBLE_EQ(strategy_->GetSignal(), 0.0);  // No entry trigger yet -> HOLD
}

TEST_F(StrategyTest, InitCorrectly) {
    strategy_->Init(nullptr, 0, true, 100.0, 100.0);

    // Now that we are in a position, an exit rule should be able to trigger
    mock_exit_->should_trigger_ = true;
    EXPECT_DOUBLE_EQ(strategy_->GetSignal(), -1.0);  // SELL
}

TEST_F(StrategyTest, InitRestoresHighestPrice) {
    // Simulate restart: position entered at 100, peak was 120 before restart.
    strategy_->Init(nullptr, 0, true, 100.0, 120.0);

    // The highest price passed to the exit rule must be 120, not 100.
    mock_exit_->should_trigger_ = false;
    strategy_->GetSignal();
    EXPECT_DOUBLE_EQ(mock_exit_->last_highest_price_, 120.0);
}

}  // namespace trading
