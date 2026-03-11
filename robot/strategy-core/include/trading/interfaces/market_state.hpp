#pragma once

#include <vector>

#include "trading/types.hpp"

using std::vector;

namespace trading {

// Abstract contract for any market data provider.
class MarketState {
   public:
    virtual ~MarketState() = default;

    // Initializes the state with historical data.
    // Returns false if history is not in chronological order.
    virtual bool Init(const vector<PricePoint>& history) = 0;

    // Updates the state with a new price tick.
    // Returns false if the tick seems corrupted (e.g. timestamp in the past, non-positive price, or unrealistic price
    // jump).
    virtual bool UpdatePrice(const PricePoint& tick) = 0;

    // Returns the most recent price tick.
    virtual double GetCurrentPrice() const = 0;
};

}  // namespace trading
