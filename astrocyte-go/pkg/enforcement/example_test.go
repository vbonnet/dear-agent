package enforcement_test

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/ai-tools/astrocyte/pkg/enforcement"
)

// Example_completeViolationFlow demonstrates the complete violation detection,
// message generation, and filing workflow.
func Example_completeViolationFlow() {
	// Step 1: Load pattern database
	home, _ := os.UserHomeDir()
	patternsPath := filepath.Join(home, "src", "ws", "oss", "repos", "engram", "patterns", "bash-anti-patterns.yaml")

	db, err := enforcement.LoadPatterns(patternsPath)
	if err != nil {
		log.Fatalf("Failed to load patterns: %v", err)
	}

	// Step 2: Create detector
	detector, err := enforcement.NewDetector(db)
	if err != nil {
		log.Fatalf("Failed to create detector: %v", err)
	}

	// Step 3: Detect violation in command
	suspiciousCommand := "cd /home/user/repo && git push origin main"
	pattern, err := detector.Detect(suspiciousCommand)
	if err != nil {
		log.Fatalf("Detection failed: %v", err)
	}

	if pattern != nil {
		fmt.Printf("Violation detected: %s\n", pattern.ID)

		// Step 4: Generate rejection message
		message := enforcement.GenerateRejectionMessage(pattern, suspiciousCommand)
		fmt.Printf("\nRejection Message:\n%s\n", message)

		// Step 5: File violation (to temporary directory for example)
		tmpDir := os.TempDir()
		violation := enforcement.ViolationData{
			PatternID:   pattern.ID,
			PatternType: "bash",
			Command:     suspiciousCommand,
			SessionID:   "example-session",
			AgentType:   "general-purpose",
			Timestamp:   time.Now(),
		}

		violationPath, err := enforcement.FileViolation(violation, tmpDir, pattern)
		if err != nil {
			log.Fatalf("Failed to file violation: %v", err)
		}

		fmt.Printf("\nViolation filed to: %s\n", violationPath)
	} else {
		fmt.Println("No violation detected")
	}
}

// Example_messageGeneration demonstrates different rejection message formats.
func Example_messageGeneration() {
	pattern := &enforcement.Pattern{
		ID:          "cd-chaining",
		Reason:      "Command chaining with cd",
		Alternative: "Use tool-specific -C flag (e.g., git -C /path)",
		Severity:    "high",
		Tier1Example: "❌ BAD: cd /repo && git push\n✅ GOOD: git -C /repo push",
	}

	command := "cd /repo && git push"

	// Basic message
	fmt.Println("=== Basic Rejection Message ===")
	basicMsg := enforcement.GenerateRejectionMessage(pattern, command)
	fmt.Println(basicMsg)

	fmt.Println("\n=== Short Message ===")
	shortMsg := enforcement.GenerateShortRejectionMessage(pattern)
	fmt.Println(shortMsg)

	fmt.Println("\n=== Message with Severity ===")
	severityMsg := enforcement.GenerateRejectionMessageWithSeverity(pattern, command)
	fmt.Println(severityMsg)
}

// Example_patternDetection demonstrates pattern detection capabilities.
func Example_patternDetection() {
	// Create a simple pattern database
	db := &enforcement.PatternDatabase{
		Patterns: []enforcement.Pattern{
			{
				ID:          "cd-chaining",
				Regex:       `cd\s+[^\s]+\s+&&`,
				Reason:      "Command chaining with cd",
				Alternative: "Use tool-specific -C flag",
				Severity:    "high",
			},
			{
				ID:          "cat-file-read",
				Regex:       `^cat\s+[^\|]+$`,
				Reason:      "Using cat to read files",
				Alternative: "Use Read tool",
				Severity:    "high",
			},
		},
	}

	detector, _ := enforcement.NewDetector(db)

	// Test various commands
	testCommands := []string{
		"cd /repo && git push",
		"cat config.yaml",
		"git -C /repo push",  // Valid command, should not match
	}

	for _, cmd := range testCommands {
		pattern, _ := detector.Detect(cmd)
		if pattern != nil {
			fmt.Printf("Command: %s\n", cmd)
			fmt.Printf("  → Violation: %s\n", pattern.ID)
			fmt.Printf("  → Reason: %s\n\n", pattern.Reason)
		} else {
			fmt.Printf("Command: %s\n", cmd)
			fmt.Printf("  → OK (no violation)\n\n")
		}
	}
}

// Example_violationFiling demonstrates violation file creation.
func Example_violationFiling() {
	tmpDir := os.TempDir()

	pattern := &enforcement.Pattern{
		ID:          "cd-chaining",
		Reason:      "Command chaining with cd",
		Alternative: "Use tool-specific -C flag (e.g., git -C /path)",
		Severity:    "high",
	}

	violation := enforcement.ViolationData{
		PatternID:          "cd-chaining",
		PatternType:        "bash",
		Command:            "cd /repo && git push",
		SessionID:          "example-session",
		AgentType:          "general-purpose",
		TaskCategory:       "version_control",
		ConversationLength: 10,
		Tags:               []string{"cd", "git", "chaining"},
		Timestamp:          time.Now(),
	}

	filepath, err := enforcement.FileViolation(violation, tmpDir, pattern)
	if err != nil {
		log.Fatalf("Failed to file violation: %v", err)
	}

	fmt.Printf("Violation file created: %s\n", filepath)

	// Read and display part of the file
	content, _ := os.ReadFile(filepath)
	fmt.Println("\nFile content (first 500 chars):")
	if len(content) > 500 {
		fmt.Printf("%s...\n", string(content[:500]))
	} else {
		fmt.Println(string(content))
	}
}
