package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

const LockFilePath = "/tmp/go-bot.lock"

// Config holds the application's configuration.
type Config struct {
	Server     ServerConfig      `toml:"server"`
	GRPC       GRPCConfig        `toml:"grpc"`
	Database   DatabaseConfig    `toml:"database"`
	Log        LogConfig         `toml:"go_log"`
	Health     HealthCheckConfig `toml:"health_check"`
	Cron       CronConfig        `toml:"cron"`
	Risk       RiskConfig        `toml:"risk"`
	Exchanges  []ExchangeConfig  `toml:"exchange"`
	Simulation SimulationConfig  `toml:"simulation"`
}

// ServerConfig holds server-related settings.
type ServerConfig struct {
	OrchestratorInterval   time.Duration   `toml:"orchestrator_interval"`
	RefreshStratInterval   time.Duration   `toml:"refresh_strat_interval"`
	DefaultExchangeTimeout time.Duration   `toml:"default_exchange_timeout"`
	ShutdownTimeout        time.Duration   `toml:"shutdown_timeout"`
	CheckPendingPolicy     []time.Duration `toml:"check_pending_policy"`
}

// GRPCConfig holds the gRPC connection parameters.
type GRPCConfig struct {
	GoBotAddress         string        `toml:"go_bot_address"`
	PythonGatewayAddress string        `toml:"python_gateway_address"`
	ManagementAddress    string        `toml:"management_address"`
	ConnectionTimeout    time.Duration `toml:"connection_timeout"`
}

// DatabaseConfig holds the database connection parameters.
type DatabaseConfig struct {
	Host                   string        `toml:"host"`
	Port                   int           `toml:"port"`
	User                   string        `toml:"user"`
	Password               string        `toml:"password"`
	DBName                 string        `toml:"dbname"`
	SSLMode                string        `toml:"sslmode"`
	MaxConns               int           `toml:"max_conns"`
	ConnectTimeout         time.Duration `toml:"connect_timeout"`
	StatementTimeout       time.Duration `toml:"statement_timeout"`
	LockTimeout            time.Duration `toml:"lock_timeout"`
	IdleInTxSessionTimeout time.Duration `toml:"idle_in_tx_session_timeout"`
}

// LogConfig holds the logging configuration.
type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
	Path   string `toml:"path"`
	Rotate bool   `toml:"rotate"`
	Source bool   `toml:"source"`
}

// HealthCheckConfig holds settings for the background health monitor.
type HealthCheckConfig struct {
	Asset         string        `toml:"asset"`
	Interval      time.Duration `toml:"interval"`
	RetryAttempts int           `toml:"retry_attempts"`
	RetryDelay    time.Duration `toml:"retry_delay"`
}

// CronConfig holds the configuration for scheduled cron jobs.
type CronConfig struct {
	MarketDataCleanup MarketDataCleanupConfig `toml:"market_data_cleanup"`
}

// MarketDataCleanupConfig holds settings for the market data cleanup cron job.
type MarketDataCleanupConfig struct {
	Enabled       bool   `toml:"enabled"`
	Schedule      string `toml:"schedule"`
	RetentionDays int    `toml:"retention_days"`
	RunOnStartup  bool   `toml:"run_on_startup"`
}

// RiskConfig holds the risk management parameters.
type RiskConfig struct {
	// MaxOpenPositions defines the maximum number of simultaneous positions allowed.
	MaxOpenPositions int `toml:"max_open_positions"`
	// MaxBudgetPerTrade limits the maximum budget allocated per trade for specific assets.
	MaxBudgetPerTrade map[string]float64 `toml:"max_budget_per_trade"`
}

// ExchangeConfig holds the exchange connection parameters.
type ExchangeConfig struct {
	Name        string        `toml:"name"`
	APIKey      string        `toml:"api_key"`
	Secret      string        `toml:"secret"`
	SandboxMode bool          `toml:"sandbox_mode"`
	HealthCheck bool          `toml:"health_check"`
	Timeout     time.Duration `toml:"timeout"`
}

// SimulationConfig holds backtesting and simulation parameters.
type SimulationConfig struct {
	Enabled     bool    `toml:"enabled"`
	Symbol      string  `toml:"symbol"`
	Begin       string  `toml:"begin"`
	End         string  `toml:"end"`
	Input       string  `toml:"input"`
	Output      string  `toml:"output"`
	InitialUSDT float64 `toml:"initial_usdt"`
}

// newWithDefaults creates a Config struct with sensible default values.
func newWithDefaults() *Config {
	return &Config{
		Server: ServerConfig{
			OrchestratorInterval:   15 * time.Second,
			RefreshStratInterval:   1 * time.Minute,
			DefaultExchangeTimeout: 10 * time.Second,
			ShutdownTimeout:        10 * time.Second,
			CheckPendingPolicy:     []time.Duration{0, 5 * time.Second, 15 * time.Second, 30 * time.Second},
		},
		GRPC: GRPCConfig{
			ConnectionTimeout: 5 * time.Second,
		},
		Database: DatabaseConfig{
			SSLMode: "disable",
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
			Source: false, // Disabled by default for performance.
		},
		Cron: CronConfig{
			MarketDataCleanup: MarketDataCleanupConfig{
				Enabled:       false,
				Schedule:      "0 0 0 * * *",
				RetentionDays: 7,
				RunOnStartup:  false,
			},
		},
		Simulation: SimulationConfig{
			Enabled: false,
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
