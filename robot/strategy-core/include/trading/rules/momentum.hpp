#pragma once

#include <utility>
#include <vector>

#include "trading/interfaces/entry_rule.hpp"
#include "trading/interfaces/market_state.hpp"

namespace trading {

// Entry rule that triggers a buy when any configured momentum window exceeds its threshold.
// Each window is a (lookback_ticks, threshold_pct) pair.
// Logic can be configured as OR (any window triggers) or AND (all windows must trigger).
class MomentumEntryRule : public EntryRule {
   public:
    MomentumEntryRule(const std::vector<std::pair<int, double>>& lookback_thresholds, bool require_all);
    bool Check(const MarketState& state) override;

   private:
    std::vector<std::pair<int, double>> lookback_thresholds_;
    bool require_all_;
};

}  // namespace trading
