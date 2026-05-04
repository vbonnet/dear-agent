package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/vbonnet/dear-agent/agm/internal/daemon"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/logging"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
	sentinelcfg "github.com/vbonnet/dear-agent/agm/internal/sentinel/config"
	sentineldaemon "github.com/vbonnet/dear-agent/agm/internal/sentinel/daemon"
)

var logger = logging.DefaultLogger()

func main() {
	if err := run(); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}

func run() error {
	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	baseDir := filepath.Join(homeDir, ".agm")
	logDir := filepath.Join(baseDir, "logs", "daemon")
	pidFile := filepath.Join(baseDir, "daemon.pid")

	// Create log directory
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Create daemon logger (uses slog.Logger with file output)
	logPath := filepath.Join(logDir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()

	daemonLogger := logging.NewTextLogger(logFile)

	// Open message queue
	queue, err := messages.NewMessageQueue()
	if err != nil {
		return fmt.Errorf("failed to open message queue: %w", err)
	}

	// Create acknowledgment manager
	ackManager := messages.NewAckManager(queue)

	doltAdapter := initDoltAdapter(daemonLogger)
	if doltAdapter != nil {
		defer doltAdapter.Close()
	}

	// Create daemon config
	cfg := daemon.Config{
		BaseDir:     baseDir,
		LogDir:      logDir,
		PIDFile:     pidFile,
		Queue:       queue,
		AckManager:  ackManager,
		Logger:      daemonLogger,
		DoltAdapter: doltAdapter,
	}

	// Create daemon
	d := daemon.NewDaemon(cfg)

	sentinel := startSentinel(daemonLogger)

	// Log startup to both stdout and file
	fmt.Println("AGM Daemon starting...")
	fmt.Printf("  Base dir: %s\n", baseDir)
	fmt.Printf("  Log dir: %s\n", logDir)
	fmt.Printf("  PID file: %s\n", pidFile)
	daemonLogger.Info("AGM Daemon starting...")

	// Start daemon (blocks until stopped via signal)
	if err := d.Start(); err != nil {
		if sentinel != nil {
			sentinel.StopMonitoring()
		}
		return fmt.Errorf("daemon failed: %w", err)
	}

	// Stop sentinel after daemon exits
	if sentinel != nil {
		sentinel.StopMonitoring()
	}
	return nil
}

// initDoltAdapter constructs the Dolt adapter and applies migrations. Returns
// nil (with warnings logged) when Dolt is unavailable so the daemon can fall
// back to YAML-only session resolution.
func initDoltAdapter(daemonLogger *slog.Logger) *dolt.Adapter {
	doltConfig, err := dolt.DefaultConfig()
	if err != nil {
		daemonLogger.Warn("Dolt config not available, session resolution will use YAML fallback", "error", err)
		return nil
	}
	adapter, err := dolt.New(doltConfig)
	if err != nil {
		daemonLogger.Warn("Dolt connection failed, session resolution will use YAML fallback", "error", err)
		return nil
	}
	if err := adapter.ApplyMigrations(); err != nil {
		adapter.Close()
		daemonLogger.Warn("Dolt migrations failed", "error", err)
		return nil
	}
	return adapter
}

// startSentinel loads the sentinel config and launches the session monitor
// goroutine. Returns nil (with warnings logged) if sentinel cannot start.
func startSentinel(daemonLogger *slog.Logger) *sentineldaemon.SessionMonitor {
	sentinelCfg, err := sentinelcfg.LoadConfig(sentinelcfg.DefaultConfigPath())
	if err != nil {
		daemonLogger.Warn("Sentinel config load failed, sentinel disabled", "error", err)
		return nil
	}
	if sentinelCfg == nil {
		return nil
	}
	sentinelCfg.ExpandPaths()
	monitor, err := sentineldaemon.NewSessionMonitor(sentinelCfg)
	if err != nil {
		daemonLogger.Warn("Sentinel init failed, sentinel disabled", "error", err)
		return nil
	}
	go func() {
		daemonLogger.Info("Sentinel session monitor starting...")
		if err := monitor.StartMonitoring(); err != nil {
			daemonLogger.Error("Sentinel monitor failed", "error", err)
		}
	}()
	return monitor
}
