#include <getopt.h>

#include <algorithm>
#include <ctime>
#include <fstream>
#include <iomanip>
#include <iostream>
#include <sstream>
#include <string>
#include <unordered_set>
#include <vector>

using std::cerr;
using std::cout;
using std::endl;
using std::string;
using std::vector;

// ---------------------------------------------------------------------------
// Configuration constants
// ---------------------------------------------------------------------------

// Default directory containing historical price CSVs.
const string kDefaultPriceDir = "prices/";

// Maximum number of momentum windows a strategy can define (mirrors
// MAX_MOMENTUM_WINDOWS in robot/strategy-core/include/trading/types.h).
const int kMaxMomentumWindows = 10;

// Maximum tolerated gap (in seconds) between a requested lookback timestamp and the
// closest available price point. Mirrors MAX_LOOKBACK_STALENESS in sliding_window.cpp
const long long kMaxLookbackStaleness = 300;

// Maximum tolerated price jump percentage to discard wrong values.
// Set to 200% to only discard indisputable data corruption while keeping extreme but real events.
const double kMaxTickPriceChange = 2.0;

// Symbol validation set
const std::unordered_set<string> kKnownSymbols = {"BTC", "ETH", "BNB", "SOL", "XRP"};

// ---------------------------------------------------------------------------
// Data types
// ---------------------------------------------------------------------------

enum class StrategyType { kMomentumProfit, kMomentumTrailing };

struct MomentumWindow {
    long long lookback_seconds;
    double threshold;
};

// Field meanings intentionally mirror `StrategyConfig` in robot/strategy-core/include/trading/types.h
// so results can be compared against a live/production strategy configuration.
struct StrategyConfig {
    StrategyType strategy_type = StrategyType::kMomentumProfit;
    long long window_seconds = 0;
    vector<MomentumWindow> momentum_windows;
    bool require_all = false;

    double stop_loss_pct = 0.0;
    double profit_target_pct = 0.0;
    double activation_pct = 0.0;
    double trailing_stop_pct = 0.0;

    long long staleness_tolerance_seconds = kMaxLookbackStaleness;
};

struct PriceTick {
    long long timestamp;
    double price;
};

struct Trade {
    long long entry_timestamp;
    double entry_price;
    long long exit_timestamp;
    double exit_price;
    double pnl_pct;
    double accumulated_pnl;  // Accumulated total pnl
    string exit_reason;
};

// ---------------------------------------------------------------------------
// CSV I/O
// ---------------------------------------------------------------------------

// Loads price history from a CSV file with "unix_timestamp,datetime_utc,open,high,low,close"
// columns (see prices/*.csv), optionally filtered to [start_period, end_period] expressed as "YYYY-MM".
// artificial_interval_seconds: used to generate AP values between original csv prices.
vector<PriceTick> LoadPriceHistory(const string& path, const string& start_period, const string& end_period,
                                   long long artificial_interval_seconds) {
    vector<PriceTick> history;
    std::ifstream file(path);
    if (!file.is_open()) {
        std::cerr << "Error: could not open price file " << path << std::endl;
        return history;
    }

    string line;
    if (!std::getline(file, line)) {
        std::cerr << "Error: price file is empty: " << path << std::endl;
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
                std::cerr << "Warning: skipping non-positive price at line " << line_number << std::endl;
                continue;
            }
            if (!first_row && timestamp < previous_timestamp) {
                std::cerr << "Error: price file is not sorted ascending at line " << line_number << std::endl;
                return {};
            }

            // Check for unrealistic price jumps
            if (!first_row) {
                if (std::abs((price - previous_price) / previous_price) > kMaxTickPriceChange) {
                    std::cerr << "Warning: skipping anomalous price jump at line " << line_number << std::endl;
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
    std::ostream* out = &std::cout;
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

// Returns the most recent price at or before (history[upto_index].timestamp - seconds_ago),
// searching only ticks in [0, upto_index] to avoid look-ahead bias. Returns 0.0 if no
// such point exists within the staleness tolerance.
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

// Evaluates the momentum entry rule against the tick at `index`.
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

// Evaluates the configured exit rule (fixed profit or trailing stop) for an open
// position. Sets `reason` when triggered.
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

// Sequentially replays the price history and returns the resulting trade log.
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
            double pnl_pct = (current_price - entry_price) / entry_price;
            accumulated_pnl *= (1.0 + pnl_pct);
            trades.push_back(
                {entry_timestamp, entry_price, history[i].timestamp, current_price, pnl_pct, accumulated_pnl, reason}
            );
            state = State::kSearching;
            entry_price = 0.0;
            highest_price = 0.0;
        }
    }

    // Force-close a still-open position at the last available price so the report
    // reflects unrealized P&L instead of silently dropping the trade.
    if (state == State::kInPosition) {
        double pnl_pct = (history.back().price - entry_price) / entry_price;
        accumulated_pnl *= (1.0 + pnl_pct);
        trades.push_back({entry_timestamp, entry_price, history.back().timestamp, history.back().price, pnl_pct,
                          accumulated_pnl, "end_of_data"});
    }

    return trades;
}

// ---------------------------------------------------------------------------
// Config parsing & validation
// ---------------------------------------------------------------------------

bool ParseMomentumWindows(const string& spec, vector<MomentumWindow>& windows) {
    std::stringstream ss(spec);
    string token;
    while (std::getline(ss, token, ',')) {
        auto colon = token.find(':');
        if (colon == string::npos) return false;
        try {
            windows.push_back({std::stoll(token.substr(0, colon)), std::stod(token.substr(colon + 1))});
        } catch (...) {
            return false;
        }
    }
    return !windows.empty();
}

