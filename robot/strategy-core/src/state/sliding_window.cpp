#include "trading/state/sliding_window.hpp"

namespace trading {

SlidingWindowPriceState::SlidingWindowPriceState(size_t window_size) : window_size_(window_size), current_price_(0.0) {}

void SlidingWindowPriceState::UpdatePrice(double price) {
    current_price_ = price;
    prices_.push_back(price);
    if (prices_.size() > window_size_) {
        prices_.pop_front();
    }
}

double SlidingWindowPriceState::GetCurrentPrice() const { return current_price_; }

double SlidingWindowPriceState::GetPriceAgo(size_t ticks_ago) const {
    if (ticks_ago >= prices_.size()) {
        return 0.0;  // Not enough history
    }
    return prices_[prices_.size() - 1 - ticks_ago];
}

bool SlidingWindowPriceState::IsReady() const { return prices_.size() >= window_size_; }

}  // namespace trading
