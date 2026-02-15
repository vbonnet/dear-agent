package main

import (
	"fmt"
	"log"
	"time"

	"github.com/vbonnet/ai-tools/astrocyte/internal/daemon"
	"github.com/vbonnet/ai-tools/astrocyte/internal/tmux"
)

// SessionMonitor demonstrates basic session monitoring using the tmux client and detector.
//
// This example:
// 1. Lists all tmux sessions
// 2. Monitors each session for stuck indicators
// 3. Reports stuck sessions with detailed information
// 4. Tracks cursor movement over time
func main() {
	fmt.Println("Astrocyte Session Monitor - Example")
	fmt.Println("====================================")

	// Create tmux client
	client := tmux.NewClient()
	fmt.Println("✓ Tmux client initialized")

	// Create stuck session detector with default thresholds
	detector := daemon.NewStuckSessionDetector()
	fmt.Printf("✓ Detector initialized (thresholds: mustering=%dm, zero_token=%dm, frozen=%dm)\n",
		detector.MusteringTimeout,
		detector.ZeroTokenWaitingTimeout,
		detector.CursorFrozenTimeout)

	// Monitor loop
	fmt.Println("\nStarting monitoring loop (Ctrl+C to exit)...")
	fmt.Println("")

	ticker := time.NewTicker(60 * time.Second) // Check every minute
	defer ticker.Stop()

	// Run once immediately, then every minute
	checkSessions(client, detector)

	for {
		<-ticker.C
		checkSessions(client, detector)
	}
}

// checkSessions checks all tmux sessions for stuck state.
func checkSessions(client *tmux.Client, detector *daemon.StuckSessionDetector) {
	timestamp := time.Now().Format("15:04:05")
	fmt.Printf("[%s] Checking sessions...\n", timestamp)

	// List all sessions
	sessions, err := client.ListSessions()
	if err != nil {
		log.Printf("Error listing sessions: %v", err)
		return
	}

	if len(sessions) == 0 {
		fmt.Println("  No active tmux sessions found")
		return
	}

	fmt.Printf("  Found %d session(s)\n", len(sessions))

	// Check each session
	healthyCount := 0
	stuckCount := 0

	for _, sessionName := range sessions {
		status := checkSession(client, detector, sessionName)
		if status == "stuck" {
			stuckCount++
		} else if status == "healthy" {
			healthyCount++
		}
	}

	fmt.Printf("  Summary: %d healthy, %d stuck\n", healthyCount, stuckCount)
	fmt.Println("")
}

// checkSession checks a single session and returns status.
func checkSession(client *tmux.Client, detector *daemon.StuckSessionDetector, sessionName string) string {
	// Capture pane state
	pane, err := tmux.CapturePaneInfo(client, sessionName)
	if err != nil {
		log.Printf("  ✗ %s: Error capturing pane: %v", sessionName, err)
		return "error"
	}

	// Track cursor movement
	detector.TrackSession(sessionName, pane.CursorX, pane.CursorY)

	// Detect stuck state
	info := detector.DetectStuckSession(pane)

	if info != nil {
		// Session is stuck
		fmt.Printf("  ⚠ %s: STUCK - %s\n", sessionName, info.Reason)
		fmt.Printf("      Cursor: (%d,%d)\n", info.CursorX, info.CursorY)
		if info.LastCommand != "" {
			fmt.Printf("      Last command: %s\n", info.LastCommand)
		}

		// Show indicators
		fmt.Printf("      Indicators:")
		for name, value := range info.Indicators {
			if value {
				fmt.Printf(" %s", name)
			}
		}
		fmt.Println()

		return "stuck"
	}

	// Check indicators even if not stuck (for debugging)
	indicators := pane.DetectStuckIndicators()

	fmt.Printf("  ✓ %s: Healthy", sessionName)

	// Show status hints
	if indicators["completed"] {
		fmt.Print(" (completed)")
	} else if indicators["idle_prompt"] {
		fmt.Print(" (idle)")
	} else if indicators["waiting"] {
		fmt.Print(" (working)")
	}

	fmt.Println()

	return "healthy"
}

// Example output:
//
// Astrocyte Session Monitor - Example
// ====================================
// ✓ Tmux client initialized
// ✓ Detector initialized (thresholds: mustering=20m, zero_token=15m, frozen=30m)
//
// Starting monitoring loop (Ctrl+C to exit)...
//
// [14:30:00] Checking sessions...
//   Found 3 session(s)
//   ✓ task-123: Healthy (working)
//   ✓ orchestrator: Healthy (idle)
//   ⚠ stuck-task: STUCK - stuck_zero_token_waiting
//       Cursor: (10,20)
//       Last command: git status
//       Indicators: waiting zero_token_waiting
//   Summary: 2 healthy, 1 stuck
//
// [14:31:00] Checking sessions...
//   Found 3 session(s)
//   ✓ task-123: Healthy (completed)
//   ✓ orchestrator: Healthy (idle)
//   ✓ stuck-task: Healthy (working)
//   Summary: 3 healthy, 0 stuck
