// Package astrocyte provides astrocyte functionality.
package astrocyte

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/eventbus"
)

var logger = slog.Default()

// Example integration showing how to use the Astrocyte watcher with the eventbus.
// This can be called from the AGM daemon or any other component that wants to
// monitor Astrocyte incidents and broadcast them as events.
//
// Usage in AGM daemon:
//
//	hub := eventbus.NewHub()
//	go hub.Run()
//
//	// Start WebSocket server
//	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
//		eventbus.ServeWebSocket(hub, w, r)
//	})
//	go http.ListenAndServe(":8080", nil)
//
//	// Start Astrocyte watcher
//	watcher := astrocyte.NewWatcher(hub, "", 15*time.Minute)
//	if err := watcher.Start(); err != nil {
//		log.Fatalf("Failed to start Astrocyte watcher: %v", err)
//	}
//	defer watcher.Stop()

// StartAstrocyteMonitoring initializes and starts the Astrocyte incident watcher.
// This is a helper function that can be integrated into the AGM daemon.
//
// Parameters:
//   - hub: The eventbus Hub to broadcast events to
//   - incidentsFile: Path to Astrocyte incidents.jsonl file (empty string uses default ~/.agm/astrocyte/incidents.jsonl)
//   - escalationWindow: Time window to prevent duplicate escalations (0 uses default 15 minutes)
//
// Returns:
//   - *Watcher: The running watcher instance (call Stop() to cleanup)
//   - error: Any initialization error
func StartAstrocyteMonitoring(hub *eventbus.Hub, incidentsFile string, escalationWindow time.Duration) (*Watcher, error) {
	watcher := NewWatcher(hub, incidentsFile, escalationWindow)
	if err := watcher.Start(); err != nil {
		return nil, err
	}
	logger.Info("Astrocyte monitoring started")
	return watcher, nil
}

// ExampleIntegration demonstrates a complete integration of the eventbus
// and Astrocyte watcher. This can be used as a template for integrating
// into the AGM daemon.
func ExampleIntegration() {
	// 1. Create and start the eventbus hub
	hub := eventbus.NewHub()
	go hub.Run()
	logger.Info("EventBus hub started")

	// 2. Start WebSocket server for clients to connect
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		eventbus.ServeWebSocket(hub, w, r)
	})

	port := eventbus.GetPort()
	go func() {
		logger.Info("WebSocket server listening", "port", port)
		if err := http.ListenAndServe(":8080", nil); err != nil { //nolint:gosec // G114: example/demo code
			logger.Warn("WebSocket server error", "error", err)
		}
	}()

	// 3. Start Astrocyte watcher
	watcher := NewWatcher(hub, "", 15*time.Minute)
	if err := watcher.Start(); err != nil {
		logger.Error("Failed to start Astrocyte watcher", "error", err)
		return
	}
	defer watcher.Stop()

	// 4. Simulate running (in production, this would be the daemon's main loop)
	logger.Info("Integration running, press Ctrl+C to stop")
	select {} // Block forever (in production, wait for shutdown signal)
}
