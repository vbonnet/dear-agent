package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/logging"
	"github.com/vbonnet/dear-agent/agm/internal/reaper"
)

var logger = logging.DefaultLogger()

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func run() error {
	sessionName := flag.String("session", "", "Session name to archive")
	logFile := flag.String("log-file", "", "Log file path")
	sessionsDir := flag.String("sessions-dir", "", "Sessions directory")
	flag.Parse()

	// Validate required flags
	if *sessionName == "" {
		flag.Usage()
		return fmt.Errorf("--session flag is required")
	}

	// Set up logging
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return fmt.Errorf("failed to open log file %s: %w", *logFile, err)
		}
		defer f.Close()
		// Create logger with file output
		opts := &slog.HandlerOptions{Level: slog.LevelInfo}
		logger = slog.New(slog.NewTextHandler(f, opts))
		slog.SetDefault(logger)
	}

	// Log startup
	logger.Info("Reaper started", "timestamp", time.Now().UTC().Format(time.RFC3339))
	logger.Info("Reaper configuration", "session", *sessionName, "pid", os.Getpid(), "log_file", *logFile, "sessions_dir", *sessionsDir)

	// Create and run reaper
	r := reaper.New(*sessionName, *sessionsDir)
	if err := r.Run(); err != nil {
		return fmt.Errorf("reaper failed: %w", err)
	}

	logger.Info("Reaper completed successfully")
	return nil
}
