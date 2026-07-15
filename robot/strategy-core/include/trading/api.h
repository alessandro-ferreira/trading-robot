#pragma once

#ifdef __cplusplus
extern "C" {
#endif

#include "trading/types.h"

// Creates and initializes a Strategy instance from the given configuration.
// Returns NULL on invalid/unrecognized config type or invalid parameters (e.g. windows not greater than lookbacks,
// non-positive percentages, etc.).
StrategyHandle Strategy_Create(StrategyConfig config);
// Updates the hyperparameters of the strategy without clearing history.
// Returns STRATEGY_FAILURE if the new config is invalid (e.g. invalid type or parameters) or if the handle is NULL.
StrategyStatus Strategy_UpdateConfig(StrategyHandle handle, StrategyConfig config);
// Destroys the strategy instance and frees memory. Safe to call with NULL.
void Strategy_Destroy(StrategyHandle handle);

// Initializes state for a fixed profit strategy.
// Returns STRATEGY_FAILURE if the history is not in chronological order or has non-positive price.
StrategyStatus Strategy_Init_Profit(StrategyHandle handle, const PricePoint* ticks, int count, int in_position,
                                    double entry_price);
// Initializes state for a trailing stop strategy.
// highest_price: The highest price seen since the position was opened, required to restore the trailing stop phase.
// Returns STRATEGY_FAILURE if the history is not in chronological order or has non-positive price.
StrategyStatus Strategy_Init_Trailing(StrategyHandle handle, const PricePoint* ticks, int count, int in_position,
                                      double entry_price, double highest_price);

// Sets the strategy's position state.
// Used to align the strategy with the actual position after a filled order.
void Strategy_SetInPosition(StrategyHandle handle, int in_position, double entry_price, double highest_price);

// Feeds a live price tick. Also tracks the highest price seen while in position.
// Returns STRATEGY_FAILURE if the tick seems corrupted (e.g. timestamp in the past, non-positive price, or unrealistic
// price jump).
StrategyStatus Strategy_UpdatePrice(StrategyHandle handle, double price, long long timestamp);

// Evaluates entry or exit rules and transitions internal state.
// Must be called exactly once per tick after UpdatePrice.
// Returns SIGNAL_INVALID if the strategy is in a corrupted state (e.g. entry price not set while ACTIVE).
StrategySignal Strategy_GetSignal(StrategyHandle handle);

// Retries the signal. It should be used in case of error when placing an order.
// Only valid for order placement signals (BUY or SELL) when pending confirmation, otherwise is ignored.
void Strategy_RetrySignal(StrategyHandle handle, StrategySignal signal);

#ifdef __cplusplus
}
#endif
