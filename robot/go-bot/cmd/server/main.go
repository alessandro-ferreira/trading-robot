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

	"trading/robot/go-bot/internal/background"
	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/logger"
)

func main() {

	// Create a context that is canceled on a SIGINT or SIGTERM signal.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Configuration & Logger ---
	// Define a command-line flag for the config file path.
	configPath := flag.String("config", "", "path to the configuration file")
	flag.Parse()

	// Check if the config file was provided.
	if configPath == nil || *configPath == "" {
		log.Fatal("❌ Configuration file path must be provided using the -config flag, e.g., -config=config.toml")
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		// Fallback to basic logging if config load fails.
		log.Fatalf("❌ Failed to load configuration: %v", err)
	}

	logger.Setup(cfg.Log)

	slog.Info("Starting Go Trading Bot...")

	slog.Info("Configuration loaded successfully",
		slog.String("db_host", cfg.Database.Host),
		slog.String("python_gateway", cfg.GRPC.PythonGatewayAddress),
	)

	// --- Infrastructure Initialization ---
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
	slog.Info("Connected to python-gateway successfully")

	// --- Service Initialization ---
	repoContainer := repository.New()

	execService := execution.NewService(slog.Default(), db, gatewayClient, repoContainer)
	slog.Info("Execution service initialized")

	// --- Background Tasks ---
	bgManager := background.NewManager(slog.Default())

	setupHealthMonitor(cfg, execService, bgManager)

	closers := setupSignalGenerators(cfg, gatewayClient, bgManager)
	for _, c := range closers {
		defer c.Close()
	}

	bgManager.Start(ctx)

	// --- Graceful Shutdown ---
	// Block until a shutdown signal is received.
	<-ctx.Done()

	slog.Info("Shutdown signal received. Starting graceful shutdown...")

	// Perform cleanup operations.
	bgManager.Wait()

	slog.Info("All background tasks stopped. Closing connections...")

	if cfg.Server.ShutdownTimeout > 0 {
		slog.Info("Waiting for shutdown delay", "duration", cfg.Server.ShutdownTimeout)
		time.Sleep(cfg.Server.ShutdownTimeout)
	}

	slog.Info("Server shutdown complete.")
}
