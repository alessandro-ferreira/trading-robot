package config

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
)

// Config holds the application's configuration.
type Config struct {
	Server   ServerConfig   `toml:"server"`
	Log      LogConfig      `toml:"log"`
	Database DatabaseConfig `toml:"database"`
	GRPC     GRPCConfig     `toml:"grpc"`
	Exchange ExchangeConfig `toml:"exchange"`
}

// ServerConfig holds server-related settings.
type ServerConfig struct {
	ShutdownTimeout time.Duration `toml:"shutdown_timeout"`
}

// LogConfig holds the logging configuration.
type LogConfig struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
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

// ExchangeConfig holds the exchange connection parameters.
type ExchangeConfig struct {
	Name        string `toml:"name"`
	APIKey      string `toml:"api_key"`
	Secret      string `toml:"secret"`
	SandboxMode bool   `toml:"sandbox_mode"`
}

// Load decodes the given file into a Config struct.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		// Check if the file doesn't exist to provide a more helpful error.
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found at %s. Please copy config.toml.example to config.toml and fill it out", path)
		}
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}
	return &cfg, nil
}
