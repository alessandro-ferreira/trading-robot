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

bool Strategy::Init(const vector<PricePoint>& history, bool in_position, double entry_price, double highest_price) {
    if (!history.empty()) {
        if (!market_state_->Init(history)) {
            return false;
        }
    }
    SetInPosition(in_position, entry_price, highest_price);
    return true;
}

void Strategy::UpdateRules(vector<unique_ptr<EntryRule>> entry_rules, vector<unique_ptr<ExitRule>> exit_rules) {
    entry_rules_ = std::move(entry_rules);
    exit_rules_ = std::move(exit_rules);
}

void Strategy::SetInPosition(bool in_position, double entry_price, double highest_price) {
    if (in_position) {
        state_ = STATE_IN_POSITION;
        entry_price_ = entry_price;
        highest_price_since_entry_ = std::max(highest_price_since_entry_, std::max(highest_price, entry_price));
    } else {
        state_ = STATE_IDLE;
        entry_price_ = 0.0;
        highest_price_since_entry_ = 0.0;
    }
}

bool Strategy::UpdatePrice(const PricePoint& tick) {
    if (!market_state_->UpdatePrice(tick)) {
        return false;
    }
    if (state_ == STATE_IN_POSITION || state_ == STATE_PENDING_SELL) {
        if (tick.price > highest_price_since_entry_) {
            highest_price_since_entry_ = tick.price;
        }
    }
    return true;
}

StrategySignal Strategy::GetSignal() {
    switch (state_) {
        case STATE_IDLE:
            // If no entry rules are defined, stay in searching mode.
            if (entry_rules_.empty()) {
                return SIGNAL_SEARCHING_BUY_ENTRY;
            }

            // Check Entry Rules: All rules must be satisfied to enter (AND logic).
            for (const auto& rule : entry_rules_) {
                if (!rule->Check(*market_state_)) {
                    return SIGNAL_SEARCHING_BUY_ENTRY;
                }
            }

            // All entry rules passed. Transition to PENDING_BUY.
            // The strategy will wait in this state until the Go bot confirms the fill.
            state_ = STATE_PENDING_BUY;
            return SIGNAL_BUY;

        case STATE_IN_POSITION:
            // If entry price is not set to a valid value, the strategy is corrupted.
            // The user needs to use Init or SetInPosition to fix it.
            if (entry_price_ <= 0.0) {
                return SIGNAL_INVALID;
            }

            // If no price has been processed yet, we stay in tracking mode.
            if (market_state_->GetCurrentPrice() <= 0.0) {
                return SIGNAL_TRACKING_SELL_EXIT;
            }

            // Check Exit Rules: If any rule is satisfied, we exit (OR logic).
            for (const auto& rule : exit_rules_) {
                if (rule->Check(*market_state_, entry_price_, highest_price_since_entry_)) {
                    // Transition to PENDING_SELL. The strategy will wait in this state until the Go bot confirms the
                    // fill.
                    state_ = STATE_PENDING_SELL;
                    return SIGNAL_SELL;
                }
            }
            return SIGNAL_TRACKING_SELL_EXIT;

        case STATE_PENDING_BUY:
            return SIGNAL_WAITING_BUY_FILL;

        case STATE_PENDING_SELL:
            return SIGNAL_WAITING_SELL_FILL;

        default:
            return SIGNAL_INVALID;
    }
}

void Strategy::RetrySignal(StrategySignal signal) {
    if (signal == SIGNAL_BUY && state_ == STATE_PENDING_BUY) {
        state_ = STATE_IDLE;
    }
    if (signal == SIGNAL_SELL && state_ == STATE_PENDING_SELL) {
        state_ = STATE_IN_POSITION;
    }
}

}  // namespace trading
