package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/reaper"
)

func main() {
	sessionName := flag.String("session", "", "Session name to archive")
	logFile := flag.String("log-file", "", "Log file path")
	sessionsDir := flag.String("sessions-dir", "", "Sessions directory")
	flag.Parse()

	// Validate required flags
	if *sessionName == "" {
		fmt.Fprintln(os.Stderr, "Error: --session flag is required")
		flag.Usage()
		os.Exit(1)
	}

	// Set up logging
	if *logFile != "" {
		f, err := os.OpenFile(*logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to open log file %s: %v\n", *logFile, err)
			os.Exit(1)
		}
		defer f.Close()
		log.SetOutput(f)
	}

	// Log startup
	log.Printf("=== Reaper started at %s ===", time.Now().UTC().Format(time.RFC3339))
	log.Printf("Session: %s", *sessionName)
	log.Printf("PID: %d", os.Getpid())
	log.Printf("Log file: %s", *logFile)
	log.Printf("Sessions directory: %s", *sessionsDir)

	// Create and run reaper
	r := reaper.New(*sessionName, *sessionsDir)
	if err := r.Run(); err != nil {
		log.Printf("❌ Reaper failed: %v", err)
		os.Exit(1)
	}

	log.Printf("✅ Reaper completed successfully")
}
