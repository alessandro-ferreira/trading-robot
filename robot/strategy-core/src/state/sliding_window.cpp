#include "trading/state/sliding_window.hpp"

#include <algorithm>

using std::vector;

namespace trading {

SlidingWindowPriceState::SlidingWindowPriceState(long long window_seconds)
    : window_seconds_(window_seconds), current_timestamp_(0), is_ready_(false) {}

bool SlidingWindowPriceState::Init(const vector<PricePoint>& history) {
    entries_.clear();
    is_ready_ = false;
    current_timestamp_ = 0;

    if (history.empty()) {
        return true;
    }

    // Validate that history is sorted by timestamp
    for (size_t i = 1; i < history.size(); ++i) {
        if (history[i].timestamp < history[i - 1].timestamp) {
            return false;
        }
    }

    // Efficiently bulk-load all but the last point without triggering logic.
    if (history.size() > 1) {
        entries_.insert(entries_.end(), history.begin(), history.end() - 1);
    }

    // Process the final point using the main update logic to correctly set readiness and perform the initial eviction.
    return UpdatePrice(history.back());
}

bool SlidingWindowPriceState::UpdatePrice(const PricePoint& tick) {
    // Integrity check: ignore out-of-order ticks, which would corrupt state.
    if (tick.timestamp < current_timestamp_) {
        return false;
    }

    current_timestamp_ = tick.timestamp;
    entries_.push_back(tick);

    long long cutoff = current_timestamp_ - window_seconds_;

    // Evict entries that are too old to be needed for any lookback.
    while (entries_.size() > 1 && entries_.front().timestamp < cutoff) {
        entries_.pop_front();
    }

    // Set readiness state.
    if (entries_.size() >= 2 && (current_timestamp_ - entries_.front().timestamp >= window_seconds_)) {
        is_ready_ = true;
    } else {
        is_ready_ = false;
    }

    return true;
}

double SlidingWindowPriceState::GetCurrentPrice() const {
    if (entries_.empty()) return 0.0;
    return entries_.back().price;
}

double SlidingWindowPriceState::GetPriceSecondsAgo(long long seconds_ago) const {
    if (entries_.empty()) return 0.0;
    long long target = current_timestamp_ - seconds_ago;

    // Use std::upper_bound for an efficient O(log n) binary search.
    // It finds the first element whose timestamp is strictly greater than the target.
    // The custom comparator tells upper_bound how to compare a PricePoint with our target timestamp.
    auto it = std::upper_bound(entries_.begin(), entries_.end(), target, [](long long target_ts, const PricePoint& p) {
        // This lambda defines "less than" for the search.
        // upper_bound finds the first element 'p' for which 'target_ts < p.timestamp' is true.
        return target_ts < p.timestamp;
    });

    // If the iterator is at the beginning, it means no entry has a timestamp <= target.
    if (it == entries_.begin()) {
        return 0.0;
    }

    // The element we want is the one just before the iterator 'it',
    // which is the last element with a timestamp <= target.
    return std::prev(it)->price;
}

bool SlidingWindowPriceState::IsReady() const { return is_ready_; }

}  // namespace trading
