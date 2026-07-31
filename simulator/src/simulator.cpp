#include "simulator.hpp"

#include <algorithm>
#include <cmath>
#include <ctime>
#include <fstream>
#include <iomanip>
#include <iostream>
#include <sstream>

using std::cerr;
using std::cout;
using std::endl;
using std::string;
using std::vector;

// ---------------------------------------------------------------------------
// Config parsing & validation
// ---------------------------------------------------------------------------

bool ParseFloat(const string& value, double& result) {
    try {
        size_t parsed = 0;
        result = std::stod(value, &parsed);
        return parsed == value.size() && std::isfinite(result);
    } catch (...) {
        return false;
    }
}

bool ParseLongLong(const string& value, long long& result) {
    try {
        size_t parsed = 0;
        result = std::stoll(value, &parsed);
        return parsed == value.size();
    } catch (...) {
        return false;
    }
}

bool ParseMomentumWindows(const string& spec, vector<MomentumWindow>& windows) {
    std::stringstream ss(spec);
    string token;
    while (std::getline(ss, token, ',')) {
        auto colon = token.find(':');
        if (colon == string::npos || colon != token.rfind(':')) return false;
        try {
            windows.push_back({std::stoll(token.substr(0, colon)), std::stod(token.substr(colon + 1))});
        } catch (...) {
            return false;
        }
    }
    return !windows.empty();
}

string ValidateConfig(const StrategyConfig& cfg) {
    if (cfg.momentum_windows.empty() || cfg.momentum_windows.size() > static_cast<size_t>(kMaxMomentumWindows)) {
        return "momentum windows count must be between 1 and " + std::to_string(kMaxMomentumWindows);
    }

    long long max_lookback = 0;
    for (const auto& window : cfg.momentum_windows) {
        if (window.lookback_seconds <= 0) return "momentum window lookback_seconds must be positive";
        max_lookback = std::max(max_lookback, window.lookback_seconds);
    }

    if (cfg.window_seconds <= max_lookback) {
        return "window_seconds must be greater than the largest momentum lookback_seconds";
    }
    if (cfg.stop_loss_pct <= 0.0) return "stop_loss_pct must be positive";

    if (cfg.strategy_type == StrategyType::kMomentumProfit && cfg.profit_target_pct <= 0.0) {
        return "profit_target_pct must be positive for momentum_profit";
    }
    if (cfg.strategy_type == StrategyType::kMomentumTrailing &&
        (cfg.activation_pct <= 0.0 || cfg.trailing_stop_pct <= 0.0)) {
        return "activation_pct and trailing_stop_pct must be positive for momentum_trailing";
    }

    return "";
}

// ---------------------------------------------------------------------------
// CSV I/O
// ---------------------------------------------------------------------------

vector<PriceTick> LoadPriceHistory(const string& input_file, const string& symbol, const string& start_period,
                                   const string& end_period, long long artificial_interval_seconds) {
    vector<PriceTick> history;

    string price_file = input_file.empty() ? kDefaultPriceDir + symbol + "_prices.csv" : input_file;
    std::ifstream file(price_file);
    if (!file.is_open()) {
        cerr << "Error: could not open price file " << price_file << endl;
        return history;
    }

    string line;
    if (!std::getline(file, line)) {
        cerr << "Error: price file is empty: " << price_file << endl;
        return history;
    }

    bool filter_by_period = !start_period.empty() && !end_period.empty();
    long long start_val = 0;
    long long end_val = 0;
    if (filter_by_period) {
        start_val = std::stoll(start_period.substr(0, 4)) * 100 + std::stoll(start_period.substr(5, 2));
        end_val = std::stoll(end_period.substr(0, 4)) * 100 + std::stoll(end_period.substr(5, 2));
    }

    long long line_number = 1;
    long long previous_timestamp = 0;
    double previous_price = 0.0;
    bool first_row = true;

    while (std::getline(file, line)) {
        ++line_number;
        if (line.empty()) continue;

        std::stringstream ss(line);
        string field, datetime_str;
        std::getline(ss, field, ',');         // unix_timestamp
        std::getline(ss, datetime_str, ',');  // datetime_utc

        try {
            long long timestamp = std::stoll(field);

            if (filter_by_period) {
                long long period_val =
                    std::stoll(datetime_str.substr(0, 4)) * 100 + std::stoll(datetime_str.substr(5, 2));
                if (period_val < start_val || period_val > end_val) continue;
            }

            for (int i = 0; i < 3; ++i) std::getline(ss, field, ',');  // skip open, high, low
            if (!std::getline(ss, field, ',')) continue;               // close
            double price = std::stod(field);

            if (price <= 0.0) {
                cerr << "Warning: skipping non-positive price at line " << line_number << endl;
                continue;
            }
            if (!first_row && timestamp < previous_timestamp) {
                cerr << "Error: price file is not sorted ascending at line " << line_number << endl;
                return {};
            }

            // Check for unrealistic price jumps
            if (!first_row) {
                if (std::abs((price - previous_price) / previous_price) > kMaxTickPriceChange) {
                    cerr << "Warning: skipping anomalous price jump at line " << line_number << endl;
                    continue;
                }
            }

            // Artificial Price Generation (Arithmetic Progression)
            if (!first_row) {
                long long gap = timestamp - previous_timestamp;
                if (gap > artificial_interval_seconds) {
                    long long num_steps = gap / artificial_interval_seconds;
                    double price_diff = price - previous_price;
                    double price_step = price_diff / num_steps;

                    for (long long j = 1; j < num_steps; ++j) {
                        long long t = previous_timestamp + (j * artificial_interval_seconds);
                        double p = previous_price + (j * price_step);
                        history.push_back({t, p});
                    }
                }
            }

            previous_timestamp = timestamp;
            previous_price = price;
            first_row = false;
            history.push_back({timestamp, price});
        } catch (const std::exception&) {
            // Ignore malformed rows (e.g. header repeated mid-file, truncated lines).
            continue;
        }
    }

    return history;
}

