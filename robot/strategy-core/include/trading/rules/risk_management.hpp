#pragma once

#include "trading/interfaces/exit_rule.hpp"

namespace trading {

// Two-phase exit rule:
//   Phase 1 (before activation): hard stop-loss at entry_price * (1 - stop_loss_pct).
//   Phase 2 (after activation):  trailing stop at highest_price * (1 - trailing_stop_pct).
// Activation is determined by the highest price seen since entry:
//   if (highest_price / entry_price - 1) >= activation_pct → Phase 2.
class RiskManagementExitRule : public ExitRule {
   public:
    RiskManagementExitRule(double stop_loss_pct, double activation_pct, double trailing_stop_pct);
    bool Check(const MarketState& state, double entry_price, double highest_price) override;

   private:
    double stop_loss_pct_;
    double activation_pct_;
    double trailing_stop_pct_;
};

}  // namespace trading
