#include "trading/strategy.hpp"

#include "trading/rules/momentum.hpp"
#include "trading/rules/risk_management.hpp"
#include "trading/state/sliding_window.hpp"

namespace trading {

Strategy::Strategy(std::unique_ptr<MarketState> state, std::vector<std::unique_ptr<EntryRule>> entry_rules,
                   std::vector<std::unique_ptr<ExitRule>> exit_rules)
    : market_state_(std::move(state)),
      entry_rules_(std::move(entry_rules)),
      exit_rules_(std::move(exit_rules)),
      in_position_(false),
      entry_price_(0.0),
      highest_price_since_entry_(0.0) {}

void Strategy::Init(const double* prices, int count, bool in_position, double entry_price, double highest_price) {
    for (int i = 0; i < count; ++i) {
        market_state_->UpdatePrice(prices[i]);
    }
    in_position_ = in_position;
    entry_price_ = entry_price;
    // Restore the persisted peak so the two-phase exit rule uses the correct phase on restart.
    highest_price_since_entry_ = in_position ? highest_price : 0.0;
}

void Strategy::UpdatePrice(double price) {
    market_state_->UpdatePrice(price);

    if (in_position_) {
        if (price > highest_price_since_entry_) {
            highest_price_since_entry_ = price;
        }
    }
}

double Strategy::GetSignal() {
    if (in_position_) {
        // Check Exit Rules
        for (const auto& rule : exit_rules_) {
            if (rule->Check(*market_state_, entry_price_, highest_price_since_entry_)) {
                in_position_ = false;
                return -1.0;  // SELL Signal
            }
        }

        return 0.0;  // HOLD Signal
    } else {
        // Check Entry Rules
        // All rules must be satisfied to enter (AND logic)
        for (const auto& rule : entry_rules_) {
            if (!rule->Check(*market_state_)) {
                return 0.0;  // No Signal
            }
        }

        // If we get here, all entry rules passed
        in_position_ = true;
        entry_price_ = market_state_->GetCurrentPrice();
        highest_price_since_entry_ = entry_price_;
        return 1.0;  // BUY Signal
    }
}

}  // namespace trading
