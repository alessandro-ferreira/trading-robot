#pragma once

namespace trading {

// Abstract contract for any market data provider.
class MarketState {
   public:
    virtual ~MarketState() = default;

    // Updates the state with a new price tick.
    virtual void UpdatePrice(double price) = 0;

    // Returns the most recent price tick.
    virtual double GetCurrentPrice() const = 0;
};

}  // namespace trading
