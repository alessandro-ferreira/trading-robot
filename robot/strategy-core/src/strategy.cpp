#include "trading/strategy.hpp"

#include "trading/rules/momentum.hpp"
#include "trading/state/sliding_window.hpp"
#include "trading/types.hpp"

using std::unique_ptr;
using std::vector;

namespace trading {

Strategy::Strategy(unique_ptr<MarketState> state, vector<unique_ptr<EntryRule>> entry_rules,
                   vector<unique_ptr<ExitRule>> exit_rules)
    : market_state_(std::move(state)),
      entry_rules_(std::move(entry_rules)),
      exit_rules_(std::move(exit_rules)),
      in_position_(false),
      entry_price_(0.0),
      highest_price_since_entry_(0.0) {}

bool Strategy::Init(const vector<PricePoint>& history, bool in_position, double entry_price, double highest_price) {
    if (!market_state_->Init(history)) {
        return false;
    }
    in_position_ = in_position;
    entry_price_ = entry_price;
    // Restore the persisted peak so the two-phase exit rule uses the correct phase on restart.
    highest_price_since_entry_ = in_position ? highest_price : 0.0;
    return true;
}

bool Strategy::UpdatePrice(const PricePoint& tick) {
    if (!market_state_->UpdatePrice(tick)) {
        return false;
    }
    if (in_position_) {
        if (tick.price > highest_price_since_entry_) {
            highest_price_since_entry_ = tick.price;
        }
    }
    return true;
}

int Strategy::GetSignal() {
    if (in_position_) {
        // Check Exit Rules
        for (const auto& rule : exit_rules_) {
            if (rule->Check(*market_state_, entry_price_, highest_price_since_entry_)) {
                in_position_ = false;
                return SIGNAL_SELL;
            }
        }

        return SIGNAL_HOLD;
    } else {
        // Check Entry Rules
        // All rules must be satisfied to enter (AND logic)
        for (const auto& rule : entry_rules_) {
            if (!rule->Check(*market_state_)) {
                return SIGNAL_HOLD;
            }
        }

        // If we get here, all entry rules passed
        in_position_ = true;
        entry_price_ = market_state_->GetCurrentPrice();
        highest_price_since_entry_ = entry_price_;
        return SIGNAL_BUY;
    }
}

}  // namespace trading
