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
	"trading/robot/go-bot/internal/components/health"
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

	bgManager.Start(ctx)

	// --- Graceful Shutdown ---
	// Block until a shutdown signal is received.
	<-ctx.Done()

	slog.Info("Shutdown signal received. Starting graceful shutdown...")

	// Perform cleanup operations.
	bgManager.Wait()

	slog.Info("Closing client connections and database pool...")
	gatewayClient.Close()
	db.Close()

	if cfg.Server.ShutdownTimeout > 0 {
		slog.Info("Waiting for shutdown delay", "duration", cfg.Server.ShutdownTimeout)
		time.Sleep(cfg.Server.ShutdownTimeout)
	}

	slog.Info("Server shutdown complete.")
}

func setupHealthMonitor(cfg *config.Config, execService *execution.Service, bgManager *background.Manager) {
	// Identify which exchanges have health checks enabled and create a monitor for them.
	var exchangeNames []string
	for _, ex := range cfg.Exchanges {
		if ex.HealthCheck {
			exchangeNames = append(exchangeNames, ex.Name)
		}
	}
	if len(exchangeNames) == 0 {
		slog.Warn("No exchanges configured for health monitoring. Health monitor will not be started.")
		return
	}
	slog.Info("Setting up health monitor for configured exchanges", "exchanges", exchangeNames)

	healthMonitor := health.NewMonitor(slog.Default(), exchangeNames)

	// Define the check logic using a closure to wrap the service call.
	// This allows us to use arbitrary methods (like GetBalance) and custom parameters (like Asset).
	// We also wrap the job with retry logic to handle transient failures robustly.
	checkMethod := func(ctx context.Context, exchange string) error {
		job := func(ctx context.Context) error {
			_, err := execService.GetBalance(ctx, exchange, cfg.Health.Asset)
			return err
		}
		return background.WithRetry(job, cfg.Health.RetryAttempts, cfg.Health.RetryDelay)(ctx)
	}

	// Register the periodic health check task
	healthTask := background.NewPeriodicTask("health-check", cfg.Health.Interval, true, func(ctx context.Context) error {
		return healthMonitor.CheckHealth(ctx, checkMethod)
	})
	bgManager.Add(healthTask)
}
