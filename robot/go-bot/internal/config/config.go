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
	Server    ServerConfig      `toml:"server"`
	Log       LogConfig         `toml:"go_log"`
	Database  DatabaseConfig    `toml:"database"`
	GRPC      GRPCConfig        `toml:"grpc"`
	Health    HealthCheckConfig `toml:"health_check"`
	Exchanges []ExchangeConfig  `toml:"exchange"`
	Risk      RiskConfig        `toml:"risk"`
}

// ServerConfig holds server-related settings.
type ServerConfig struct {
	OrchestratorInterval   time.Duration `toml:"orchestrator_interval"`
	RefreshStratInterval   time.Duration `toml:"refresh_strat_interval"`
	DefaultExchangeTimeout time.Duration `toml:"default_exchange_timeout"`
	ShutdownTimeout        time.Duration `toml:"shutdown_timeout"`
}

// LogConfig holds the logging configuration.
type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
	Path   string `toml:"path"`
	Rotate bool   `toml:"rotate"`
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
	GoBotAddress         string        `toml:"go_bot_address"`
	PythonGatewayAddress string        `toml:"python_gateway_address"`
	ManagementAddress    string        `toml:"management_address"`
	ConnectionTimeout    time.Duration `toml:"connection_timeout"`
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
	Name        string        `toml:"name"`
	APIKey      string        `toml:"api_key"`
	Secret      string        `toml:"secret"`
	SandboxMode bool          `toml:"sandbox_mode"`
	HealthCheck bool          `toml:"health_check"`
	Timeout     time.Duration `toml:"timeout"`
}

// RiskConfig holds the risk management parameters.
type RiskConfig struct {
	// MaxOpenPositions defines the maximum number of simultaneous positions allowed.
	MaxOpenPositions int `toml:"max_open_positions"`
	// MaxBudgetPerTrade limits the maximum budget allocated per trade for specific assets.
	MaxBudgetPerTrade map[string]float64 `toml:"max_budget_per_trade"`
}

// newWithDefaults creates a Config struct with sensible default values.
func newWithDefaults() *Config {
	return &Config{
		Server: ServerConfig{
			OrchestratorInterval:   10 * time.Second,
			DefaultExchangeTimeout: 10 * time.Second,
			ShutdownTimeout:        10 * time.Second,
		},
		Log: LogConfig{
			Level:  "info",
			Format: "text",
			Source: false, // Disabled by default for performance.
		},
		Database: DatabaseConfig{
			SSLMode: "disable",
		},
		GRPC: GRPCConfig{
			ConnectionTimeout: 5 * time.Second,
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
