package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "trading/robot/go-bot/gen/go/v1"
	"trading/robot/go-bot/internal/api"
	"trading/robot/go-bot/internal/background"
	"trading/robot/go-bot/internal/components/execution"
	"trading/robot/go-bot/internal/components/portfolio"
	reconcil "trading/robot/go-bot/internal/components/reconciliation"
	"trading/robot/go-bot/internal/config"
	"trading/robot/go-bot/internal/database"
	"trading/robot/go-bot/internal/database/repository"
	"trading/robot/go-bot/internal/logger"
	"trading/robot/go-bot/internal/orchestrator"

	"google.golang.org/grpc"
)

func main() {

	// Create a context that is canceled when an interrupt signal is received.
	ctx, stop := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT,
	)
	defer stop()

	// --- Security Check ---
	if os.Geteuid() == 0 {
		log.Fatal("❌ Running as root is not allowed for security reasons")
	}

	// Use a lock file to prevent multiple instances from running simultaneously.
	lockFile, err := os.OpenFile(config.LockFilePath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		log.Fatalf("❌ Could not create lock file: %v", err)
	}
	defer func() {
		lockFile.Close()
		os.Remove(config.LockFilePath)
	}()

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		log.Fatal("❌ Another instance is already running (failed to acquire file lock)")
	}
	_, _ = lockFile.Seek(0, 0)
	_ = lockFile.Truncate(0)
	fmt.Fprintf(lockFile, "%d", os.Getpid())

	// --- Configuration & Logger ---
	// Define a command-line flag for the config file path.
	configPath := flag.String("config", "", "path to the configuration file")
	flag.Parse()

	// Check if the config file was provided.
	if configPath == nil || *configPath == "" {
		log.Fatal("❌ Configuration file path must be provided using the -config flag")
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
		slog.String("management_server", cfg.GRPC.ManagementAddress),
	)

	// --- Infrastructure Initialization ---
	db, err := database.NewDBPool(ctx, cfg.Database)
	if err != nil {
		slog.Error("Failed to initialize database connection", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	slog.Info("Database connection pool established")

	repoContainer := repository.New()

	// Initialize gRPC client for the Python gateway
	gatewayClient, err := execution.NewGatewayClient(&cfg.GRPC)
	if err != nil {
		slog.Error("Failed to connect to python-gateway", "error", err)
		os.Exit(1)
	}
	defer gatewayClient.Close()
	slog.Info("Connected to python-gateway successfully")

	// Initialize the management gRPC server
	managementServer := api.NewManagementServer(slog.Default(), db, repoContainer)
	grpcServer := grpc.NewServer()
	pb.RegisterManagementServiceServer(grpcServer, managementServer)

	lis, err := net.Listen("tcp", cfg.GRPC.ManagementAddress)
	if err != nil {
		slog.Error("Failed to listen for management API", "error", err, "address", cfg.GRPC.ManagementAddress)
		os.Exit(1)
	}

	go func() {
		if err := grpcServer.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			slog.Error("Management gRPC server stopped with error", "error", err)
		}
	}()

	// Initialize execution service
	execService := execution.NewService(slog.Default(), db, gatewayClient, repoContainer)
	slog.Info("Execution service initialized")

	pf := portfolio.NewPortfolio(slog.Default(), db, repoContainer)
	recon := reconcil.NewReconciler(slog.Default(), db, repoContainer, execService, pf)

	// --- Background Tasks ---
	bgManager := background.NewManager(slog.Default())

	setupHealthMonitor(cfg, execService, bgManager)
	setupOrderSync(cfg, recon, bgManager)
	setupPositionSync(cfg, execService, pf, recon, bgManager)
	setupTradeAudit(cfg, recon, bgManager)

	bgManager.Start(ctx)

	// --- Orchestration ---
	orch, err := orchestrator.New(slog.Default(), db, repoContainer, cfg, pf, recon, execService)
	if err != nil {
		slog.Error("Failed to initialize Orchestrator", "error", err)
		os.Exit(1)
	}
	defer orch.Close()

	go func() {
		if err := orch.Start(ctx); err != nil && err != context.Canceled {
			slog.Error("Orchestrator stopped with error", "error", err)
		}
	}()

	// --- Graceful Shutdown ---
	// Block until a shutdown signal is received.
	<-ctx.Done()

	slog.Info("Shutdown signal received. Starting graceful shutdown...")

	// Perform cleanup operations.
	grpcServer.GracefulStop()
	bgManager.Wait()

	slog.Info("All background tasks stopped. Closing connections...")

	if cfg.Server.ShutdownTimeout > 0 {
		slog.Info("Waiting for shutdown delay", "duration", cfg.Server.ShutdownTimeout.String())
		time.Sleep(cfg.Server.ShutdownTimeout)
	}

	slog.Info("Server shutdown complete.")
}
