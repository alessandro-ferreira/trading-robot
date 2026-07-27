#include <getopt.h>

#include <iostream>
#include <string>
#include <vector>
#include <fstream>

#include "simulator.hpp"

using std::cerr;
using std::cout;
using std::endl;
using std::string;
using std::vector;

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
         << "  -f, --input <file>         Input CSV price file (default: " << kDefaultPriceDir << "<symbol>_prices.csv)\n"
         << "  -o, --output <file>        Output CSV trade log (default: stdout)\n"
         << "  -i, --interval <sec>       Artificial price generation interval in seconds\n"
         << "  -h, --help                 Display this help message\n\n"
         << "Example: \n"
         << prog << " -s BTC -b 2021-01 -e 2021-12 -t profit -w 21600 -m 10800:0.01,18000:0.02 -l 0.05 -p 0.10 -i 30\n"
         << endl;
}

int main(int argc, char** argv) {
    string symbol, begin, end, type_str, momentum_spec, input_file, output_file;
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
        {"input", required_argument, 0, 'f'},
        {"output", required_argument, 0, 'o'},
        {"interval", required_argument, 0, 'i'},
        {"help", no_argument, 0, 'h'},
        {0, 0, 0, 0}
    };

    int opt;
    while ((opt = getopt_long(argc, argv, "s:b:e:t:w:m:al:p:r:f:o:i:h", long_options, nullptr)) != -1) {
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
            case 'f': input_file = optarg; break;
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

    vector<PriceTick> history = LoadPriceHistory(input_file, symbol, begin, end, artificial_interval);
    if (history.empty()) {
        cerr << "Error: no data." << endl;
        return 1;
    }

    vector<Trade> trades = RunSimulation(history, cfg);

    // Write the log file or to stdout
    WriteTradeLog(output_file, trades, history);

    return 0;
}
