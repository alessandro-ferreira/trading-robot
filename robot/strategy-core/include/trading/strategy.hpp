#pragma once

#include <memory>
#include <vector>

#include "trading/interfaces/entry_rule.hpp"
#include "trading/interfaces/exit_rule.hpp"
#include "trading/interfaces/market_state.hpp"

namespace trading {

class Strategy {
   public:
    Strategy(std::unique_ptr<MarketState> state, std::vector<std::unique_ptr<EntryRule>> entry_rules,
             std::vector<std::unique_ptr<ExitRule>> exit_rules);

    // Initializes the strategy with history and position state.
    // highest_price must be the highest price seen since the position was opened (persisted by the caller).
    // Ignored when in_position is false.
    void Init(const double* prices, int count, bool in_position, double entry_price, double highest_price);

    // Feeds a live price tick. Also tracks the highest price seen while in position.
    void UpdatePrice(double price);

    // Evaluates entry or exit rules and transitions internal state.
    // Must be called exactly once per tick after UpdatePrice.
    // Returns: 1.0 (Buy), -1.0 (Sell), 0.0 (Hold)
    double GetSignal();

   private:
    // Components (Composition)
    std::unique_ptr<MarketState> market_state_;
    std::vector<std::unique_ptr<EntryRule>> entry_rules_;
    std::vector<std::unique_ptr<ExitRule>> exit_rules_;

    // Internal State
    bool in_position_;
    double entry_price_;
    double highest_price_since_entry_;
};

}  // namespace trading
