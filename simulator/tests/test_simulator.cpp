#include <gtest/gtest.h>

#include <cstdio>
#include <fstream>

#include "simulator.hpp"

namespace {

TEST(SimulatorTest, ParseFloat) {
    double value = 0.0;
    EXPECT_TRUE(ParseFloat("0.05", value));
    EXPECT_DOUBLE_EQ(value, 0.05);

    EXPECT_FALSE(ParseFloat("", value));
    EXPECT_FALSE(ParseFloat("abc", value));
    EXPECT_FALSE(ParseFloat("0.05x", value));
    EXPECT_FALSE(ParseFloat("inf", value));
}

TEST(SimulatorTest, ParseLongLong) {
    long long value = 0;
    EXPECT_TRUE(ParseLongLong("1234567890", value));
    EXPECT_EQ(value, 1234567890);

    EXPECT_FALSE(ParseLongLong("", value));
    EXPECT_FALSE(ParseLongLong("abc", value));
    EXPECT_FALSE(ParseLongLong("123x", value));
    EXPECT_FALSE(ParseLongLong("123.45", value));
}

TEST(SimulatorTest, ParseMomentumWindows) {
    std::vector<MomentumWindow> windows;
    EXPECT_TRUE(ParseMomentumWindows("1000:0.01,2000:0.02", windows));
    ASSERT_EQ(windows.size(), 2);
    EXPECT_EQ(windows[0].lookback_seconds, 1000);
    EXPECT_DOUBLE_EQ(windows[0].threshold, 0.01);
    EXPECT_EQ(windows[1].lookback_seconds, 2000);
    EXPECT_DOUBLE_EQ(windows[1].threshold, 0.02);

    EXPECT_FALSE(ParseMomentumWindows("invalid", windows));
    EXPECT_FALSE(ParseMomentumWindows("1000:", windows));
    EXPECT_FALSE(ParseMomentumWindows("abc:0.01", windows));
    EXPECT_FALSE(ParseMomentumWindows("1000:0.01:0.02", windows));
}

TEST(SimulatorTest, ValidateConfig) {
    StrategyConfig cfg;
    // Missing windows
    EXPECT_NE(ValidateConfig(cfg), "");

    cfg.momentum_windows = {{1000, 0.01}};
    cfg.window_seconds = 2000;
    cfg.stop_loss_pct = 0.05;
    cfg.profit_target_pct = 0.10;
    EXPECT_EQ(ValidateConfig(cfg), "");

    // window_seconds <= max_lookback
    cfg.window_seconds = 1000;
    EXPECT_NE(ValidateConfig(cfg), "");

    // kMomentumProfit with invalid profit
    cfg.strategy_type = StrategyType::kMomentumProfit;
    cfg.profit_target_pct = 0.0;
    cfg.window_seconds = 2000;
    EXPECT_NE(ValidateConfig(cfg), "");

    // kMomentumTrailing with invalid params
    cfg.strategy_type = StrategyType::kMomentumTrailing;
    cfg.activation_pct = 0.0;
    cfg.trailing_stop_pct = 0.05;
    EXPECT_NE(ValidateConfig(cfg), "");

    cfg.activation_pct = 0.05;
    cfg.trailing_stop_pct = 0.0;
    EXPECT_NE(ValidateConfig(cfg), "");
}

// Helper to create a temporary price CSV
void CreateTempPriceFile(const std::string& filename, const std::vector<std::string>& lines) {
    std::ofstream file(filename);
    file << "unix_timestamp,datetime_utc,open,high,low,close\n";
    for (const auto& line : lines) {
        file << line << "\n";
    }
}

TEST(SimulatorTest, LoadPriceHistoryBasic) {
    std::string filename = "test_prices.csv";
    CreateTempPriceFile(filename,
                        {"1609459200,2021-01-01 00:00,10,11,9,10.5", "1609459260,2021-01-01 00:01,10.5,12,10,11.0"});

    auto history = LoadPriceHistory(filename, "", "", "", 10);
    ASSERT_EQ(history.size(), 7);  // 1609459200, 210, 220, 230, 240, 250, 260
    EXPECT_EQ(history[0].timestamp, 1609459200);
    EXPECT_DOUBLE_EQ(history[0].price, 10.5);
    EXPECT_EQ(history[6].timestamp, 1609459260);
    EXPECT_DOUBLE_EQ(history[6].price, 11.0);

    std::remove(filename.c_str());
}

TEST(SimulatorTest, LoadPriceHistoryFilter) {
    std::string filename = "test_filter.csv";
    CreateTempPriceFile(filename,
                        {"1609459200,2021-01-01 00:00,10,11,9,10.0", "1612137600,2021-02-01 00:00,11,12,10,11.0",
                         "1614556800,2021-03-01 00:00,12,13,11,12.0"});

    // Filter for January and February only
    // Use a large artificial_interval to prevent unexpected interpolation
    auto history = LoadPriceHistory(filename, "", "2021-01", "2021-02", 10000000);
    ASSERT_EQ(history.size(), 2);
    EXPECT_EQ(history[0].timestamp, 1609459200);
    EXPECT_EQ(history[1].timestamp, 1612137600);

    std::remove(filename.c_str());
}

TEST(SimulatorTest, LoadPriceHistoryAnomalousJump) {
    std::string filename = "test_jump.csv";
    // kMaxTickPriceChange is 2.0 (200%)
    CreateTempPriceFile(filename, {"1609473600,2021-01-01 00:00,10,10,10,10.0",
                                   "1609473660,2021-01-01 00:01,10,10,10,40.0",  // +300% jump
                                   "1609473720,2021-01-01 00:02,10,10,10,15.0"});

    auto history = LoadPriceHistory(filename, "", "", "", 300);
    ASSERT_EQ(history.size(), 2);  // Second row should be skipped
    EXPECT_DOUBLE_EQ(history[0].price, 10.0);
    EXPECT_DOUBLE_EQ(history[1].price, 15.0);

    std::remove(filename.c_str());
}

TEST(SimulatorTest, LoadPriceHistoryErrorCases) {
    // Non-existent file
    auto h1 = LoadPriceHistory("non_existent.csv", "", "", "", 300);
    EXPECT_TRUE(h1.empty());

    // Empty file
    std::string empty_file = "empty.csv";
    { std::ofstream f(empty_file); }
    auto h2 = LoadPriceHistory(empty_file, "", "", "", 300);
    EXPECT_TRUE(h2.empty());
    std::remove(empty_file.c_str());

    // Inconsistent/Malformed CSV
    std::string malformed_file = "malformed.csv";
    CreateTempPriceFile(malformed_file, {
                                            "1000,2021-01-01 00:00,10,10,10,0.0",   // Non-positive price (skipped)
                                            "1100,2021-01-01 00:01,10,10,10,10.0",  // Valid row (becomes first_row)
                                            "1050,2021-01-01 00:01,10,10,10,10.0",  // Out of order (returns {})
                                            "INVALID_ROW",                          // Exception catch (skipped)
                                            "1200,2021-01-01 00:02,10,10,10,10.5"   // Valid row
                                        });
    auto h3 = LoadPriceHistory(malformed_file, "", "", "", 300);
    // Should return empty vector because the 3rd row triggers the "Error: file not sorted" return {}
    EXPECT_TRUE(h3.empty());
    std::remove(malformed_file.c_str());
}

TEST(SimulatorTest, WriteTradeLogBasic) {
    std::vector<Trade> trades = {{0, 10.0, 60, 11.0, 0.1, 1.1, "profit_target"}};
    std::vector<PriceTick> history = {{0, 10.0}, {60, 11.0}};

    std::string filename = "test_trades.csv";
    WriteTradeLog(filename, trades, history);

    std::ifstream file(filename);
    std::string line;
    int line_count = 0;
    while (std::getline(file, line)) {
        // compare the line with expected content
        if (line_count == 0) {
            EXPECT_EQ(line,
                      "entry_timestamp,entry_date,entry_price,exit_timestamp,exit_date,exit_price,pnl_pct,exit_reason");
        } else if (line_count == 1) {
            EXPECT_EQ(line, "0,1970-01-01 00:00,10.0000,60,1970-01-01 00:01,11.0000,0.1000,profit_target");
        } else if (line_count == 2) {
            EXPECT_EQ(line, "0,1970-01-01 00:00,10.0000,60,1970-01-01 00:01,11.0000,0.1000,end_of_period");
        }
        line_count++;
    }

    EXPECT_EQ(line_count, 3);

    file.close();
    std::remove(filename.c_str());
}

TEST(SimulatorTest, WriteTradeLogErrorCases) {
    // Invalid path to trigger file error
    std::vector<Trade> trades;
    std::vector<PriceTick> history;
    WriteTradeLog("/invalid/path/to/file.csv", trades, history);
    // Should just return silently after printing to cerr
}

TEST(SimulatorTest, PriceSecondsAgoBasic) {
    std::vector<PriceTick> history = {{1000, 10.0}, {1010, 10.1}, {1020, 10.2}, {1030, 10.3}};

    // Exact match
    EXPECT_DOUBLE_EQ(PriceSecondsAgo(history, 3, 10, 300), 10.2);
    // Closest match before target
    EXPECT_DOUBLE_EQ(PriceSecondsAgo(history, 3, 15, 300), 10.1);
    // Staleness limit check
    EXPECT_DOUBLE_EQ(PriceSecondsAgo(history, 3, 31, 30), 0.0);
    // Out of range (before start)
    EXPECT_DOUBLE_EQ(PriceSecondsAgo(history, 3, 100, 300), 0.0);
}

TEST(SimulatorTest, MomentumEntryTriggeredDataGaps) {
    std::vector<PriceTick> history = {{1000, 10.0}, {1100, 11.0}};

    StrategyConfig cfg;
    cfg.window_seconds = 500;
    cfg.momentum_windows = {{1000, 0.01}};  // Lookback 1000s from ts 1100 is ts 100
    cfg.require_all = true;

    // require_all=true and data missing (ts 100 < start 1000) -> should return false
    EXPECT_FALSE(MomentumEntryTriggered(history, 1, cfg));

    // require_all=false and data missing -> should check next window or return require_all
    cfg.require_all = false;
    EXPECT_FALSE(MomentumEntryTriggered(history, 1, cfg));
}

TEST(SimulatorTest, MomentumEntryTriggeredProfit) {
    std::vector<PriceTick> history = {
        {1000, 10.0},  // Start (+0s)
        {2000, 10.0},
        {3000, 11.0}  // Current (+2000s)
    };

    StrategyConfig cfg;
    cfg.window_seconds = 2000;
    cfg.momentum_windows = {{1000, 0.05}};  // 10% change > 5% threshold
    cfg.require_all = false;

    EXPECT_TRUE(MomentumEntryTriggered(history, 2, cfg));

    cfg.momentum_windows[0].threshold = 0.15;  // 10% < 15%
    EXPECT_FALSE(MomentumEntryTriggered(history, 2, cfg));
}

TEST(SimulatorTest, MomentumEntryTriggeredRequireAll) {
    std::vector<PriceTick> history = {
        {1000, 10.0},
        {2000, 10.5},  // +5%
        {3000, 11.0}   // +10% from start, +4.7% from 2000
    };

    StrategyConfig cfg;
    cfg.window_seconds = 2000;
    cfg.momentum_windows = {
        {2000, 0.08},  // 10% change from 1000 -> 3000 (Pass)
        {1000, 0.03}   // 4.7% change from 2000 -> 3000 (Pass)
    };
    cfg.require_all = true;

    EXPECT_TRUE(MomentumEntryTriggered(history, 2, cfg));

    cfg.momentum_windows[1].threshold = 0.06;  // 4.7% < 6% (Fail)
    EXPECT_FALSE(MomentumEntryTriggered(history, 2, cfg));
}

TEST(SimulatorTest, ExitTriggeredProfit) {
    StrategyConfig cfg;
    cfg.strategy_type = StrategyType::kMomentumProfit;
    cfg.stop_loss_pct = 0.05;
    cfg.profit_target_pct = 0.10;

    std::string reason;

    // No trigger
    EXPECT_FALSE(ExitTriggered(cfg, 10.0, 10.0, 10.0, reason));

    // Profit target
    EXPECT_TRUE(ExitTriggered(cfg, 11.0, 10.0, 10.0, reason));
    EXPECT_EQ(reason, "profit_target");

    // Stop loss
    EXPECT_TRUE(ExitTriggered(cfg, 9.4, 10.0, 10.0, reason));
    EXPECT_EQ(reason, "stop_loss");
}

TEST(SimulatorTest, ExitTriggeredTrailing) {
    StrategyConfig cfg;
    cfg.strategy_type = StrategyType::kMomentumTrailing;
    cfg.stop_loss_pct = 0.05;
    cfg.activation_pct = 0.10;
    cfg.trailing_stop_pct = 0.02;

    std::string reason;

    // No trigger, not activated yet
    EXPECT_FALSE(ExitTriggered(cfg, 10.5, 10.0, 10.5, reason));

    // Stop loss (not activated)
    EXPECT_TRUE(ExitTriggered(cfg, 9.4, 10.0, 10.5, reason));
    EXPECT_EQ(reason, "stop_loss");

    // Activate (reached 11.0, +10%)
    EXPECT_FALSE(ExitTriggered(cfg, 11.0, 10.0, 11.0, reason));

    // Trailing stop trigger (11.0 * 0.98 = 10.78)
    EXPECT_TRUE(ExitTriggered(cfg, 10.7, 10.0, 11.0, reason));
    EXPECT_EQ(reason, "trailing_stop");
}

TEST(SimulatorTest, RunSimulationProfit) {
    std::vector<PriceTick> history = {
        {1000, 100.0},
        {1100, 105.0},  // No entry (window 200s)
        {1200, 110.0},  // Entry triggered (+10% > 5% threshold)
        {1300, 115.0},
        {1400, 122.0}  // Profit target trigger (110 * 1.1 = 121)
    };

    StrategyConfig cfg;
    cfg.strategy_type = StrategyType::kMomentumProfit;
    cfg.window_seconds = 200;
    cfg.momentum_windows = {{200, 0.05}};
    cfg.profit_target_pct = 0.10;
    cfg.stop_loss_pct = 0.05;

    auto trades = RunSimulation(history, cfg);
    ASSERT_EQ(trades.size(), 1);
    EXPECT_EQ(trades[0].entry_timestamp, 1200);
    EXPECT_EQ(trades[0].exit_timestamp, 1400);
    EXPECT_EQ(trades[0].exit_reason, "profit_target");
}

TEST(SimulatorTest, RunSimulationForceClose) {
    std::vector<PriceTick> history = {{1000, 100.0}, {1200, 110.0}, {1300, 112.0}};

    StrategyConfig cfg;
    cfg.strategy_type = StrategyType::kMomentumProfit;
    cfg.window_seconds = 200;
    cfg.momentum_windows = {{200, 0.05}};
    cfg.profit_target_pct = 0.50;  // Won't hit
    cfg.stop_loss_pct = 0.50;      // Won't hit

    auto trades = RunSimulation(history, cfg);
    ASSERT_EQ(trades.size(), 1);
    EXPECT_EQ(trades[0].exit_reason, "end_of_data");
    EXPECT_EQ(trades[0].exit_timestamp, 1300);
}

}  // namespace
