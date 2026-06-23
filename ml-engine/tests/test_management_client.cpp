#include <grpcpp/grpcpp.h>
#include <gtest/gtest.h>

#include "ManagementClient.hpp"
#include "management.grpc.pb.h"

namespace trading::ml {

using trading::robot::management::v1::ManagementService;
using trading::robot::management::v1::UpdateRiskRequest;
using trading::robot::management::v1::UpdateRiskResponse;
using trading::robot::management::v1::UpdateStrategyRequest;
using trading::robot::management::v1::UpdateStrategyResponse;

// FakeManagementService implements the server-side gRPC logic for testing.
class FakeManagementService final : public ManagementService::Service {
   public:
    grpc::Status UpdateStrategy([[maybe_unused]] grpc::ServerContext* context, const UpdateStrategyRequest* request,
                                UpdateStrategyResponse* response) override {
        if (request->symbol() == "FAIL") {
            response->set_success(false);
        } else if (request->symbol() == "GRPC_ERROR") {
            return grpc::Status(grpc::StatusCode::INTERNAL, "Internal Server Error");
        } else {
            response->set_success(true);
        }
        return grpc::Status::OK;
    }

    grpc::Status UpdateRisk([[maybe_unused]] grpc::ServerContext* context,
                            [[maybe_unused]] const UpdateRiskRequest* request, UpdateRiskResponse* response) override {
        if (request->symbol() == "GRPC_ERROR") {
            return grpc::Status(grpc::StatusCode::INTERNAL, "Internal Server Error");
        }
        response->set_success(true);
        return grpc::Status::OK;
    }
};

class ManagementClientTest : public ::testing::Test {
   protected:
    void SetUp() override {
        // Start a local gRPC server on an available port.
        grpc::ServerBuilder builder;
        builder.RegisterService(&service_);
        server_ = builder.BuildAndStart();

        // Create a channel and client pointing to the local server.
        auto channel = server_->InProcessChannel(grpc::ChannelArguments());
        client_ = std::make_unique<ManagementClient>(channel);
    }

    void TearDown() override { server_->Shutdown(); }

    FakeManagementService service_;
    std::unique_ptr<grpc::Server> server_;
    std::unique_ptr<ManagementClient> client_;
};

TEST_F(ManagementClientTest, UpdateStrategySuccess) {
    StrategyUpdate update;
    update.exchange = "binance";
    update.symbol = "BTC/USDT";
    update.strategy_type = "momentum_trailing";
    update.enabled = true;
    update.momentum_params.label = "default";
    update.momentum_params.window_seconds = 10;
    update.momentum_params.stop_loss_pct = 0.1;
    update.momentum_params.profit_target_pct = 0.05;
    update.momentum_params.activation_pct = 0.05;
    update.momentum_params.trailing_stop_pct = 0.02;
    update.momentum_params.windows.push_back({10, 0.01});

    EXPECT_TRUE(client_->UpdateStrategy(update));
}

TEST_F(ManagementClientTest, UpdateStrategyGRPCError) {
    StrategyUpdate update;
    update.symbol = "GRPC_ERROR";
    EXPECT_FALSE(client_->UpdateStrategy(update));
}

TEST_F(ManagementClientTest, UpdateStrategyReturnsErrorOnFailure) {
    StrategyUpdate update;
    update.symbol = "FAIL";
    EXPECT_FALSE(client_->UpdateStrategy(update));
}

TEST_F(ManagementClientTest, UpdateRiskSuccess) {
    RiskUpdate update;
    update.exchange = "binance";
    update.symbol = "BTC/USDT";
    update.allocated_budget = 100.0;
    update.max_asset_units = 1.0;

    EXPECT_TRUE(client_->UpdateRisk(update));
}

TEST_F(ManagementClientTest, UpdateRiskGRPCError) {
    RiskUpdate update;
    update.symbol = "GRPC_ERROR";
    EXPECT_FALSE(client_->UpdateRisk(update));
}

}  // namespace trading::ml

int main(int argc, char** argv) {
    ::testing::InitGoogleTest(&argc, argv);
    return RUN_ALL_TESTS();
}
