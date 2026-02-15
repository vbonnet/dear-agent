package main

import (
	"fmt"
	"os"

	"github.com/vbonnet/ai-tools/astrocyte/pkg/enforcement"
)

func main() {
	// Load bash patterns
	patterns, err := enforcement.LoadPatternsByType("bash")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading patterns: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded %d bash patterns\n\n", len(patterns.Patterns))

	// Create detector
	detector, err := enforcement.NewDetector(patterns)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating detector: %v\n", err)
		os.Exit(1)
	}

	// Show skipped patterns (unsupported regex features)
	if len(detector.GetSkippedPatterns()) > 0 {
		fmt.Printf("Note: Skipped %d patterns with unsupported regex features:\n", len(detector.GetSkippedPatterns()))
		for _, id := range detector.GetSkippedPatterns() {
			fmt.Printf("  - %s\n", id)
		}
		fmt.Println()
	}

	// Test commands
	commands := []string{
		"cd /repo && git push",
		"cat file.txt",
		"grep 'pattern' file.txt",
		"git status",
		"ls -la",
	}

	fmt.Println("Testing violation detection:\n")

	for _, cmd := range commands {
		pattern, err := detector.Detect(cmd)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error detecting violations: %v\n", err)
			continue
		}

		if pattern != nil {
			fmt.Printf("❌ VIOLATION: %s\n", cmd)
			fmt.Printf("   Pattern: %s\n", pattern.ID)
			fmt.Printf("   Reason: %s\n", pattern.Reason)
			fmt.Printf("   Alternative: %s\n", pattern.Alternative)
			fmt.Printf("   Severity: %s\n\n", pattern.Severity)
		} else {
			fmt.Printf("✅ OK: %s\n\n", cmd)
		}
	}
}
