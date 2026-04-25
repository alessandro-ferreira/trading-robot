#pragma once

#include <grpcpp/grpcpp.h>

#include <memory>

#include "IEngine.hpp"

using std::shared_ptr;
using std::unique_ptr;

namespace trading::ml {

// ManagementClient handles gRPC communication with the Go Bot's Management Service.
class ManagementClient {
   public:
    explicit ManagementClient(shared_ptr<grpc::Channel> channel);
    ~ManagementClient();

    // UpdateStrategy sends a strategy update request to the Go Bot.
    // Returns true if the RPC was successful and the bot accepted the update.
    bool UpdateStrategy(const StrategyUpdate& update);

    // UpdateRisk sends a risk update request to the Go Bot.
    // Returns true if the RPC was successful.
    bool UpdateRisk(const RiskUpdate& update);

   private:
    // PIMPL pattern to hide gRPC/Proto generated types from the public header.
    class Impl;
    unique_ptr<Impl> impl_;
};

}  // namespace trading::ml
