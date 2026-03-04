#pragma once

#ifdef __cplusplus
extern "C" {
#endif

#include "trading/types.h"

// Creates and initializes a Strategy instance from the given configuration.
// Returns NULL on invalid/unrecognized config type or invalid parameters.
StrategyHandle Strategy_Create(StrategyConfig config);
// Destroys the strategy instance and frees memory. Safe to call with NULL.
void Strategy_Destroy(StrategyHandle handle);

// Initializes state for a fixed profit strategy.
// Returns STRATEGY_FAILURE if the history is not in chronological order.
StrategyStatus Strategy_Init_Profit(StrategyHandle handle, const PricePoint* ticks, int count, int in_position,
                                    double entry_price);
// Initializes state for a trailing stop strategy.
// highest_price: The highest price seen since the position was opened, required to restore the trailing stop phase.
// Returns STRATEGY_FAILURE if the history is not in chronological order.
StrategyStatus Strategy_Init_Trailing(StrategyHandle handle, const PricePoint* ticks, int count, int in_position,
                                      double entry_price, double highest_price);

// Feeds a live price tick. Also tracks the highest price seen while in position.
// Returns STRATEGY_FAILURE if the timestamp is older than the last received tick.
StrategyStatus Strategy_UpdatePrice(StrategyHandle handle, double price, long long timestamp);

// Evaluates entry or exit rules and transitions internal state.
// Returns: SIGNAL_BUY (1), SIGNAL_SELL (-1), SIGNAL_HOLD (0)
Signal Strategy_GetSignal(StrategyHandle handle);

#ifdef __cplusplus
}
#endif
