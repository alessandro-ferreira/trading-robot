#include "trading/rules/momentum.hpp"

#include "trading/state/sliding_window.hpp"

namespace trading {

MomentumEntryRule::MomentumEntryRule(const std::vector<std::pair<int, double>>& lookback_thresholds, bool require_all)
    : lookback_thresholds_(lookback_thresholds), require_all_(require_all) {}

bool MomentumEntryRule::Check(const MarketState& state) {
    const auto* momentum_state = dynamic_cast<const SlidingWindowPriceState*>(&state);
    if (!momentum_state || !momentum_state->IsReady()) {
        return false;
    }

    for (const auto& [lookback, threshold] : lookback_thresholds_) {
        double current = momentum_state->GetCurrentPrice();
        double past = momentum_state->GetPriceAgo(lookback);

        if (past <= 0.000001) {
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
