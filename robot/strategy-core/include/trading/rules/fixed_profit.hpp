#pragma once

#include "trading/interfaces/exit_rule.hpp"

namespace trading {

// Fixed profit exit rule:
//   1. Hard stop-loss at entry_price * (1 - stop_loss_pct).
//   2. Take-profit at entry_price * (1 + profit_target_pct).
class FixedProfitExitRule : public ExitRule {
   public:
    FixedProfitExitRule(double stop_loss_pct, double profit_target_pct);
    bool Check(const MarketState& state, double entry_price, double highest_price) override;

   private:
    double stop_loss_pct_;
    double profit_target_pct_;
};

}  // namespace trading
