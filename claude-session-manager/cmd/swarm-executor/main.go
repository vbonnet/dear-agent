package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Configuration flags
	maxParallel   int
	batchMode     string
	safetyMargin  float64
	allowOverride bool
	queuePath     string
	verbose       bool
)

const (
	// Default safe batch size from research (Task 0.3)
	defaultMaxParallel = 15

	// Batch modes
	batchModeAuto   = "auto"
	batchModeManual = "manual"
	batchModeOff    = "off"
)

var rootCmd = &cobra.Command{
	Use:   "swarm-executor",
	Short: "Execute swarms with automatic batch size validation",
	Long: `Swarm Executor - Safe parallel agent execution with automatic batching

Based on research from deadlock investigation (38-agent swarm failure):
- Safe limit: 15 agents per parallel execution
- Formula: min(CPU_cores × 2, 15)
- Safety margin: 60% from observed failure point

Features:
- Pre-flight validation of swarm size
- Automatic batching for large swarms
- Configurable limits and behavior
- Progress tracking per wave

Examples:
  # Launch swarm with default settings (15 agent limit)
  swarm-executor execute --queue tasks.yaml

  # Override limit (use with caution)
  swarm-executor execute --queue tasks.yaml --max-parallel 20

  # Disable batching (risk of deadlock)
  swarm-executor execute --queue tasks.yaml --batch-mode off
`,
	Version: "1.0.0",
}

func main() {
	// Add persistent flags
	rootCmd.PersistentFlags().IntVar(&maxParallel, "max-parallel", defaultMaxParallel, "Maximum parallel agents (safe limit)")
	rootCmd.PersistentFlags().StringVar(&batchMode, "batch-mode", batchModeAuto, "Batching mode: auto, manual, off")
	rootCmd.PersistentFlags().Float64Var(&safetyMargin, "safety-margin", 0.6, "Safety margin from failure point (0.6 = 60%)")
	rootCmd.PersistentFlags().BoolVar(&allowOverride, "allow-override", false, "Allow overriding safe limits with confirmation")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "Verbose output")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