void WriteTradeLog(const string& path, const vector<Trade>& trades, const vector<PriceTick>& history) {
    std::ostream* out = &cout;
    std::ofstream file;

    if (!path.empty()) {
        file.open(path);
        if (!file.is_open()) {
            cerr << "Error: could not create trade log file " << path << endl;
            return;
        }
        out = &file;
    }

    auto format_ts = [](long long ts) {
        std::time_t t = static_cast<std::time_t>(ts);
        std::tm* tm = std::gmtime(&t);
        std::stringstream ss;
        ss << std::put_time(tm, "%Y-%m-%d %H:%M");
        return ss.str();
    };

    *out << "entry_timestamp,entry_date,entry_price,"
         << "exit_timestamp,exit_date,exit_price,pnl_pct,exit_reason\n";

    double accumulated_pnl = 1.0;

    // Write individual trades
    for (const auto& t : trades) {
        *out << t.entry_timestamp << "," << format_ts(t.entry_timestamp) << "," << std::fixed << std::setprecision(4)
             << t.entry_price << "," << t.exit_timestamp << "," << format_ts(t.exit_timestamp) << "," << t.exit_price
             << "," << std::fixed << std::setprecision(4) << t.pnl_pct << "," << t.exit_reason << "\n";
        accumulated_pnl = t.accumulated_pnl;
    }

    // Final row (accumulated): Always output if we have history, even if trades is empty
    if (!history.empty()) {
        long long first_ts = history.front().timestamp;
        long long last_ts = history.back().timestamp;
        *out << first_ts << "," << format_ts(first_ts) << "," << std::fixed << std::setprecision(4)
             << history.front().price << "," << last_ts << "," << format_ts(last_ts) << "," << history.back().price
             << "," << std::fixed << std::setprecision(4) << accumulated_pnl - 1.0 << ",end_of_period\n";
    }
}

// ---------------------------------------------------------------------------
// Strategy logic (sequential version)
// ---------------------------------------------------------------------------

double PriceSecondsAgo(const vector<PriceTick>& history, size_t upto_index, long long seconds_ago,
                       long long staleness_tolerance_seconds) {
    long long target = history[upto_index].timestamp - seconds_ago;

    // Binary search for the last tick with timestamp <= target, restricted to [0, upto_index].
    size_t lo = 0;
    size_t hi = upto_index;  // inclusive
    long long best_timestamp = -1;
    double best_price = 0.0;
    while (lo <= hi) {
        size_t mid = lo + (hi - lo) / 2;
        if (history[mid].timestamp <= target) {
            best_timestamp = history[mid].timestamp;
            best_price = history[mid].price;
            if (mid == upto_index) break;  // avoid unsigned underflow on hi = mid - 1 when mid == 0
            lo = mid + 1;
        } else {
            if (mid == 0) break;
            hi = mid - 1;
        }
    }

    if (best_timestamp < 0) return 0.0;                                       // no tick at or before the target
    if ((target - best_timestamp) > staleness_tolerance_seconds) return 0.0;  // too stale
    return best_price;
}

