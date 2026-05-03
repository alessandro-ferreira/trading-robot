#pragma once

#include <memory>
#include <vector>

#include "trading/interfaces/entry_rule.hpp"
#include "trading/interfaces/exit_rule.hpp"
#include "trading/interfaces/market_state.hpp"
#include "trading/types.hpp"

using std::unique_ptr;
using std::vector;

namespace trading {

class Strategy {
   public:
    Strategy(unique_ptr<MarketState> state, vector<unique_ptr<EntryRule>> entry_rules,
             vector<unique_ptr<ExitRule>> exit_rules);

    // Initializes the strategy with history and position state.
    // highest_price must be the highest price seen since the position was opened (persisted by the caller).
    // Returns false if the history is not in chronological order or has non-positive price.
    bool Init(const vector<PricePoint>& history, bool in_position, double entry_price, double highest_price);

    // Updates the entry and exit rules without resetting the market state history.
    void UpdateRules(vector<unique_ptr<EntryRule>> entry_rules, vector<unique_ptr<ExitRule>> exit_rules);

    // Sets the strategy's position state and prices.
    void SetInPosition(bool in_position, double entry_price, double highest_price);

    // Feeds a live price tick. Also tracks the highest price seen while in position.
    // Returns false if the tick seems corrupted (e.g. timestamp in the past, non-positive price, or unrealistic price
    // jump).
    bool UpdatePrice(const PricePoint& tick);

    // Returns the current internal state of the strategy.
    StrategyState GetState() const { return state_; }

    // Evaluates entry or exit rules and transitions internal state.
    // Must be called exactly once per tick after UpdatePrice.
    // Returns SIGNAL_INVALID if the strategy is in a corrupted state (e.g. entry price not set while ACTIVE).
    StrategySignal GetSignal();

    // Changes the internal state to resend the specified signal.
    // Only valid for order placement signals (BUY or SELL) when pending confirmation, otherwise is ignored.
    void RetrySignal(StrategySignal signal);

   private:
    // Components (Composition)
    unique_ptr<MarketState> market_state_;
    vector<unique_ptr<EntryRule>> entry_rules_;
    vector<unique_ptr<ExitRule>> exit_rules_;

    // Internal State
    StrategyState state_;
    double entry_price_;
    double highest_price_since_entry_;
};

}  // namespace trading
