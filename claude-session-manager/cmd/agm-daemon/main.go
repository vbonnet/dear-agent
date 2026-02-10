package main

import (
	"flag"
	"log"
	"time"

	"github.com/vbonnet/ai-tools/main/claude-session-manager/internal/daemon"
)

func main() {
	// Parse flags
	port := flag.Int("port", 8765, "HTTP API port")
	statusDir := flag.String("status-dir", "~/.agm/status", "Status file directory")
	pollInterval := flag.Duration("poll-interval", 2*time.Second, "Session polling interval")

	flag.Parse()

	// Create and start daemon
	d, err := daemon.NewDaemon(*port, *statusDir, *pollInterval)
	if err != nil {
		log.Fatalf("Failed to create daemon: %v", err)
	}

	log.Println("AGM Daemon starting...")
	log.Printf("  Port: %d", *port)
	log.Printf("  Status dir: %s", *statusDir)
	log.Printf("  Poll interval: %v", *pollInterval)

	if err := d.Start(); err != nil {
		log.Fatalf("Daemon failed: %v", err)
	}
}
