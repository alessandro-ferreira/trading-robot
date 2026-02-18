package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/logger"
)

func main() {
	// Create a context that is canceled on a SIGINT or SIGTERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop() // stop() removes the signal handler.

	// Define a command-line flag for the config file path.
	configPath := flag.String("config", "", "path to the configuration file")
	flag.Parse()

	// Check if the config file was provided.
	if configPath == nil || *configPath == "" {
		log.Fatal("❌ Configuration file path must be provided using the -config flag, e.g., -config=config.toml")
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		// If config fails, we can't set up the logger from it, so use a basic log.
		log.Fatalf("❌ Failed to load configuration: %v", err)
	}

	// Set up the structured logger *after* loading the config.
	logger.Setup(cfg.Log)

	slog.Info("Starting Go Trading Bot...")

	slog.Info("Configuration loaded successfully",
		slog.String("db_host", cfg.Database.Host),
		slog.String("python_gateway", cfg.GRPC.PythonGatewayAddress),
	)

	// Initialize database connection
	db, err := database.NewDBPool(ctx, cfg.Database)
	if err != nil {
		slog.Error("Failed to initialize database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("Database connection pool established")

	// Initialize gRPC client for the Python gateway
	gatewayClient, err := execution.NewGatewayClient(&cfg.GRPC)
	if err != nil {
		slog.Error("Failed to connect to python-gateway", "error", err)
		os.Exit(1)
	}
	defer gatewayClient.Close()

	// Initialize repositories and services
	repoContainer := repository.New()

	execService := execution.NewService(slog.Default(), db, gatewayClient, repoContainer)
	slog.Info("Execution service initialized")

	// Example usage: Get balance for a fixed asset. This is expected to fail
	// with a 'not implemented' error until the service and repository are fully implemented.
	err = execService.GetBalance(ctx, "binance", "BTC") // "BTC" is the symbol for Bitcoin
	if err != nil {
		slog.Warn("Could not get initial balance (expected during development)", "exchange", "binance", "asset", "BTC", "error", err)
	} else {
		slog.Info("Successfully retrieved initial balance")
	}

	// --- Graceful Shutdown ---
	// Block until the context is canceled (i.e., a shutdown signal is received).
	<-ctx.Done()

	slog.Info("Shutdown signal received. Starting graceful shutdown...")

	// Perform cleanup operations.
	slog.Info("Closing client connections and database pool...")
	gatewayClient.Close()
	db.Close()

	// Wait for a configurable period to allow for any final logs to be processed.
	if cfg.Server.ShutdownTimeout > 0 {
		slog.Info("Waiting for shutdown delay", "duration", cfg.Server.ShutdownTimeout)
		time.Sleep(cfg.Server.ShutdownTimeout)
	}

	slog.Info("Server shutdown complete.")
}
