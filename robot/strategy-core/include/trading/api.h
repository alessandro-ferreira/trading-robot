#pragma once

#ifdef __cplusplus
extern "C" {
#endif

#include "trading/types.h"

// Creates and initializes a Strategy instance from the given configuration.
// Returns NULL on invalid/unrecognized config type or invalid parameters.
StrategyHandle Strategy_Create(StrategyConfig config);
// Updates the hyperparameters of the strategy without clearing history.
// Returns STRATEGY_FAILURE if the new config is invalid (e.g. invalid type or parameters) or if the handle is NULL.
StrategyStatus Strategy_UpdateConfig(StrategyHandle handle, StrategyConfig config);
// Destroys the strategy instance and frees memory. Safe to call with NULL.
void Strategy_Destroy(StrategyHandle handle);

// Initializes state for a fixed profit strategy.
// Returns STRATEGY_FAILURE if the history is not in chronological order.
StrategyStatus Strategy_Init_Profit(StrategyHandle handle, const PricePoint* ticks, int count, StrategyState state,
                                    double entry_price);
// Initializes state for a trailing stop strategy.
// highest_price: The highest price seen since the position was opened, required to restore the trailing stop phase.
// Returns STRATEGY_FAILURE if the history is not in chronological order.
StrategyStatus Strategy_Init_Trailing(StrategyHandle handle, const PricePoint* ticks, int count, StrategyState state,
                                      double entry_price, double highest_price);

// Feeds a live price tick. Also tracks the highest price seen while in position.
// Returns STRATEGY_FAILURE if the tick seems corrupted (e.g. timestamp in the past, non-positive price, or unrealistic
// price jump).
StrategyStatus Strategy_UpdatePrice(StrategyHandle handle, double price, long long timestamp);

// Returns the current state of the strategy.
StrategyState Strategy_GetState(StrategyHandle handle);

// Evaluates entry or exit rules and transitions internal state.
Signal Strategy_GetSignal(StrategyHandle handle);
// Confirms that a pending signal has been filled, allowing the strategy to transition state.
void Strategy_ConfirmSignal(StrategyHandle handle, Signal signal, double fill_price);
// Cancels a pending signal, returning the strategy to its previous state.
void Strategy_CancelSignal(StrategyHandle handle, Signal signal);
// Resets the strategy to IDLE state (e.g. on order cancellation or external fill).
void Strategy_ResetSignal(StrategyHandle handle);

#ifdef __cplusplus
}
#endif
