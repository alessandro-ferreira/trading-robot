#include <getopt.h>

#include <chrono>
#include <iostream>
#include <memory>
#include <string>
#include <thread>
#include <vector>

#include "ManagementClient.hpp"
#include "TemplateEngine.hpp"

using std::cerr;
using std::cout;
using std::endl;

using std::exception;
using std::string;
using std::unique_ptr;
using std::vector;

/**
 * print_usage displays the command-line interface options.
 */
void print_usage() {
    cout << "Usage: engine [options]\n"
         << "Options:\n"
         << "  -t, --target <addr>           Go Bot Management API address (default: localhost:50052)\n"
         << "  -e, --engine <name>           Intelligence engine to use (default: template)\n"
         << "  -r, --max-retries <num>       Number of retries for failed updates (default: 3)\n"
         << "  -s, --retries-delay <seconds> Delay between retries in seconds (default: 5)\n"
         << "  -h, --help                    Display this help message\n"
         << endl;
}

int main(int argc, char** argv) {
    // Default values for the options
    string target_address = "localhost:50052";
    string engine_name = "template";
    int max_retries = 3;
    int retry_delay = 5;

    // CLI argument definitions.
    static struct option long_options[] = {{"target", required_argument, 0, 't'},
                                           {"engine", required_argument, 0, 'e'},
                                           {"max_retries", required_argument, 0, 'r'},
                                           {"retries-delay", required_argument, 0, 's'},
                                           {"help", no_argument, 0, 'h'},
                                           {0, 0, 0, 0}};

    int opt;
    try {
        // Parse command-line options.
        while ((opt = getopt_long(argc, argv, "t:e:r:s:h", long_options, nullptr)) != -1) {
            switch (opt) {
                case 't':
                    target_address = optarg;
                    break;
                case 'e':
                    engine_name = optarg;
                    break;
                case 'r':
                    max_retries = std::stoi(optarg);
                    break;
                case 's':
                    retry_delay = std::stoi(optarg);
                    break;
                case 'h':
                    print_usage();
                    return 0;
                default:
                    print_usage();
                    return 1;
            }
        }
    } catch (const exception& e) {
        cerr << "Error: Option '-" << static_cast<char>(opt) << "' received invalid value '" << optarg << "' ("
             << e.what() << ")" << endl;
        print_usage();
        return 1;
    }
    cout << "Machine Learning Engine starting..." << endl;

    // Initialize gRPC channel and Management Client.
    auto channel = grpc::CreateChannel(target_address, grpc::InsecureChannelCredentials());
    auto client = std::make_unique<trading::ml::ManagementClient>(channel);

    // Intelligence Engine selection and initialization.
    unique_ptr<trading::ml::IEngine> engine;
    if (engine_name == "template") {
        engine = std::make_unique<trading::ml::TemplateEngine>();
    } else {
        cerr << "Unknown engine: " << engine_name << endl;
        return 1;
    }

    if (!engine->Init()) {
        cerr << "Failed to initialize engine: " << engine->GetName() << endl;
        return 1;
    }

    cout << "Engine initialized: " << engine->GetName() << endl;
    cout << "Connecting to Management Service at " << target_address << "..." << endl;
    cout << "Retry policy: max_retries=" << max_retries << ", retry_delay=" << retry_delay << "s" << endl;

    // Update Phase: Single-pass update for the configured instruments.
    vector<string> symbols = {"BTC/USDT", "ETH/USDT", "LTC/USDT"};

    for (const auto& symbol : symbols) {
        try {
            string exchange = "binance";
            auto strategy_update = engine->GenerateStrategyUpdate(exchange, symbol);
            auto risk_update = engine->GenerateRiskUpdate(exchange, symbol);

            // Execute updates with a retry policy for reliability.
            for (int i = 0; i < max_retries; ++i) {
                bool strategy_ok = client->UpdateStrategy(strategy_update);
                bool risk_ok = client->UpdateRisk(risk_update);

                if (strategy_ok && risk_ok) {
                    cout << "[" << engine->GetName() << "] Successfully updated strategy and risk for " << symbol
                         << endl;
                    break;
                }

                if (!strategy_ok) {
                    cerr << "Update failed: UpdateStrategy returned error for " << symbol << " (attempt " << i + 1
                         << "/" << max_retries << ")" << endl;
                }
                if (!risk_ok) {
                    cerr << "Update failed: UpdateRisk returned error for " << symbol << " (attempt " << i + 1 << "/"
                         << max_retries << ")" << endl;
                }

                if (i < max_retries - 1) {
                    std::this_thread::sleep_for(std::chrono::seconds(retry_delay));
                } else {
                    cerr << "Critical: Failed to sync parameters for " << symbol << " after max retries." << endl;
                }
            }
        } catch (const exception& e) {
            cerr << "Unexpected error processing " << symbol << ": " << e.what() << endl;
        }
    }

    // Comment this block to keep the strategies enabled
    for (const auto& symbol : symbols) {
        try {
            string exchange = "binance";
            auto strategy_update = engine->GenerateStrategyUpdate(exchange, symbol);
            strategy_update.enabled = false;
            strategy_update.momentum_params = {};

            bool strategy_ok = client->UpdateStrategy(strategy_update);
            if (strategy_ok) {
                cout << "[" << engine->GetName() << "] Successfully disabled strategy for " << symbol << endl;
            } else {
                cerr << "Failed to disable strategy for " << symbol << endl;
            }
        } catch (const exception& e) {
            cerr << "Unexpected error disabling strategy for " << symbol << ": " << e.what() << endl;
        }
    }

    return 0;
}
