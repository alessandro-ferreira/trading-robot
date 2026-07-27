#ifndef SIMULATOR_HPP
#define SIMULATOR_HPP

#include <string>
#include <vector>

using std::string;
using std::vector;

// ---------------------------------------------------------------------------
// Configuration constants
// ---------------------------------------------------------------------------

// Default directory containing historical price CSVs in the format <SYMBOL>_prices.csv (e.g., BTC_prices.csv).
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

// 0.2% tax on each buy/sell transaction
const double kExchangeTaxRate = 0.002;

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
// Function declarations
// ---------------------------------------------------------------------------

// Parses a momentum spec string and returns a vector of MomentumWindow objects.
bool ParseMomentumWindows(const string& spec, vector<MomentumWindow>& windows);

// Validates the configuration. Returns an empty string if valid, otherwise a
// human-readable description of the first validation failure encountered.
string ValidateConfig(const StrategyConfig& cfg);

// Loads price history from a CSV file with "unix_timestamp,datetime_utc,open,high,low,close"
// columns (see prices/*.csv), optionally filtered to [start_period, end_period] expressed as "YYYY-MM".
// artificial_interval_seconds: used to generate AP values between original csv prices.
vector<PriceTick> LoadPriceHistory(const string& input_file, const string& symbol, const string& start_period,
                                   const string& end_period, long long artificial_interval_seconds);

// Writes the trade log to a CSV file with columns:
// entry_timestamp,entry_date,entry_price,exit_timestamp,exit_date,exit_price,pnl_pct,exit_reason
void WriteTradeLog(const string& path, const vector<Trade>& trades, const vector<PriceTick>& history);

// Returns the most recent price at or before (history[upto_index].timestamp - seconds_ago),
// searching only ticks in [0, upto_index] to avoid look-ahead bias. Returns 0.0 if no
// such point exists within the staleness tolerance.
double PriceSecondsAgo(const vector<PriceTick>& history, size_t upto_index, long long seconds_ago,
                       long long staleness_tolerance_seconds);

// Evaluates the momentum entry rule against the tick at `index`.
bool MomentumEntryTriggered(const vector<PriceTick>& history, size_t index, const StrategyConfig& cfg);

// Evaluates the configured exit rule (fixed profit or trailing stop) for an open
// position. Sets `reason` when triggered.
bool ExitTriggered(const StrategyConfig& cfg, double current_price, double entry_price, double highest_price,
                   string& reason);

// Sequentially replays the price history and returns the resulting trade log.
vector<Trade> RunSimulation(const vector<PriceTick>& history, const StrategyConfig& cfg);

#endif  // SIMULATOR_HPP
