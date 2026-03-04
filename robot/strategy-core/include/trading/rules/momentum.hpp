#pragma once

#include <vector>

#include "trading/interfaces/entry_rule.hpp"
#include "trading/types.hpp"

using std::pair;
using std::vector;

namespace trading {

// Entry rule that triggers a buy when any configured momentum window exceeds its threshold.
// Each window is a (lookback_ticks, threshold_pct) pair.
// Logic can be configured as OR (any window triggers) or AND (all windows must trigger).
class MomentumEntryRule : public EntryRule {
   public:
    MomentumEntryRule(const vector<MomentumWindow>& windows, bool require_all);
    bool Check(const MarketState& state) override;

   private:
    vector<MomentumWindow> windows_;
    bool require_all_;
};

}  // namespace trading
