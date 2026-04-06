#include "trading/strategy.hpp"

#include "trading/rules/momentum.hpp"
#include "trading/state/sliding_window.hpp"
#include "trading/types.hpp"

using std::unique_ptr;
using std::vector;

namespace trading {

Strategy::Strategy(unique_ptr<MarketState> state, vector<unique_ptr<EntryRule>> entry_rules,
                   vector<unique_ptr<ExitRule>> exit_rules) {
    market_state_ = std::move(state);
    entry_rules_ = std::move(entry_rules);
    exit_rules_ = std::move(exit_rules);
    state_ = STATE_IDLE;
    entry_price_ = 0.0;
    highest_price_since_entry_ = 0.0;
}

bool Strategy::Init(const vector<PricePoint>& history, StrategyState state, double entry_price, double highest_price) {
    if (!history.empty()) {
        if (!market_state_->Init(history)) {
            return false;
        }
    }
    state_ = state;
    entry_price_ = entry_price;
    // Restore the persisted peak so the two-phase exit rule uses the correct phase on restart.
    highest_price_since_entry_ = (state == STATE_ACTIVE || state == STATE_PENDING_SELL) ? highest_price : 0.0;
    return true;
}

void Strategy::UpdateRules(vector<unique_ptr<EntryRule>> entry_rules, vector<unique_ptr<ExitRule>> exit_rules) {
    entry_rules_ = std::move(entry_rules);
    exit_rules_ = std::move(exit_rules);
}

bool Strategy::UpdatePrice(const PricePoint& tick) {
    if (!market_state_->UpdatePrice(tick)) {
        return false;
    }
    if (state_ == STATE_ACTIVE || state_ == STATE_PENDING_SELL) {
        if (tick.price > highest_price_since_entry_) {
            highest_price_since_entry_ = tick.price;
        }
    }
    return true;
}

Signal Strategy::GetSignal() {
    switch (state_) {
        case STATE_IDLE:
            // Check Entry Rules: All rules must be satisfied to enter (AND logic).
            for (const auto& rule : entry_rules_) {
                if (!rule->Check(*market_state_)) {
                    return SIGNAL_SEARCHING_ENTRY;
                }
            }

            // All entry rules passed. Transition to PENDING_BUY.
            // The strategy will wait in this state until the Go bot confirms the fill.
            state_ = STATE_PENDING_BUY;
            return SIGNAL_BUY;

        case STATE_ACTIVE:
            // Check Exit Rules: If any rule is satisfied, we exit (OR logic).
            for (const auto& rule : exit_rules_) {
                if (rule->Check(*market_state_, entry_price_, highest_price_since_entry_)) {
                    // Transition to PENDING_SELL. The strategy will wait in this state until the Go bot confirms the
                    // fill.
                    state_ = STATE_PENDING_SELL;
                    return SIGNAL_SELL;
                }
            }
            return SIGNAL_TRACKING_EXIT;

        case STATE_PENDING_BUY:
            return SIGNAL_WAITING_BUY_FILL;

        case STATE_PENDING_SELL:
            return SIGNAL_WAITING_SELL_FILL;

        default:
            // This case should be unreachable with a valid state, but we return a sentinel.
            return SIGNAL_INVALID;
    }
}

void Strategy::ConfirmSignal(Signal signal, double fill_price) {
    if (signal == SIGNAL_BUY) {
        state_ = STATE_ACTIVE;
        entry_price_ = fill_price;
        highest_price_since_entry_ = fill_price;
    } else if (signal == SIGNAL_SELL) {
        // For sells, we just return to IDLE
        ResetSignal();
    }
}

void Strategy::CancelSignal(Signal signal) {
    if (signal == SIGNAL_BUY && state_ == STATE_PENDING_BUY) {
        ResetSignal();
    } else if (signal == SIGNAL_SELL && state_ == STATE_PENDING_SELL) {
        state_ = STATE_ACTIVE;
    }
}

void Strategy::ResetSignal() {
    state_ = STATE_IDLE;
    entry_price_ = 0.0;
    highest_price_since_entry_ = 0.0;
}

}  // namespace trading
