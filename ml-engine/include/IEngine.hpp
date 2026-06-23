#pragma once

#include <optional>
#include <string>
#include <vector>

using std::optional;
using std::string;
using std::vector;

namespace trading::ml {

// MomentumWindow represents a lookback period and threshold for momentum calculation.
struct MomentumWindow {
    int32_t lookback_seconds;
    double threshold;
};

// MomentumParams encapsulates the configuration for momentum-based strategy variants.
struct MomentumParams {
    string label;
    int32_t window_seconds;
    vector<MomentumWindow> windows;
    bool require_all;
    double stop_loss_pct;
    optional<double> profit_target_pct;
    optional<double> activation_pct;
    optional<double> trailing_stop_pct;
};

// StrategyUpdate contains the high-level instructions to be sent to the Go Bot.
struct StrategyUpdate {
    string exchange;
    string symbol;
    string strategy_type;
    bool enabled;
    MomentumParams momentum_params;
};

// RiskUpdate contains the instructions to update pair-specific risk parameters.
struct RiskUpdate {
    string exchange;
    string symbol;
    double allocated_budget;
    double max_asset_units;
};

// IEngine is the abstract interface for intelligence providers.
// Both the public template and the private ML core must implement this interface.
class IEngine {
   public:
    virtual ~IEngine() = default;

    // Initialize the engine with any required models or configuration.
    virtual bool Init() = 0;

    // GenerateUpdate evaluates market conditions for a specific pair and returns
    // the desired strategy configuration.
    virtual StrategyUpdate GenerateStrategyUpdate(const string& exchange, const string& symbol) = 0;

    // GenerateRiskUpdate evaluates market conditions and returns the desired
    // risk configuration for a specific pair.
    virtual RiskUpdate GenerateRiskUpdate(const string& exchange, const string& symbol) = 0;

    // GetName returns the provider identifier (e.g., "template_v1" or "private_core_v2").
    virtual string GetName() const = 0;
};

}  // namespace trading::ml
