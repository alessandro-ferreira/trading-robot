#include "trading/rules/trailing_stop.hpp"

#include "trading/interfaces/market_state.hpp"

namespace trading {

TrailingStopExitRule::TrailingStopExitRule(double stop_loss_pct, double activation_pct, double trailing_stop_pct) {
    stop_loss_pct_ = stop_loss_pct;
    activation_pct_ = activation_pct;
    trailing_stop_pct_ = trailing_stop_pct;
}

bool TrailingStopExitRule::Check(const MarketState& state, double entry_price, double highest_price) {
    double current = state.GetCurrentPrice();

    // Use the highest price seen since entry to determine which phase we are in.
    // Once the position has ever reached the activation threshold, we stay in Phase 2
    // even if current price has since dropped back below it.
    double peak_gain = (highest_price - entry_price) / entry_price;

    if (peak_gain >= activation_pct_) {
        // Phase 2: trailing stop — exit if current has pulled back more than trailing_stop_pct from peak.
        return current <= highest_price * (1.0 - trailing_stop_pct_);
    }

    // Phase 1: flat stop-loss — exit if current has fallen more than stop_loss_pct from entry.
    return current <= entry_price * (1.0 - stop_loss_pct_);
}

}  // namespace trading
