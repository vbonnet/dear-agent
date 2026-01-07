package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/[REDACTED_EMPLOYER]-src/ai-tools/autonomous-swarm/pkg/csm"
	"github.com/[REDACTED_EMPLOYER]-src/ai-tools/autonomous-swarm/pkg/executor"
	"github.com/[REDACTED_EMPLOYER]-src/ai-tools/autonomous-swarm/pkg/taskqueue"
	"github.com/[REDACTED_EMPLOYER]-src/ai-tools/autonomous-swarm/pkg/telemetry"
)

const version = "0.1.0"

func main() {
	// Define flags
	var (
		queuePath   = flag.String("queue", "", "Path to TASK-QUEUE.yaml file (required)")
		beadID      = flag.String("bead-id", "", "Bead ID to execute (required)")
		sessionName = flag.String("session", "", "CSM session name (required)")
		showVersion = flag.Bool("version", false, "Show version and exit")
		showHelp    = flag.Bool("help", false, "Show help and exit")
	)

	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("swarm-executor version %s\n", version)
		os.Exit(0)
	}

	// Handle help flag
	if *showHelp {
		printUsage()
		os.Exit(0)
	}

	// Validate required flags
	if *queuePath == "" || *beadID == "" || *sessionName == "" {
		fmt.Fprintf(os.Stderr, "Error: Missing required flags\n\n")
		printUsage()
		os.Exit(1)
	}

	// Execute bead
	exitCode := executeBead(*queuePath, *beadID, *sessionName)
	os.Exit(exitCode)
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `swarm-executor - Autonomous bead execution harness

Usage:
  swarm-executor --queue <path> --bead-id <id> --session <name>
  swarm-executor --version
  swarm-executor --help

Flags:
  --queue <path>    Path to TASK-QUEUE.yaml file (required)
  --bead-id <id>    Bead ID to execute (required)
  --session <name>  CSM session name for execution (required)
  --version         Show version and exit
  --help            Show this help and exit

Exit Codes:
  0  Success - Bead executed successfully
  1  Error - Execution failed (see stderr for details)
  2  Escalation - Bead requires human intervention

Examples:
  # Execute a bead
  swarm-executor --queue ./TASK-QUEUE.yaml --bead-id bead-1 --session session-1

  # Show version
  swarm-executor --version
`)
}

func executeBead(queuePath string, beadID string, sessionName string) int {
	// Initialize telemetry
	workDir := filepath.Dir(queuePath)
	logFile := filepath.Join(workDir, "EXECUTION-LOG.jsonl")
	roadmapFile := filepath.Join(workDir, "ROADMAP.md")

	logger := telemetry.NewLogger(logFile)

	// Log execution start
	startEvent := &telemetry.ExecutionEvent{
		BeadID: beadID,
		Event:  "execute",
		Details: map[string]interface{}{
			"session": sessionName,
			"action":  "start",
		},
	}
	if err := logger.LogEvent(startEvent); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to log start event: %v\n", err)
	}

	// Initialize task queue coordinator
	coord := taskqueue.NewCoordinator(queuePath)
	if err := coord.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load task queue: %v\n", err)
		logError(logger, beadID, fmt.Sprintf("queue load failed: %v", err))
		return 1
	}

	// Initialize CSM orchestrator and prompter
	orch := csm.NewOrchestrator()
	prompter := csm.NewPrompter()

	// Initialize execution harness (max 3 iterations)
	harness := executor.NewHarness(coord, orch, prompter, 3)

	// Execute bead
	fmt.Printf("Executing bead %s in session %s...\n", beadID, sessionName)

	err := harness.ExecuteBead(beadID, sessionName)

	if err != nil {
		// Check if escalation error
		if executor.IsEscalationError(err) {
			fmt.Fprintf(os.Stderr, "Escalation: %v\n", err)
			logError(logger, beadID, fmt.Sprintf("escalation: %v", err))

			// Generate roadmap
			if roadmapErr := telemetry.GenerateRoadmap(queuePath, roadmapFile); roadmapErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: Failed to generate roadmap: %v\n", roadmapErr)
			}

			return 2 // Escalation exit code
		}

		// Regular error
		fmt.Fprintf(os.Stderr, "Error: Execution failed: %v\n", err)
		logError(logger, beadID, fmt.Sprintf("execution failed: %v", err))

		// Generate roadmap
		if roadmapErr := telemetry.GenerateRoadmap(queuePath, roadmapFile); roadmapErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to generate roadmap: %v\n", roadmapErr)
		}

		return 1 // Error exit code
	}

	// Success - log completion
	completeEvent := &telemetry.ExecutionEvent{
		BeadID: beadID,
		Event:  "complete",
		Details: map[string]interface{}{
			"session": sessionName,
			"status":  "success",
		},
	}
	if err := logger.LogEvent(completeEvent); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to log completion event: %v\n", err)
	}

	// Generate roadmap
	if err := telemetry.GenerateRoadmap(queuePath, roadmapFile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to generate roadmap: %v\n", err)
	}

	fmt.Printf("✓ Bead %s executed successfully\n", beadID)
	return 0 // Success exit code
}

func logError(logger *telemetry.Logger, beadID string, message string) {
	errorEvent := &telemetry.ExecutionEvent{
		BeadID: beadID,
		Event:  "error",
		Details: map[string]interface{}{
			"message": message,
		},
	}
	_ = logger.LogEvent(errorEvent) // Ignore logging errors during error handling
}
