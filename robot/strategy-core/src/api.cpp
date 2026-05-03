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

// Helper function to build the internal C++ strategy rules from the C config struct. Returns false if the config is
// invalid.
static bool BuildRules(const StrategyConfig& config, vector<unique_ptr<trading::EntryRule>>& entry_rules,
                       vector<unique_ptr<trading::ExitRule>>& exit_rules) {
    if (config.type != STRATEGY_MOMENTUM_PROFIT && config.type != STRATEGY_MOMENTUM_TRAILING) {
        return false;
    }

    if (config.num_momentum_windows > MAX_MOMENTUM_WINDOWS || config.num_momentum_windows <= 0) {
        return false;
    }

    if (config.stop_loss_pct <= 0.0) {
        return false;
    }

    if (config.type == STRATEGY_MOMENTUM_PROFIT && config.profit_target_pct <= 0.0) {
        return false;
    }

    if (config.type == STRATEGY_MOMENTUM_TRAILING &&
        (config.activation_pct <= 0.0 || config.trailing_stop_pct <= 0.0)) {
        return false;
    }

    vector<trading::MomentumWindow> momentum_windows;
    long long max_lookback = 0;
    for (int i = 0; i < config.num_momentum_windows; ++i) {
        if (config.momentum_windows[i].lookback_seconds > max_lookback) {
            max_lookback = config.momentum_windows[i].lookback_seconds;
        }
        momentum_windows.push_back({config.momentum_windows[i].lookback_seconds, config.momentum_windows[i].threshold});
    }

    if (config.window_seconds <= max_lookback) {
        return false;
    }

    bool require_all = (config.momentum_require_all != 0);
    entry_rules.push_back(std::make_unique<trading::MomentumEntryRule>(momentum_windows, require_all));

    if (config.type == STRATEGY_MOMENTUM_PROFIT) {
        exit_rules.push_back(
            std::make_unique<trading::FixedProfitExitRule>(config.stop_loss_pct, config.profit_target_pct));
    } else {
        exit_rules.push_back(std::make_unique<trading::TrailingStopExitRule>(
            config.stop_loss_pct, config.activation_pct, config.trailing_stop_pct));
    }

    return true;
}

StrategyHandle Strategy_Create(StrategyConfig config) {
    if (config.type == STRATEGY_DUMMY) {
        auto state = std::make_unique<trading::SlidingWindowPriceState>(1);
        return new trading::Strategy(std::move(state), {}, {});
    }

    vector<unique_ptr<trading::EntryRule>> entries;
    vector<unique_ptr<trading::ExitRule>> exits;
    if (!BuildRules(config, entries, exits)) {
        return nullptr;
    }

    auto state = std::make_unique<trading::SlidingWindowPriceState>(config.window_seconds);
    return new trading::Strategy(std::move(state), std::move(entries), std::move(exits));
}

StrategyStatus Strategy_UpdateConfig(StrategyHandle handle, StrategyConfig config) {
    if (!handle) return STRATEGY_FAILURE;

    auto strategy = static_cast<trading::Strategy*>(handle);
    if (config.type == STRATEGY_DUMMY) {
        strategy->UpdateRules({}, {});
        return STRATEGY_SUCCESS;
    }

    vector<unique_ptr<trading::EntryRule>> entries;
    vector<unique_ptr<trading::ExitRule>> exits;
    if (!BuildRules(config, entries, exits)) {
        return STRATEGY_FAILURE;
    }

    strategy->UpdateRules(std::move(entries), std::move(exits));
    return STRATEGY_SUCCESS;
}

void Strategy_Destroy(StrategyHandle handle) {
    if (handle) {
        auto strategy = static_cast<trading::Strategy*>(handle);
        delete strategy;
    }
}

// Maps the C PriceTick array to the internal C++ type at the API boundary.
static vector<trading::PricePoint> ToHistory(const PricePoint* ticks, int count) {
    if (!ticks || count <= 0) {
        return {};
    }
    return vector<trading::PricePoint>(ticks, ticks + count);
}

StrategyStatus Strategy_Init_Profit(StrategyHandle handle, const PricePoint* ticks, int count, int in_position,
                                    double entry_price) {
    if (handle) {
        auto strategy = static_cast<trading::Strategy*>(handle);
        bool success = strategy->Init(ToHistory(ticks, count), in_position != 0, entry_price, 0.0);

        return success ? STRATEGY_SUCCESS : STRATEGY_FAILURE;
    }
    return STRATEGY_FAILURE;
}

StrategyStatus Strategy_Init_Trailing(StrategyHandle handle, const PricePoint* ticks, int count, int in_position,
                                      double entry_price, double highest_price) {
    if (handle) {
        auto strategy = static_cast<trading::Strategy*>(handle);
        bool success = strategy->Init(ToHistory(ticks, count), in_position != 0, entry_price, highest_price);

        return success ? STRATEGY_SUCCESS : STRATEGY_FAILURE;
    }
    return STRATEGY_FAILURE;
}

void Strategy_SetInPosition(StrategyHandle handle, int in_position, double entry_price, double highest_price) {
    if (handle) {
        auto strategy = static_cast<trading::Strategy*>(handle);
        strategy->SetInPosition(in_position != 0, entry_price, highest_price);
    }
}

StrategyStatus Strategy_UpdatePrice(StrategyHandle handle, double price, long long timestamp) {
    if (handle) {
        auto strategy = static_cast<trading::Strategy*>(handle);
        bool success = strategy->UpdatePrice({timestamp, price});

        return success ? STRATEGY_SUCCESS : STRATEGY_FAILURE;
    }
    return STRATEGY_FAILURE;
}

StrategySignal Strategy_GetSignal(StrategyHandle handle) {
    if (!handle) return SIGNAL_INVALID;

    auto strategy = static_cast<trading::Strategy*>(handle);
    return strategy->GetSignal();
}

void Strategy_RetrySignal(StrategyHandle handle, StrategySignal signal) {
    if (handle) {
        auto strategy = static_cast<trading::Strategy*>(handle);
        strategy->RetrySignal(signal);
    }
}

}  // extern "C"
