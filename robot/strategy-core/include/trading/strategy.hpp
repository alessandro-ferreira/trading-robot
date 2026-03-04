#pragma once

#include <memory>
#include <vector>

#include "trading/interfaces/entry_rule.hpp"
#include "trading/interfaces/exit_rule.hpp"
#include "trading/interfaces/market_state.hpp"

using std::unique_ptr;
using std::vector;

namespace trading {

class Strategy {
   public:
    Strategy(unique_ptr<MarketState> state, vector<unique_ptr<EntryRule>> entry_rules,
             vector<unique_ptr<ExitRule>> exit_rules);

    // Initializes the strategy with history and position state.
    // highest_price must be the highest price seen since the position was opened (persisted by the caller).
    bool Init(const vector<PricePoint>& history, bool in_position, double entry_price, double highest_price);

    // Feeds a live price tick. Also tracks the highest price seen while in position.
    bool UpdatePrice(const PricePoint& tick);

    // Evaluates entry or exit rules and transitions internal state.
    // Must be called exactly once per tick after UpdatePrice.
    // Returns: 1 (Buy), -1 (Sell), 0 (Hold)
    int GetSignal();

   private:
    // Components (Composition)
    unique_ptr<MarketState> market_state_;
    vector<unique_ptr<EntryRule>> entry_rules_;
    vector<unique_ptr<ExitRule>> exit_rules_;

    // Internal State
    bool in_position_;
    double entry_price_;
    double highest_price_since_entry_;
};

}  // namespace trading
