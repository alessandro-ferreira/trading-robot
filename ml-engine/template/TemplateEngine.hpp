#pragma once
#include "IEngine.hpp"

using std::string;

namespace trading::ml {

class TemplateEngine : public IEngine {
   public:
    TemplateEngine() = default;
    bool Init() override { return true; }

    // GenerateStrategyUpdate evaluates market conditions for a specific pair and returns
    // the desired strategy configuration.
    StrategyUpdate GenerateStrategyUpdate(const string& exchange, const string& symbol) override;

    // GenerateRiskUpdate evaluates market conditions and returns the desired
    // risk configuration for a specific pair.
    RiskUpdate GenerateRiskUpdate(const string& exchange, const string& symbol) override;

    string GetName() const override { return "template_v1"; }
};

}  // namespace trading::ml
