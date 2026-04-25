#include "TemplateEngine.hpp"

using std::string;

namespace trading::ml {

StrategyUpdate TemplateEngine::GenerateStrategyUpdate(const string& exchange, const string& symbol) {
    StrategyUpdate update;
    update.exchange = exchange;
    update.symbol = symbol;
    update.enabled = true;

    // Mirroring values from migration 000015_insert_strategies.up.sql
    if (symbol == "BTC/USDT") {
        update.strategy_type = "momentum_trailing";
        update.momentum_params.label = "default";
        update.momentum_params.window_seconds = 10;
        update.momentum_params.require_all = false;
        update.momentum_params.stop_loss_pct = 0.1;
        update.momentum_params.activation_pct = 0.05;
        update.momentum_params.trailing_stop_pct = 0.02;
        update.momentum_params.windows.push_back({5, 0.0001});

    } else if (symbol == "ETH/USDT") {
        update.strategy_type = "momentum_profit";
        update.momentum_params.label = "default";
        update.momentum_params.window_seconds = 10;
        update.momentum_params.require_all = true;
        update.momentum_params.stop_loss_pct = 0.1;
        update.momentum_params.profit_target_pct = 0.05;
        update.momentum_params.windows.push_back({5, 0.0001});
        update.momentum_params.windows.push_back({6, 0.0002});
        update.momentum_params.windows.push_back({8, 0.0003});

    } else if (symbol == "LTC/USDT") {
        update.strategy_type = "dummy";
        // Dummy strategy requires minimal momentum params structure for valid RPC
        update.momentum_params.label = "default";
        update.momentum_params.window_seconds = 0;
        update.momentum_params.require_all = false;
        update.momentum_params.stop_loss_pct = 0.0;

    } else {
        // Fallback or disable for unknown symbols
        update.strategy_type = "dummy";
        update.enabled = false;
        update.momentum_params.label = "unknown";
        update.momentum_params.window_seconds = 0;
        update.momentum_params.require_all = false;
        update.momentum_params.stop_loss_pct = 0.0;
    }

    return update;
}

RiskUpdate TemplateEngine::GenerateRiskUpdate(const string& exchange, const string& symbol) {
    RiskUpdate update;
    update.exchange = exchange;
    update.symbol = symbol;

    // Mirroring values from migration 000016_insert_risks.up.sql
    if (symbol == "BTC/USDT") {
        update.risk_per_trade = 100.0;
        update.max_position_size = 1.0;
    } else if (symbol == "ETH/USDT") {
        update.risk_per_trade = 50.0;
        update.max_position_size = 10.0;
    } else if (symbol == "LTC/USDT") {
        update.risk_per_trade = 25.0;
        update.max_position_size = 5.0;
    } else {
        // Safe defaults for unknown pairs
        update.risk_per_trade = 0.0;
        update.max_position_size = 0.0;
    }

    return update;
}

}  // namespace trading::ml
