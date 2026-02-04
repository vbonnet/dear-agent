package main

import (
	"fmt"
	"os"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/hook"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/plugin"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/plugin/claudetasks"
)

// csm-verify-roadmap is a git pre-commit hook that verifies ROADMAP.md consistency
// with task manager state. Blocks commits if mismatches are detected.
//
// Usage:
//   csm-verify-roadmap [roadmap-path] [session-dir]
//
// Exit codes:
//   0 - Verification passed
//   1 - Verification failed (mismatches found)
//   2 - Error during verification

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <roadmap-path> <session-dir>\n", os.Args[0])
		os.Exit(2)
	}

	roadmapPath := os.Args[1]
	sessionDir := os.Args[2]

	// Verify paths exist
	if _, err := os.Stat(roadmapPath); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: ROADMAP.md not found: %s\n", roadmapPath)
		os.Exit(2)
	}

	if _, err := os.Stat(sessionDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Error: Session directory not found: %s\n", sessionDir)
		os.Exit(2)
	}

	// Auto-detect task manager plugin
	taskPlugin := detectTaskManager(sessionDir)
	if taskPlugin == nil {
		fmt.Fprintf(os.Stderr, "Error: No task manager plugin found for session\n")
		fmt.Fprintf(os.Stderr, "Hint: Ensure ROADMAP.md or .beads/ directory exists\n")
		os.Exit(2)
	}

	// Create verifier
	verifier := hook.NewVerifier(taskPlugin)

	// Run verification
	result, err := verifier.VerifyROADMAP(roadmapPath, sessionDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error during verification: %v\n", err)
		os.Exit(2)
	}

	// Check result
	if !result.Passed {
		errorMsg := hook.FormatError(result, taskPlugin.Metadata().Name)
		fmt.Fprint(os.Stderr, errorMsg)
		os.Exit(1) // Verification failed
	}

	// Verification passed
	fmt.Println("✓ ROADMAP.md verification passed")
	os.Exit(0)
}

// detectTaskManager auto-detects which task manager to use
func detectTaskManager(sessionDir string) plugin.TaskManagerPlugin {
	// Try Claude tasks (checks for ROADMAP.md)
	claudePlugin := claudetasks.NewPlugin()
	if claudePlugin.SupportsSession(sessionDir) {
		return claudePlugin
	}

	// Future: Try beads plugin (checks for .beads/ directory)
	// beadsPlugin := beads.NewPlugin()
	// if beadsPlugin.SupportsSession(sessionDir) {
	//     return beadsPlugin
	// }

	return nil
}
