#include "ManagementClient.hpp"
#include "management.grpc.pb.h"

using std::shared_ptr;
using std::unique_ptr;

namespace trading::ml {

using trading::robot::management::v1::ManagementService;
using trading::robot::management::v1::UpdateRiskRequest;
using trading::robot::management::v1::UpdateRiskResponse;
using trading::robot::management::v1::UpdateStrategyRequest;
using trading::robot::management::v1::UpdateStrategyResponse;

class ManagementClient::Impl {
   public:
    explicit Impl(shared_ptr<grpc::Channel> channel) : stub_(ManagementService::NewStub(channel)) {}

    bool UpdateStrategy(const StrategyUpdate& update) {
        UpdateStrategyRequest request;
        request.set_exchange(update.exchange);
        request.set_symbol(update.symbol);
        request.set_strategy_type(update.strategy_type);
        request.set_enabled(update.enabled);

        auto* m_params = request.mutable_momentum_params();
        m_params->set_label(update.momentum_params.label);
        m_params->set_window_seconds(update.momentum_params.window_seconds);
        m_params->set_require_all(update.momentum_params.require_all);
        m_params->set_stop_loss_pct(update.momentum_params.stop_loss_pct);

        // Map optional fields using explicit presence
        if (update.momentum_params.profit_target_pct)
            m_params->set_profit_target_pct(*update.momentum_params.profit_target_pct);
        if (update.momentum_params.activation_pct) m_params->set_activation_pct(*update.momentum_params.activation_pct);
        if (update.momentum_params.trailing_stop_pct)
            m_params->set_trailing_stop_pct(*update.momentum_params.trailing_stop_pct);

        for (const auto& w : update.momentum_params.windows) {
            auto* win = m_params->add_windows();
            win->set_lookback_seconds(w.lookback_seconds);
            win->set_threshold(w.threshold);
        }

        UpdateStrategyResponse response;
        grpc::ClientContext context;

        grpc::Status status = stub_->UpdateStrategy(&context, request, &response);
        if (!status.ok()) {
            return false;
        }
        return response.success();
    }

    bool UpdateRisk(const RiskUpdate& update) {
        UpdateRiskRequest request;
        request.set_exchange(update.exchange);
        request.set_symbol(update.symbol);
        request.set_risk_per_trade(update.risk_per_trade);
        request.set_max_position_size(update.max_position_size);

        UpdateRiskResponse response;
        grpc::ClientContext context;

        grpc::Status status = stub_->UpdateRisk(&context, request, &response);
        if (!status.ok()) {
            return false;
        }
        return response.success();
    }

   private:
    unique_ptr<ManagementService::Stub> stub_;
};

ManagementClient::ManagementClient(shared_ptr<grpc::Channel> channel) : impl_(std::make_unique<Impl>(channel)) {}

ManagementClient::~ManagementClient() = default;

bool ManagementClient::UpdateStrategy(const StrategyUpdate& update) { return impl_->UpdateStrategy(update); }

bool ManagementClient::UpdateRisk(const RiskUpdate& update) { return impl_->UpdateRisk(update); }

}  // namespace trading::ml
