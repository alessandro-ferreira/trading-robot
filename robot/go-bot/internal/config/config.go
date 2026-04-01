package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	StrategyDummy            = "dummy"
	StrategyMomentumProfit   = "momentum_profit"
	StrategyMomentumTrailing = "momentum_trailing"
)

// Config holds the application's configuration.
type Config struct {
	Server    ServerConfig      `toml:"server"`
	Log       LogConfig         `toml:"log"`
	Database  DatabaseConfig    `toml:"database"`
	GRPC      GRPCConfig        `toml:"grpc"`
	Health    HealthCheckConfig `toml:"health_check"`
	Exchanges []ExchangeConfig  `toml:"exchange"`
	Risk      RiskConfig        `toml:"risk"`
	Pairs     []PairConfig      `toml:"pairs"`
}

// ServerConfig holds server-related settings.
type ServerConfig struct {
	OrchestratorInterval time.Duration `toml:"orchestrator_interval"`
	ShutdownTimeout      time.Duration `toml:"shutdown_timeout"`
}

// LogConfig holds the logging configuration.
type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
	Source bool   `toml:"source"`
}

// DatabaseConfig holds the database connection parameters.
type DatabaseConfig struct {
	Host     string `toml:"host"`
	Port     int    `toml:"port"`
	User     string `toml:"user"`
	Password string `toml:"password"`
	DBName   string `toml:"dbname"`
	SSLMode  string `toml:"sslmode"`
}

// GRPCConfig holds the gRPC connection parameters.
type GRPCConfig struct {
	GoBotAddress         string `toml:"go_bot_address"`
	PythonGatewayAddress string `toml:"python_gateway_address"`
}

// HealthCheckConfig holds settings for the background health monitor.
type HealthCheckConfig struct {
	Asset         string        `toml:"asset"`
	Interval      time.Duration `toml:"interval"`
	RetryAttempts int           `toml:"retry_attempts"`
	RetryDelay    time.Duration `toml:"retry_delay"`
}

// ExchangeConfig holds the exchange connection parameters.
type ExchangeConfig struct {
	Name        string `toml:"name"`
	APIKey      string `toml:"api_key"`
	Secret      string `toml:"secret"`
	SandboxMode bool   `toml:"sandbox_mode"`
	HealthCheck bool   `toml:"health_check"`
}

// RiskConfig holds the risk management parameters.
type RiskConfig struct {
	// MaxOpenPositions defines the maximum number of simultaneous positions allowed.
	MaxOpenPositions int `toml:"max_open_positions"`
	// MaxDailyLoss defines the maximum allowed loss in quote currency for the day.
	MaxDailyLoss float64 `toml:"max_daily_loss"`
}

// PairConfig defines a specific trading pair and its associated strategy.
type PairConfig struct {
	Symbol   string         `toml:"symbol"`
	Exchange string         `toml:"exchange"`
	Risk     PairRiskConfig `toml:"risk"`
	Strategy StrategyConfig `toml:"strategy"`
}

// PairRiskConfig holds risk parameters specific to a trading pair.
type PairRiskConfig struct {
	RiskPerTrade float64 `toml:"risk_per_trade"`
}

// StrategyConfig holds the trading strategy parameters.
type StrategyConfig struct {
	Type     string         `toml:"type"`
	Momentum MomentumConfig `toml:"momentum"`
}

type MomentumConfig struct {
	WindowSeconds   int64   `toml:"window_seconds"`
	LookbackSeconds int64   `toml:"lookback_seconds"`
	Threshold       float64 `toml:"threshold"`
	RequireAll      bool    `toml:"require_all"`
	StopLossPct     float64 `toml:"stop_loss_pct"`
	ProfitTargetPct float64 `toml:"profit_target_pct"`
	ActivationPct   float64 `toml:"activation_pct"`
	TrailingStopPct float64 `toml:"trailing_stop_pct"`
}

// newWithDefaults creates a Config struct with sensible default values.
func newWithDefaults() *Config {
	return &Config{
		Server: ServerConfig{
			OrchestratorInterval: 10 * time.Second,
			ShutdownTimeout:      10 * time.Second,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
			Source: false, // Disabled by default for performance.
		},
		Database: DatabaseConfig{
			SSLMode: "disable",
		},
	}
}

// Load decodes the given file into a Config struct.
func Load(path string) (*Config, error) {
	cfg := newWithDefaults()
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		// Check if the file doesn't exist to provide a more helpful error.
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s", path)
		}
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}
	return cfg, nil
}
