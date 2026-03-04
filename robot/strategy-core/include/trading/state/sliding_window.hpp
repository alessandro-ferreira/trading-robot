#pragma once

#include <deque>
#include <vector>

#include "trading/interfaces/market_state.hpp"

using std::deque;
using std::vector;

namespace trading {

// A MarketState implementation that retains price history for a configurable time window.
// Prices older than window_duration_seconds are evicted to bound memory usage.
// IsReady() returns true once at least window_duration_seconds of history has been accumulated,
// guaranteeing that all configured lookbacks can be resolved.
class SlidingWindowPriceState : public MarketState {
   public:
    explicit SlidingWindowPriceState(long long window_duration_seconds);

    bool Init(const vector<PricePoint>& history) override;
    bool UpdatePrice(const PricePoint& tick) override;
    double GetCurrentPrice() const override;

    // Returns true once the accumulated history spans at least window_duration_seconds.
    bool IsReady() const;

    // Returns the most recent price recorded at or before (current_timestamp - seconds_ago).
    // Returns 0.0 if no such entry exists in the retained window.
    double GetPriceSecondsAgo(long long seconds_ago) const;

   private:
    deque<PricePoint> entries_;
    long long window_seconds_;
    long long current_timestamp_;
    bool is_ready_;
};

}  // namespace trading
