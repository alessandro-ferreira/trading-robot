#pragma once

#ifdef __cplusplus
extern "C" {
#endif

#define MAX_MOMENTUM_WINDOWS 10

// Defines the status code returned by API functions that can fail.
typedef int StrategyStatus;
#define STRATEGY_SUCCESS 1
#define STRATEGY_FAILURE 0

// Defines the signal returned by the strategy evaluation.
typedef int Signal;
#define SIGNAL_BUY 1
#define SIGNAL_SELL -1
#define SIGNAL_HOLD 0

// Opaque handle to a strategy instance.
typedef void* StrategyHandle;

// Enumeration of supported strategy types.
typedef enum { STRATEGY_DUMMY = 0, STRATEGY_MOMENTUM_PROFIT = 1, STRATEGY_MOMENTUM_TRAILING = 2 } StrategyType;

// Defines parameters for a single momentum condition.
typedef struct {
    long long lookback_seconds;  // Time duration (in seconds) to look back for momentum comparison
    double threshold;            // Minimum percentage change required (e.g. 0.01 = 1%)
} MomentumWindow;

// A single timestamped price observation.
typedef struct {
    long long timestamp;  // Unix timestamp in seconds
    double price;         // Price at the given timestamp
} PricePoint;

typedef struct {
    StrategyType type;
    long long window_seconds;  // Duration (in seconds) of price history to retain
    MomentumWindow momentum_windows[MAX_MOMENTUM_WINDOWS];
    int num_momentum_windows;
    int momentum_require_all;  // 1 = AND (all windows must trigger), 0 = OR (any window triggers)
    double stop_loss_pct;      // Common: exit if price drops this far below entry (e.g. 0.05 = 5%)
    double profit_target_pct;  // Profit: Exit if price rises this far above entry (e.g. 0.10 = 10%)
    double activation_pct;     // Trailing: Threshold gain that activates the trailing stop
    double trailing_stop_pct;  // Trailing: Exit if price drops this far below peak
} StrategyConfig;

#ifdef __cplusplus
}
#endif
