package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/reaper"
)

// Version information - set via ldflags at build time
var (
	Version   = "2.0.0-dev"
	GitCommit = "unknown"
	BuildDate = "unknown"
	BuiltBy   = "unknown"
)

func main() {
	// Print header to stderr
	executable, err := os.Executable()
	if err != nil {
		executable = "unknown"
	}
	fmt.Fprintf(os.Stderr, "csm-reaper %s (%s)\n", Version, executable)
	// Parse args: csm-reaper --session <name> --log-file <path> --sessions-dir <path>
	if len(os.Args) < 5 {
		fmt.Fprintf(os.Stderr, "Usage: csm-reaper --session <name> --log-file <path> [--sessions-dir <path>]\n")
		os.Exit(1)
	}

	// Simple arg parsing
	var sessionName, logFile, sessionsDir string
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--session":
			if i+1 < len(os.Args) {
				sessionName = os.Args[i+1]
				i++
			}
		case "--log-file":
			if i+1 < len(os.Args) {
				logFile = os.Args[i+1]
				i++
			}
		case "--sessions-dir":
			if i+1 < len(os.Args) {
				sessionsDir = os.Args[i+1]
				i++
			}
		}
	}

	if sessionName == "" || logFile == "" {
		fmt.Fprintf(os.Stderr, "Error: --session and --log-file are required\n")
		fmt.Fprintf(os.Stderr, "Usage: csm-reaper --session <name> --log-file <path> [--sessions-dir <path>]\n")
		os.Exit(1)
	}

	// Setup logging to file (0600 permissions for security - logs may contain sensitive info)
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open log file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	log.SetOutput(f)
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Printf("=== Reaper started at %s ===", time.Now().Format(time.RFC3339))
	log.Printf("Session: %s", sessionName)
	log.Printf("PID: %d", os.Getpid())
	log.Printf("Log file: %s", logFile)
	log.Printf("Sessions directory: %s", sessionsDir)

	// Run reaper
	r := reaper.New(sessionName, sessionsDir)
	if err := r.Run(); err != nil {
		log.Printf("❌ Reaper failed: %v", err)
		os.Exit(1)
	}

	log.Printf("✓ Reaper completed successfully")
	log.Printf("=== Reaper finished at %s ===", time.Now().Format(time.RFC3339))
}
