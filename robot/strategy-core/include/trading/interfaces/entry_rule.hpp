#pragma once

namespace trading {

class MarketState;  // Forward declaration

class EntryRule {
   public:
    virtual ~EntryRule() = default;
    // Returns true if the entry condition is satisfied for the current market state.
    virtual bool Check(const MarketState& state) = 0;
};

}  // namespace trading
