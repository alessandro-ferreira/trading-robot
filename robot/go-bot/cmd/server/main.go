package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/logger"
)

func main() {
	// Define a command-line flag for the config file path.
	configPath := flag.String("config", "config.toml", "path to the configuration file")
	flag.Parse()

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
	dbPool, err := database.NewDBPool(context.Background(), cfg.Database)
	if err != nil {
		slog.Error("Failed to initialize database connection", "error", err)
		os.Exit(1)
	}
	defer dbPool.Close()

	slog.Info("Database connection pool established")

	// TODO: Initialize gRPC client and start trading strategies.

	// --- Graceful Shutdown ---
	// Create a channel to listen for OS signals.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Block until a signal is received.
	sig := <-stop

	// Log the received signal.
	slog.Info("Shutdown signal received", "signal", sig.String())

	// Create a context with a timeout for graceful shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	slog.Info("Closing database connection pool...")
	dbPool.Close() // pgxpool.Close() is safe to call multiple times.

	// Wait for the shutdown context to be done (e.g., timeout).
	<-shutdownCtx.Done()
	if err := shutdownCtx.Err(); err == context.DeadlineExceeded {
		slog.Error("Graceful shutdown timed out. Forcing exit.")
		os.Exit(1)
	}

	slog.Info("Server shutdown complete.")
}
