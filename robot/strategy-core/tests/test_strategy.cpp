#include <gtest/gtest.h>

#include <memory>
#include <vector>

#include "trading/interfaces/entry_rule.hpp"
#include "trading/interfaces/exit_rule.hpp"
#include "trading/interfaces/market_state.hpp"
#include "trading/strategy.hpp"

using std::unique_ptr;
using std::vector;

namespace trading {

// --- Mocks ---

class MockMarketState : public MarketState {
   public:
    double current_price_ = 0.0;
    long long last_timestamp_ = 0;
    bool Init([[maybe_unused]] const vector<PricePoint>& history) override { return true; }
    bool UpdatePrice(const PricePoint& tick) override {
        current_price_ = tick.price;
        last_timestamp_ = tick.timestamp;
        return true;
    }
    double GetCurrentPrice() const override { return current_price_; }
};

class MockEntryRule : public EntryRule {
   public:
    bool should_trigger_ = false;
    bool Check([[maybe_unused]] const MarketState& state) override { return should_trigger_; }
};

class MockExitRule : public ExitRule {
   public:
    bool should_trigger_ = false;
    mutable double last_highest_price_ = 0.0;  // mutable to allow modification in const Check
    bool Check([[maybe_unused]] const MarketState& state, [[maybe_unused]] double entry_price,
               double highest_price) override {
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
        vector<unique_ptr<EntryRule>> entry_rules;
        entry_rules.push_back(std::move(entry));

        auto exit = std::make_unique<MockExitRule>();
        mock_exit_ = exit.get();
        vector<unique_ptr<ExitRule>> exit_rules;
        exit_rules.push_back(std::move(exit));

        strategy_ = std::make_unique<Strategy>(std::move(state), std::move(entry_rules), std::move(exit_rules));
    }

    MockMarketState* mock_state_;
    MockEntryRule* mock_entry_;
    MockExitRule* mock_exit_;
    unique_ptr<Strategy> strategy_;
};

// --- Tests ---

TEST_F(StrategyTest, StartsWithNoSignal) {
    strategy_->UpdatePrice({1, 100.0});
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_HOLD);
}

TEST_F(StrategyTest, EntersPositionWhenEntryRuleTriggers) {
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});

    // Signal should be SIGNAL_BUY
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_BUY);
}

TEST_F(StrategyTest, TracksHighestPriceWhileInPosition) {
    // Enter Position at 100
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});
    strategy_->GetSignal();  // Consumes the buy signal, sets in_position_ = true

    // Price moves up to 110
    strategy_->UpdatePrice({2, 110.0});

    // Trigger Exit Rule
    mock_exit_->should_trigger_ = true;

    // Check Signal
    // Verify the result is SIGNAL_SELL.
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_SELL);
    // Verify that the highest price was correctly passed to the rule
    EXPECT_DOUBLE_EQ(mock_exit_->last_highest_price_, 110.0);
}

TEST_F(StrategyTest, ExitsPositionWhenExitRuleTriggers) {
    // Enter Position
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_BUY);  // BUY

    // Update Price (Holding)
    mock_entry_->should_trigger_ = false;  // Entry rule doesn't matter anymore
    strategy_->UpdatePrice({2, 105.0});
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_HOLD);  // HOLD

    // Exit Trigger
    mock_exit_->should_trigger_ = true;
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_SELL);  // SELL
}

TEST_F(StrategyTest, ResetsAfterExit) {
    // Buy
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});
    strategy_->GetSignal();

    // Sell to exit the position
    mock_exit_->should_trigger_ = true;
    strategy_->GetSignal();

    // Next tick: Ensure no new entry or exit is triggered. The strategy should be reset.
    mock_entry_->should_trigger_ = false;
    mock_exit_->should_trigger_ = false;
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_HOLD);  // No entry trigger yet -> HOLD
}

TEST_F(StrategyTest, InitCorrectly) {
    strategy_->Init({}, true, 100.0, 100.0);

    // Now that we are in a position, an exit rule should be able to trigger
    mock_exit_->should_trigger_ = true;
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_SELL);  // SELL
}

TEST_F(StrategyTest, InitRestoresHighestPrice) {
    // Simulate restart: position entered at 100, peak was 120 before restart.
    strategy_->Init({}, true, 100.0, 120.0);

    // The highest price passed to the exit rule must be 120, not 100.
    mock_exit_->should_trigger_ = false;
    strategy_->GetSignal();
    EXPECT_DOUBLE_EQ(mock_exit_->last_highest_price_, 120.0);
}

}  // namespace trading
