package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/vbonnet/ai-tools/astrocyte/internal/config"
	"github.com/vbonnet/ai-tools/astrocyte/internal/daemon"
)

var (
	// Version information (set via -ldflags during build)
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	// Command-line flags
	var (
		configPath   = flag.String("config", config.DefaultConfigPath(), "Path to configuration file")
		checkOnce    = flag.Bool("check", false, "Check all sessions once and exit (don't run daemon)")
		versionFlag  = flag.Bool("version", false, "Show version information")
		verboseFlag  = flag.Bool("verbose", false, "Enable verbose logging")
		logLevel     = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	)
	flag.Parse()

	// Show version
	if *versionFlag {
		fmt.Printf("astrocyte version %s (commit %s, built %s)\n", version, commit, date)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Override verbose setting from CLI flag
	if *verboseFlag {
		cfg.Logging.Verbose = true
	}

	// Expand paths in configuration
	cfg.ExpandPaths()

	// Configure logging
	configureLogging(*logLevel, cfg.Logging.Verbose)

	log.Printf("Astrocyte daemon starting (version %s)", version)
	if cfg.Logging.Verbose {
		log.Printf("Configuration loaded from: %s", *configPath)
		log.Printf("Check interval: %v", cfg.Monitoring.IntervalDuration)
		log.Printf("Stuck threshold: %v", cfg.Monitoring.StuckThresholdDuration)
		log.Printf("Recovery strategy: %s", cfg.Recovery.Strategy)
	}

	// Create session monitor
	monitor, err := daemon.NewSessionMonitor(cfg)
	if err != nil {
		log.Fatalf("Failed to create session monitor: %v", err)
	}

	// If check-once mode, run single check and exit
	if *checkOnce {
		log.Printf("Running single session check...")
		if err := monitor.CheckAllSessions(); err != nil {
			log.Fatalf("Session check failed: %v", err)
		}
		log.Printf("Session check complete")
		os.Exit(0)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start monitoring in background goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- monitor.StartMonitoring()
	}()

	// Wait for shutdown signal or error
	select {
	case <-sigChan:
		log.Printf("Received shutdown signal, stopping gracefully...")
		monitor.StopMonitoring()

	case err := <-errChan:
		if err != nil {
			log.Fatalf("Monitor failed: %v", err)
		}
	}

	log.Printf("Astrocyte daemon stopped")
}

// configureLogging sets up logging based on level and verbose flag.
func configureLogging(level string, verbose bool) {
	// Set log flags
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	// If verbose, include file/line information
	if verbose {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)
	}

	// TODO: Implement log level filtering
	// For now, we log everything at INFO level
	// In production, would use structured logging library (e.g., zerolog, zap)
}
