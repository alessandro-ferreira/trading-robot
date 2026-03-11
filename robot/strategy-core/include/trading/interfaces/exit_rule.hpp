#pragma once

namespace trading {

class MarketState;  // Forward declaration

class ExitRule {
   public:
    virtual ~ExitRule() = default;

    // Returns true if the exit condition is satisfied.
    // entry_price: price at which the position was opened.
    // highest_price: highest price seen since the position was opened.
    virtual bool Check(const MarketState& state, double entry_price, double highest_price) = 0;
};

}  // namespace trading
