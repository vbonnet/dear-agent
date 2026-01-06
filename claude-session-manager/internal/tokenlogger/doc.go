// Package tokenlogger provides telemetry event logging for CSM (Claude Session Manager).
//
// This package implements the engram telemetry.EventListener interface to automatically
// log telemetry events to CSM session directories in JSONL format.
//
// # Purpose
//
// Enable CSM to capture engram telemetry events in session logs for debugging and analysis.
// Events are written to ~/src/sessions/{uuid}/token-usage.jsonl when CSM session is active.
//
// # Basic Usage
//
//	import (
//	    "github.com/vbonnet/ai-tools/claude-session-manager/internal/tokenlogger"
//	    "github.com/vbonnet/engram/core/pkg/telemetry"
//	)
//
//	// Create token logger
//	logger := tokenlogger.NewTokenLogger()
//
//	// Register with engram Collector (in engram CLI initialization)
//	collector.AddListener(logger)
//
//	// Events are automatically logged when CSM session is active
//	// No additional code needed - telemetry events flow automatically
//
// # V1 Scope (P3 Prototype)
//
//   - EventListener implementation in ai-tools
//   - Manual plugin registration (edit engram CLI root.go)
//   - ERROR-level filtering only (default MinLevel)
//   - JSONL file format with 0600 permissions
//   - Graceful degradation when no CSM session
//
// # Out of Scope for V1
//
//   - Production plugin registration system (config-based or auto-discovery)
//   - Log rotation or file size limits
//   - Event aggregation or summarization
//   - Configurable log levels per user
//   - Plugin marketplace integration
//
// # Integration Steps
//
// 1. Modify engram CLI (~/src/ws/oss/repos/engram/main/core/cmd/engram/cmd/root.go):
//
//	import (
//	    "github.com/vbonnet/ai-tools/claude-session-manager/internal/tokenlogger"
//	    "github.com/vbonnet/engram/core/internal/telemetry"
//	)
//
//	var GlobalCollector *telemetry.Collector
//
//	var rootCmd = &cobra.Command{
//	    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
//	        return initTelemetry()
//	    },
//	}
//
//	func initTelemetry() error {
//	    // Load config
//	    cfg, err := config.Load()
//	    if err != nil {
//	        return nil // Graceful degradation
//	    }
//
//	    // Initialize Collector
//	    GlobalCollector, err = telemetry.NewCollector(cfg.Telemetry.Enabled, cfg.Telemetry.Path)
//	    if err != nil {
//	        fmt.Fprintf(os.Stderr, "Warning: Failed to initialize telemetry: %v\n", err)
//	        return nil
//	    }
//
//	    // Register CSM token logger plugin
//	    GlobalCollector.AddListener(tokenlogger.NewTokenLogger())
//
//	    return nil
//	}
//
// 2. Ensure go.mod replace directive exists:
//
//	replace github.com/vbonnet/engram/core => ../../../engram/main/core
//
// 3. Run engram commands within CSM session:
//
//	csm new my-session
//	engram retrieve some-knowledge
//	cat ~/src/sessions/{uuid}/token-usage.jsonl
//
// # Error Handling
//
// TokenLogger gracefully handles all error conditions:
//
//   - No CSM session: Events dropped silently (return nil)
//   - CSM not installed: Events dropped silently
//   - Session directory missing: Events dropped silently (CSM creates directories)
//   - File write errors: Errors propagated to Collector (logged but doesn't crash)
//   - Concurrent writes: Thread-safe with sync.Mutex and O_APPEND
//
// # Performance
//
//   - File writes: <500μs per event (local filesystem, SSD)
//   - Session detection: <10ms first call, <1μs cached
//   - Memory overhead: <1KB per TokenLogger instance
//   - No goroutine leaks (no background workers)
//
// # Testing
//
// Run tests with race detector:
//
//	go test ./internal/tokenlogger/... -v -race
//	go test ./internal/tokenlogger/... -cover
//
// Coverage target: >80%
//
// # Architecture
//
// This package is part of the P3 CSM Token Logger Plugin project following
// the Wayfinder methodology (D1-D4 discovery, S4-S11 delivery).
//
// See project deliverables:
//   - D3: Plugin Registration System architecture
//   - D4: Solution requirements and specifications
//   - S5: Implementation research (P1 patterns)
//   - S6: Detailed design (API, data flow, error handling)
//   - S7: Implementation plan (task breakdown, dependencies)
//
// # Related
//
//   - P1 Telemetry Foundation: core/internal/telemetry
//   - CSM Session Manager: claude-session-manager/
//   - Wayfinder session ID: c0144e65-25e2-4471-971d-8e987c57a53b
package tokenlogger
