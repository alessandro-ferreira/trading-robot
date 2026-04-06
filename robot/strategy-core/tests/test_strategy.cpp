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
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_SEARCHING_ENTRY);
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
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_BUY);

    // Confirm fill to move from PENDING_BUY to ACTIVE
    strategy_->ConfirmSignal(SIGNAL_BUY, 100.0);

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

TEST_F(StrategyTest, GetStateExposesInternalState) {
    EXPECT_EQ(strategy_->GetState(), STATE_IDLE);
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});
    strategy_->GetSignal();
    EXPECT_EQ(strategy_->GetState(), STATE_PENDING_BUY);
}

TEST_F(StrategyTest, ExitsPositionWhenExitRuleTriggers) {
    // Enter Position
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_BUY);  // BUY
    strategy_->ConfirmSignal(SIGNAL_BUY, 100.0);

    // Update Price (Holding)
    mock_entry_->should_trigger_ = false;  // Entry rule doesn't matter anymore
    strategy_->UpdatePrice({2, 105.0});
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_TRACKING_EXIT);

    // Exit Trigger
    mock_exit_->should_trigger_ = true;
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_SELL);  // SELL
}

TEST_F(StrategyTest, ResetsAfterExit) {
    // Buy
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});
    strategy_->GetSignal();  // Transition to PENDING_BUY
    strategy_->ConfirmSignal(SIGNAL_BUY, 100.0);

    // Sell to exit the position
    mock_exit_->should_trigger_ = true;
    strategy_->GetSignal();  // Transition to PENDING_SELL
    strategy_->ConfirmSignal(SIGNAL_SELL, 90.0);

    // Next tick: Ensure no new entry or exit is triggered. The strategy should be reset.
    mock_entry_->should_trigger_ = false;
    mock_exit_->should_trigger_ = false;
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_SEARCHING_ENTRY);
}

TEST_F(StrategyTest, InitCorrectly) {
    strategy_->Init({}, STATE_ACTIVE, 100.0, 100.0);

    // Now that we are in a position, an exit rule should be able to trigger
    mock_exit_->should_trigger_ = true;
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_SELL);  // SELL
}

TEST_F(StrategyTest, InitWithEmptyHistoryPreservesExistingBuffer) {
    // Warm up buffer
    strategy_->UpdatePrice({1, 100.0});

    // Rehydrate trade metadata but pass empty history
    strategy_->Init({}, STATE_ACTIVE, 100.0, 100.0);

    // If history was wiped, mock_state_ would be empty.
    // We verify by checking if an update still works sequentially.
    EXPECT_TRUE(strategy_->UpdatePrice({2, 105.0}));
    EXPECT_DOUBLE_EQ(mock_state_->current_price_, 105.0);
}

TEST_F(StrategyTest, InitRestoresHighestPrice) {
    // Simulate restart: position entered at 100, peak was 120 before restart.
    strategy_->Init({}, STATE_ACTIVE, 100.0, 120.0);

    // The highest price passed to the exit rule must be 120, not 100.
    mock_exit_->should_trigger_ = false;
    strategy_->GetSignal();
    EXPECT_DOUBLE_EQ(mock_exit_->last_highest_price_, 120.0);
}

TEST_F(StrategyTest, PendingBuyStateLocksSignal) {
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});

    // First call triggers transition to PENDING_BUY
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_BUY);

    // Subsequent calls should return WAITING_BUY_FILL until confirmed
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_WAITING_BUY_FILL);
}

TEST_F(StrategyTest, ConfirmBuyMovesToActive) {
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});
    strategy_->GetSignal();

    strategy_->ConfirmSignal(SIGNAL_BUY, 105.0);

    strategy_->UpdatePrice({2, 106.0});
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_TRACKING_EXIT);
}

TEST_F(StrategyTest, PendingSellStateLocksSignal) {
    strategy_->Init({}, STATE_ACTIVE, 100.0, 100.0);
    mock_exit_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 90.0});

    // First call triggers transition to PENDING_SELL
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_SELL);

    // Subsequent calls should return WAITING_SELL_FILL until confirmed
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_WAITING_SELL_FILL);
}

TEST_F(StrategyTest, CancelBuyReturnsToIdle) {
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});
    strategy_->GetSignal();  // PENDING_BUY

    strategy_->CancelSignal(SIGNAL_BUY);

    mock_entry_->should_trigger_ = false;
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_SEARCHING_ENTRY);
}

TEST_F(StrategyTest, CancelSellReturnsToActiveAndPreservesState) {
    // Init in active state
    strategy_->Init({}, STATE_ACTIVE, 100.0, 120.0);
    mock_exit_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 90.0});
    strategy_->GetSignal();  // PENDING_SELL

    strategy_->CancelSignal(SIGNAL_SELL);

    mock_exit_->should_trigger_ = false;
    // Verify we are back to tracking and highest_price (120) is preserved
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_TRACKING_EXIT);
    EXPECT_DOUBLE_EQ(mock_exit_->last_highest_price_, 120.0);
}

TEST_F(StrategyTest, ResetSignalReturnsToIdle) {
    mock_entry_->should_trigger_ = true;
    strategy_->UpdatePrice({1, 100.0});
    strategy_->GetSignal();

    strategy_->ResetSignal();

    mock_entry_->should_trigger_ = false;  // Ensure it doesn't immediately re-trigger a buy
    EXPECT_EQ(strategy_->GetSignal(), SIGNAL_SEARCHING_ENTRY);
}

}  // namespace trading
