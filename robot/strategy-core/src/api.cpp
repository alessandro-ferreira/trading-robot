#include "trading/api.h"

#include <algorithm>  // for std::max
#include <memory>
#include <vector>

#include "trading/rules/momentum.hpp"
#include "trading/rules/risk_management.hpp"
#include "trading/state/sliding_window.hpp"
#include "trading/strategy.hpp"

extern "C" {

StrategyHandle Strategy_Create(StrategyConfig config) {
    if (config.type == STRATEGY_DUMMY) {
        // A minimal strategy that does nothing, for integration testing.
        auto state = std::make_unique<trading::SlidingWindowPriceState>(1);
        std::vector<std::unique_ptr<trading::EntryRule>> entry_rules;
        std::vector<std::unique_ptr<trading::ExitRule>> exit_rules;
        // Provide dummy values for profit/trailing parameters for the dummy strategy.
        return new trading::Strategy(std::move(state), std::move(entry_rules), std::move(exit_rules));
    }

    if (config.type == STRATEGY_MOMENTUM) {
        if (config.num_momentum_windows > MAX_MOMENTUM_WINDOWS || config.num_momentum_windows < 0) {
            return nullptr;  // Invalid config
        }

        if (config.stop_loss_pct <= 0.0) {
            return nullptr;  // Stop loss must be positive
        }

        if (config.activation_pct <= 0.0) {
            return nullptr;  // Activation threshold must be positive
        }

        if (config.trailing_stop_pct <= 0.0) {
            return nullptr;  // Trailing stop distance must be positive
        }

        std::vector<std::pair<int, double>> lookback_thresholds;
        int max_lookback = 0;
        for (int i = 0; i < config.num_momentum_windows; ++i) {
            if (config.momentum_windows[i].lookback > max_lookback) {
                max_lookback = config.momentum_windows[i].lookback;
            }
            lookback_thresholds.push_back({config.momentum_windows[i].lookback, config.momentum_windows[i].threshold});
        }

        if (config.window_size <= max_lookback) {
            return nullptr;  // Window size must be strictly greater than max lookback to calculate change
        }

        auto state = std::make_unique<trading::SlidingWindowPriceState>(config.window_size);

        std::vector<std::unique_ptr<trading::EntryRule>> entry_rules;
        bool require_all = (config.momentum_require_all != 0);
        entry_rules.push_back(std::make_unique<trading::MomentumEntryRule>(lookback_thresholds, require_all));

        std::vector<std::unique_ptr<trading::ExitRule>> exit_rules;
        exit_rules.push_back(std::make_unique<trading::RiskManagementExitRule>(
            config.stop_loss_pct, config.activation_pct, config.trailing_stop_pct));

        return new trading::Strategy(std::move(state), std::move(entry_rules), std::move(exit_rules));
    }

    return nullptr;
}

void Strategy_Destroy(StrategyHandle handle) {
    if (handle) {
        delete static_cast<trading::Strategy*>(handle);
    }
}

void Strategy_Init(StrategyHandle handle, const double* prices, int count, int in_position, double entry_price,
                   double highest_price) {
    if (handle) {
        static_cast<trading::Strategy*>(handle)->Init(prices, count, in_position != 0, entry_price, highest_price);
    }
}

void Strategy_UpdatePrice(StrategyHandle handle, double price) {
    if (handle) {
        static_cast<trading::Strategy*>(handle)->UpdatePrice(price);
    }
}

double Strategy_GetSignal(StrategyHandle handle) {
    if (!handle) return 0.0;
    return static_cast<trading::Strategy*>(handle)->GetSignal();
}
}
