#pragma once

#include <cstddef>  // For size_t
#include <deque>

#include "trading/interfaces/market_state.hpp"

namespace trading {

class SlidingWindowPriceState : public MarketState {
   public:
    explicit SlidingWindowPriceState(size_t window_size);

    void UpdatePrice(double price) override;
    double GetCurrentPrice() const override;

    // Returns true once the deque has been filled to window_size, meaning all lookbacks are valid.
    bool IsReady() const;
    // Returns the price ticks_ago steps back; ticks_ago=1 is the previous tick, ticks_ago=0 is current.
    // Returns 0.0 if there is insufficient history.
    double GetPriceAgo(size_t ticks_ago) const;

   private:
    std::deque<double> prices_;
    size_t window_size_;
    double current_price_;
};

}  // namespace trading