// Validates the configuration. Returns an empty string if valid, otherwise a
// human-readable description of the first validation failure encountered.
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
// CLI
// ---------------------------------------------------------------------------

void print_usage(const char* prog) {
    cout << "Usage: " << prog << " [options]\n"
         << "Options:\n"
         << "  -s, --symbol <sym>         Crypto symbol (e.g., BTC)\n"
         << "  -b, --begin <YYYY-MM>      Start period\n"
         << "  -e, --end <YYYY-MM>        End period\n"
         << "  -t, --type <type>          Strategy type: profit or trailing\n"
         << "  -w, --window <sec>         Window seconds\n"
         << "  -m, --momentum <spec>      Momentum windows (lookback:threshold,...)\n"
         << "  -a, --all                  Require all momentum windows (logical AND)\n"
         << "  -l, --loss <pct>           Stop loss percentage (e.g., 0.05 for 5%)\n"
         << "  -p, --profit <pct>         Profit target percentage (for profit type)\n"
         << "  -r, --trailing <act:stop>  Activation and trailing stop (for trailing type)\n"
         << "  -o, --output <file>        Output CSV trade log (default: stdout)\n"
         << "  -i, --interval <sec>       Artificial price generation interval in seconds\n"
         << "  -h, --help                 Display this help message\n\n"
         << "Example: \n"
         << prog << " -s BTC -b 2021-01 -e 2021-12 -t profit -w 21600 -m 10800:0.01,18000:0.02 -l 0.05 -p 0.10 -i 30\n"
         << endl;
}

int main(int argc, char** argv) {
    string symbol, begin, end, type_str, momentum_spec, output_file;
    StrategyConfig cfg;
    long long artificial_interval = 0;
    bool symbol_set = false, begin_set = false, end_set = false, type_set = false;
    bool win_set = false, mom_set = false, loss_set = false;

    static struct option long_options[] = {
        {"symbol", required_argument, 0, 's'},
        {"begin", required_argument, 0, 'b'},
        {"end", required_argument, 0, 'e'},
        {"type", required_argument, 0, 't'},
        {"window", required_argument, 0, 'w'},
        {"momentum", required_argument, 0, 'm'},
        {"all", no_argument, 0, 'a'},
        {"loss", required_argument, 0, 'l'},
        {"profit", required_argument, 0, 'p'},
        {"trailing", required_argument, 0, 'r'},
        {"output", required_argument, 0, 'o'},
        {"interval", required_argument, 0, 'i'},
        {"help", no_argument, 0, 'h'},
        {0, 0, 0, 0}
    };

    int opt;
    while ((opt = getopt_long(argc, argv, "s:b:e:t:w:m:al:p:r:o:i:h", long_options, nullptr)) != -1) {
        switch (opt) {
            case 's': symbol = optarg; symbol_set = true; break;
            case 'b': begin = optarg; begin_set = true; break;
            case 'e': end = optarg; end_set = true; break;
            case 't': type_str = optarg; type_set = true; break;
            case 'w': cfg.window_seconds = std::stoll(optarg); win_set = true; break;
            case 'm': momentum_spec = optarg; mom_set = true; break;
            case 'a': cfg.require_all = true; break;
            case 'l': cfg.stop_loss_pct = std::stod(optarg); loss_set = true; break;
            case 'p': cfg.profit_target_pct = std::stod(optarg); break;
            case 'r': {
                string val = optarg; auto c = val.find(':');
                if (c != string::npos) {
                    cfg.activation_pct = std::stod(val.substr(0, c));
                    cfg.trailing_stop_pct = std::stod(val.substr(c + 1));
                }
                break;
            }
            case 'o': output_file = optarg; break;
            case 'i': artificial_interval = std::stoll(optarg); break;
            case 'h': print_usage(argv[0]); return 0;
            default: print_usage(argv[0]); return 1;
        }
    }

    if (!symbol_set || !begin_set || !end_set || !type_set || !win_set || !mom_set || !loss_set) {
        cerr << "Error: missing required options." << endl;
        print_usage(argv[0]);
        return 1;
    }

    if (begin > end) {
        cerr << "Error: begin period (" << begin << ") must be before or equal to end period (" << end << ")." << endl;
        return 1;
    }

    if (type_str == "profit")
        cfg.strategy_type = StrategyType::kMomentumProfit;
    else if (type_str == "trailing")
        cfg.strategy_type = StrategyType::kMomentumTrailing;
    else {
        cerr << "Error: invalid type." << endl;
        return 1;
    }

    // Logic: Interval must be <= kMaxLookbackStaleness. Default to kMaxLookbackStaleness.
    if (artificial_interval <= 0 || artificial_interval > kMaxLookbackStaleness) {
        artificial_interval = kMaxLookbackStaleness;
    }

    if (!ParseMomentumWindows(momentum_spec, cfg.momentum_windows)) {
        cerr << "Error: invalid momentum spec." << endl;
        return 1;
    }

    string validation_error = ValidateConfig(cfg);
    if (!validation_error.empty()) {
        cerr << "Error: invalid configuration: " << validation_error << endl;
        return 1;
    }

    if (kKnownSymbols.find(symbol) == kKnownSymbols.end()) {
        cerr << "Warning: unknown symbol '" << symbol << "'" << endl;
    }

    string price_file = kDefaultPriceDir + symbol + "_prices.csv";
    vector<PriceTick> history = LoadPriceHistory(price_file, begin, end, artificial_interval);
    if (history.empty()) {
        cerr << "Error: no data." << endl;
        return 1;
    }

    vector<Trade> trades = RunSimulation(history, cfg);

    // Write the log file or to stdout
    WriteTradeLog(output_file, trades, history);

    return 0;
}