bool MomentumEntryTriggered(const vector<PriceTick>& history, size_t index, const StrategyConfig& cfg) {
    if ((history[index].timestamp - history[0].timestamp) < cfg.window_seconds) return false;

    double current = history[index].price;
    for (const auto& window : cfg.momentum_windows) {
        double past = PriceSecondsAgo(history, index, window.lookback_seconds, cfg.staleness_tolerance_seconds);

        // A non-positive past price means the lookback went beyond available history.
        if (past <= 0.0) {
            if (cfg.require_all) return false;  // AND: missing data fails the check.
            continue;                           // OR: skip this window and try the next.
        }

        double pct_change = (current - past) / past;
        bool triggered = pct_change >= window.threshold;

        if (cfg.require_all && !triggered) return false;  // AND: one failure means total failure.
        if (!cfg.require_all && triggered) return true;   // OR: one success means total success.
    }

    return cfg.require_all;  // AND: all windows passed. OR: none of the windows passed.
}

bool ExitTriggered(const StrategyConfig& cfg, double current_price, double entry_price, double highest_price,
                   string& reason) {
    if (cfg.strategy_type == StrategyType::kMomentumProfit) {
        if (current_price <= entry_price * (1.0 - cfg.stop_loss_pct)) {
            reason = "stop_loss";
            return true;
        }
        if (current_price >= entry_price * (1.0 + cfg.profit_target_pct)) {
            reason = "profit_target";
            return true;
        }
        return false;
    }

    // momentum_trailing: two-phase exit.
    double peak_gain = (highest_price - entry_price) / entry_price;
    if (peak_gain >= cfg.activation_pct) {
        // Phase 2: trailing stop.
        if (current_price <= highest_price * (1.0 - cfg.trailing_stop_pct)) {
            reason = "trailing_stop";
            return true;
        }
        return false;
    }

    // Phase 1: flat stop-loss.
    if (current_price <= entry_price * (1.0 - cfg.stop_loss_pct)) {
        reason = "stop_loss";
        return true;
    }
    return false;
}

vector<Trade> RunSimulation(const vector<PriceTick>& history, const StrategyConfig& cfg) {
    vector<Trade> trades;
    if (history.empty()) return trades;

    enum class State { kSearching, kInPosition };
    State state = State::kSearching;
    double entry_price = 0.0;
    long long entry_timestamp = 0;
    double highest_price = 0.0;
    double accumulated_pnl = 1.0;

    for (size_t i = 0; i < history.size(); ++i) {
        double current_price = history[i].price;

        if (state == State::kSearching) {
            if (MomentumEntryTriggered(history, i, cfg)) {
                state = State::kInPosition;
                entry_price = current_price;
                entry_timestamp = history[i].timestamp;
                highest_price = current_price;
            }
            continue;
        }

        // In position: track the peak price and evaluate exit rules.
        highest_price = std::max(highest_price, current_price);

        string reason;
        if (ExitTriggered(cfg, current_price, entry_price, highest_price, reason)) {
            double buy_price = entry_price * (1.0 + kExchangeTaxRate);
            double sell_price = current_price * (1.0 - kExchangeTaxRate);

            double pnl_pct = ((sell_price - buy_price) / buy_price);
            accumulated_pnl *= (1.0 + pnl_pct);
            trades.push_back(
                {entry_timestamp, entry_price, history[i].timestamp, current_price, pnl_pct, accumulated_pnl, reason});
            state = State::kSearching;
            entry_price = 0.0;
            highest_price = 0.0;
        }
    }

    // Force-close a still-open position at the last available price so the report
    // reflects unrealized P&L instead of silently dropping the trade.
    if (state == State::kInPosition) {
        double buy_price = entry_price * (1.0 + kExchangeTaxRate);
        double sell_price = history.back().price * (1.0 - kExchangeTaxRate);

        double pnl_pct = (sell_price - buy_price) / buy_price;
        accumulated_pnl *= (1.0 + pnl_pct);
        trades.push_back({entry_timestamp, entry_price, history.back().timestamp, history.back().price, pnl_pct,
                          accumulated_pnl, "end_of_data"});
    }

    return trades;
}
