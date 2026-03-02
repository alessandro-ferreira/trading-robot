#pragma once

#ifdef __cplusplus
extern "C" {
#endif

typedef void* StrategyHandle;

typedef enum { STRATEGY_DUMMY = 0, STRATEGY_MOMENTUM_PROFIT = 1, STRATEGY_MOMENTUM_TRAILING = 2 } StrategyType;

#define MAX_MOMENTUM_WINDOWS 10

typedef struct {
    int lookback;      // Number of ticks back to compare current price against
    double threshold;  // Minimum percentage change required (e.g. 0.01 = 1%)
} MomentumWindow;

typedef struct {
    StrategyType type;
    int window_size;
    MomentumWindow momentum_windows[MAX_MOMENTUM_WINDOWS];
    int num_momentum_windows;
    int momentum_require_all;  // 1 = AND (all windows must trigger), 0 = OR (any window triggers)
    double stop_loss_pct;      // Common: exit if price drops this far below entry (e.g. 0.05 = 5%)
    double profit_target_pct;  // Profit: Exit if price rises this far above entry (e.g. 0.10 = 10%)
    double activation_pct;     // Trailing: Threshold gain that activates the trailing stop
    double trailing_stop_pct;  // Trailing: Exit if price drops this far below peak
} StrategyConfig;

// Creates and initializes a Strategy instance from the given configuration.
// Returns NULL on invalid or unrecognized config type.
StrategyHandle Strategy_Create(StrategyConfig config);
// Destroys the strategy instance and frees memory. Safe to call with NULL.
void Strategy_Destroy(StrategyHandle handle);

// Initializes state for a fixed profit strategy.
void Strategy_Init_Profit(StrategyHandle handle, const double* prices, int count, int in_position, double entry_price);
// Initializes state for a trailing stop strategy.
// highest_price: The highest price seen since the position was opened, required to restore the trailing stop phase.
void Strategy_Init_Trailing(StrategyHandle handle, const double* prices, int count, int in_position, double entry_price,
                            double highest_price);

// Feeds a live price tick. Also tracks the highest price seen while in position.
void Strategy_UpdatePrice(StrategyHandle handle, double price);

// Evaluates entry or exit rules and transitions internal state.
// Returns: 1.0 (Buy), -1.0 (Sell), 0.0 (Hold)
double Strategy_GetSignal(StrategyHandle handle);

#ifdef __cplusplus
}
#endif
