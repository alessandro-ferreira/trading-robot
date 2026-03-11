#include "trading/rules/fixed_profit.hpp"

#include "trading/interfaces/market_state.hpp"

namespace trading {

FixedProfitExitRule::FixedProfitExitRule(double stop_loss_pct, double profit_target_pct) {
    stop_loss_pct_ = stop_loss_pct;
    profit_target_pct_ = profit_target_pct;
}

bool FixedProfitExitRule::Check(const MarketState& state, double entry_price, [[maybe_unused]] double highest_price) {
    double current = state.GetCurrentPrice();

    // Stop Loss
    if (current <= entry_price * (1.0 - stop_loss_pct_)) return true;

    // Take Profit
    if (current >= entry_price * (1.0 + profit_target_pct_)) return true;

    return false;
}

}  // namespace trading
