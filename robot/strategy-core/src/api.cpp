#include "trading/api.h"

#include <algorithm>
#include <memory>
#include <vector>

#include "trading/rules/fixed_profit.hpp"
#include "trading/rules/momentum.hpp"
#include "trading/rules/trailing_stop.hpp"
#include "trading/state/sliding_window.hpp"
#include "trading/strategy.hpp"
#include "trading/types.hpp"

using std::pair;
using std::unique_ptr;
using std::vector;

extern "C" {

StrategyHandle Strategy_Create(StrategyConfig config) {
    if (config.type == STRATEGY_DUMMY) {
        // A minimal strategy that does nothing, for integration testing.
        auto state = std::make_unique<trading::SlidingWindowPriceState>(1);
        vector<unique_ptr<trading::EntryRule>> entry_rules;
        vector<unique_ptr<trading::ExitRule>> exit_rules;
        // Provide dummy values for profit/trailing parameters for the dummy strategy.
        return new trading::Strategy(std::move(state), std::move(entry_rules), std::move(exit_rules));
    }

    if (config.type == STRATEGY_MOMENTUM_PROFIT || config.type == STRATEGY_MOMENTUM_TRAILING) {
        if (config.num_momentum_windows > MAX_MOMENTUM_WINDOWS || config.num_momentum_windows < 0) {
            return nullptr;  // Invalid config
        }

        if (config.stop_loss_pct <= 0.0) {
            return nullptr;  // Stop loss must be positive
        }

        if (config.type == STRATEGY_MOMENTUM_PROFIT) {
            if (config.profit_target_pct <= 0.0) {
                return nullptr;  // Profit target must be positive
            }
        }

        if (config.type == STRATEGY_MOMENTUM_TRAILING) {
            if (config.activation_pct <= 0.0 || config.trailing_stop_pct <= 0.0) {
                return nullptr;  // Trailing parameters must be positive
            }
        }

        vector<trading::MomentumWindow> momentum_windows;
        long long max_lookback_seconds = 0;
        for (int i = 0; i < config.num_momentum_windows; ++i) {
            if (config.momentum_windows[i].lookback_seconds > max_lookback_seconds) {
                max_lookback_seconds = config.momentum_windows[i].lookback_seconds;
            }
            momentum_windows.push_back(
                {config.momentum_windows[i].lookback_seconds, config.momentum_windows[i].threshold});
        }

        if (config.window_seconds <= max_lookback_seconds) {
            return nullptr;  // Window duration must be strictly greater than max lookback to calculate change
        }

        auto state = std::make_unique<trading::SlidingWindowPriceState>(config.window_seconds);

        vector<unique_ptr<trading::EntryRule>> entry_rules;
        bool require_all = (config.momentum_require_all != 0);
        entry_rules.push_back(std::make_unique<trading::MomentumEntryRule>(momentum_windows, require_all));

        vector<unique_ptr<trading::ExitRule>> exit_rules;

        if (config.type == STRATEGY_MOMENTUM_PROFIT) {
            exit_rules.push_back(
                std::make_unique<trading::FixedProfitExitRule>(config.stop_loss_pct, config.profit_target_pct));
        } else {
            exit_rules.push_back(std::make_unique<trading::TrailingStopExitRule>(
                config.stop_loss_pct, config.activation_pct, config.trailing_stop_pct));
        }

        return new trading::Strategy(std::move(state), std::move(entry_rules), std::move(exit_rules));
    }

    return nullptr;
}

void Strategy_Destroy(StrategyHandle handle) {
    if (handle) {
        delete static_cast<trading::Strategy*>(handle);
    }
}

// Maps the C PriceTick array to the internal C++ type at the API boundary.
static vector<trading::PricePoint> ToHistory(const PricePoint* ticks, int count) {
    if (!ticks || count <= 0) {
        return {};
    }
    // The C and C++ structs are now layout-compatible, allowing for direct construction.
    return vector<trading::PricePoint>(ticks, ticks + count);
}

StrategyStatus Strategy_Init_Profit(StrategyHandle handle, const PricePoint* ticks, int count, int in_position,
                                    double entry_price) {
    if (handle) {
        bool success =
            static_cast<trading::Strategy*>(handle)->Init(ToHistory(ticks, count), in_position != 0, entry_price, 0.0);
        return success ? STRATEGY_SUCCESS : STRATEGY_FAILURE;
    }
    return STRATEGY_FAILURE;
}

StrategyStatus Strategy_Init_Trailing(StrategyHandle handle, const PricePoint* ticks, int count, int in_position,
                                      double entry_price, double highest_price) {
    if (handle) {
        bool success = static_cast<trading::Strategy*>(handle)->Init(ToHistory(ticks, count), in_position != 0,
                                                                     entry_price, highest_price);
        return success ? STRATEGY_SUCCESS : STRATEGY_FAILURE;
    }
    return STRATEGY_FAILURE;
}

StrategyStatus Strategy_UpdatePrice(StrategyHandle handle, double price, long long timestamp) {
    if (handle) {
        bool success = static_cast<trading::Strategy*>(handle)->UpdatePrice({timestamp, price});
        return success ? STRATEGY_SUCCESS : STRATEGY_FAILURE;
    }
    return STRATEGY_FAILURE;
}

Signal Strategy_GetSignal(StrategyHandle handle) {
    if (!handle) return SIGNAL_HOLD;
    return static_cast<trading::Strategy*>(handle)->GetSignal();
}
}
