package scanners

import (
	"context"
	"time"

	"github.com/vbonnet/dear-agent/engram/internal/metacontext"
)

// Scanner analyzes a working directory and returns detected signals.
// Implementations: FileScanner, DependencyScanner, GitScanner, ConversationScanner.
type Scanner interface {
	// Name returns the scanner identifier (used in telemetry, errors).
	Name() string

	// Scan analyzes the working directory and returns signals.
	// Context propagates timeout and cancellation.
	Scan(ctx context.Context, req *metacontext.AnalyzeRequest) ([]metacontext.Signal, error)

	// Priority returns execution priority (10-50, higher = runs first).
	// Used for ordering parallel scanner execution in fan-out pattern.
	Priority() int
}

// ScanResult contains the result of a single scanner execution.
// Used in fan-out/fan-in orchestration pattern.
type ScanResult struct {
	Scanner string               // Scanner name
	Signals []metacontext.Signal // Detected signals
	Error   error                // Scan error (nil if successful)
	Timing  time.Duration        // Execution time
}
