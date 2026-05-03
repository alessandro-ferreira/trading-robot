#include "trading/rules/momentum.hpp"

#include "trading/state/sliding_window.hpp"
#include "trading/types.hpp"

using std::vector;

namespace trading {

MomentumEntryRule::MomentumEntryRule(const vector<MomentumWindow>& windows, bool require_all) {
    windows_ = windows;
    require_all_ = require_all;
}

bool MomentumEntryRule::Check(const MarketState& state) {
    const auto* momentum_state = dynamic_cast<const SlidingWindowPriceState*>(&state);
    if (!momentum_state || !momentum_state->IsReady()) {
        return false;
    }

    double current = momentum_state->GetCurrentPrice();
    for (const auto& [lookback_seconds, threshold] : windows_) {
        double past = momentum_state->GetPriceSecondsAgo(lookback_seconds);

        // A non-positive past price indicates that the lookback period went beyond the available history.
        if (past <= 0.0) {
            if (require_all_) return false;  // In AND mode, invalid data fails the check
            continue;                        // In OR mode, skip invalid data
        }

        double pct_change = (current - past) / past;
        bool met = pct_change >= threshold;

        if (require_all_ && !met) return false;  // AND: one failure means total failure
        if (!require_all_ && met) return true;   // OR: one success means total success
    }

    return require_all_;  // If AND: true (all passed). If OR: false (none passed).
}

}  // namespace trading
