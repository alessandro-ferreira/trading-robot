#pragma once

#ifdef __cplusplus
extern "C" {
#endif

typedef void* StrategyHandle;

typedef enum { STRATEGY_DUMMY = 0, STRATEGY_MOMENTUM = 1 } StrategyType;

#define MAX_MOMENTUM_WINDOWS 10

typedef struct {
    int lookback;
    double threshold;
} MomentumWindow;

typedef struct {
    StrategyType type;
    int window_size;
    MomentumWindow momentum_windows[MAX_MOMENTUM_WINDOWS];
    int num_momentum_windows;
    int momentum_require_all;  // 1 = AND (all windows must trigger), 0 = OR (any window triggers)
    double stop_loss_pct;      // Phase 1: exit if price drops this far below entry (e.g. 0.05 = 5%)
    double activation_pct;     // Threshold gain (from entry) that activates the trailing stop (e.g. 0.05 = 5%)
    double trailing_stop_pct;  // Phase 2: exit if price drops this far below peak (e.g. 0.03 = 3%)
} StrategyConfig;

// Creates and initializes a Strategy instance from the given configuration.
// Returns NULL on invalid or unrecognized config type.
StrategyHandle Strategy_Create(StrategyConfig config);
// Destroys the strategy instance and frees memory. Safe to call with NULL.
void Strategy_Destroy(StrategyHandle handle);
// Initializes the strategy state (history and position) after creation.
// Combines history loading and position restoration.
// highest_price: the highest price seen since the position was opened, as persisted by the caller.
//               Used to correctly restore the trailing-stop phase on restart. Pass 0 when not in position.
void Strategy_Init(StrategyHandle handle, const double* prices, int count, int in_position, double entry_price,
                   double highest_price);
// Feeds a live price tick. Also tracks the highest price seen while in position.
void Strategy_UpdatePrice(StrategyHandle handle, double price);
// Evaluates entry or exit rules and transitions internal state.
// Returns: 1.0 (Buy), -1.0 (Sell), 0.0 (Hold)
double Strategy_GetSignal(StrategyHandle handle);

#ifdef __cplusplus
}
#endif
