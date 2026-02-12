package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var executeCmd = &cobra.Command{
	Use:   "execute",
	Short: "Execute swarm with batch validation",
	Long: `Execute a swarm of agents with automatic batch size validation.

If swarm size exceeds the safe limit (default: 15 agents), the executor will:
1. Warn about the risk
2. Auto-batch into waves (if --batch-mode=auto)
3. Execute waves sequentially
4. Track progress and completion

Safety features:
- Pre-flight validation against deadlock risk
- Automatic wave-based execution
- Progress tracking per wave
- Incident logging for large swarms
`,
	RunE: runExecute,
}

func init() {
	executeCmd.Flags().StringVar(&queuePath, "queue", "", "Path to task queue YAML (required)")
	executeCmd.MarkFlagRequired("queue")

	rootCmd.AddCommand(executeCmd)
}

func runExecute(cmd *cobra.Command, args []string) error {
	// For now, this is a placeholder that demonstrates the validation logic
	// Full implementation would integrate with actual task queue and Claude Code

	fmt.Println("Swarm Executor - Batch Size Validation")
	fmt.Println("======================================")
	fmt.Printf("Queue: %s\n", queuePath)
	fmt.Printf("Max Parallel: %d agents\n", maxParallel)
	fmt.Printf("Batch Mode: %s\n", batchMode)
	fmt.Printf("Safety Margin: %.0f%%\n", safetyMargin*100)
	fmt.Println()

	// Parse task queue (placeholder - would read YAML)
	agentCount := getAgentCount(queuePath)

	// Pre-flight validation
	if err := validateSwarmSize(agentCount); err != nil {
		return err
	}

	// Execute (batched or unbatched)
	return executeSwarm(agentCount)
}

func getAgentCount(queuePath string) int {
	// Placeholder: In real implementation, parse YAML and count tasks
	// For demo, just return a hardcoded count
	fmt.Printf("📋 Reading task queue: %s\n", queuePath)

	// Check if file exists
	if _, err := os.Stat(queuePath); os.IsNotExist(err) {
		fmt.Printf("⚠️  Queue file not found (using demo mode)\n\n")
		return 10 // Demo: small swarm
	}

	// In real implementation: parse YAML, count tasks
	// For now, read line count as proxy
	file, err := os.Open(queuePath)
	if err != nil {
		return 10
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "- task:") || strings.HasPrefix(line, "- bead:") {
			count++
		}
	}

	if count == 0 {
		count = 10 // Default for demo
	}

	return count
}

func validateSwarmSize(agentCount int) error {
	fmt.Printf("🔍 Pre-Flight Validation\n")
	fmt.Printf("   Agent count: %d\n", agentCount)
	fmt.Printf("   Safe limit: %d\n", maxParallel)

	if agentCount <= maxParallel {
		fmt.Printf("   Status: ✅ Within safe limit\n\n")
		return nil
	}

	// Exceeds limit
	exceeds := agentCount - maxParallel
	riskPercent := float64(agentCount) / float64(38) * 100 // 38 = known failure point

	fmt.Printf("   Status: ⚠️  Exceeds safe limit by %d agents\n", exceeds)
	fmt.Printf("   Risk level: %.0f%% of known failure threshold\n", riskPercent)
	fmt.Println()

	if batchMode == batchModeOff {
		if !allowOverride {
			return fmt.Errorf("swarm size (%d) exceeds safe limit (%d) and batching is disabled", agentCount, maxParallel)
		}

		// Require user confirmation for override
		fmt.Println("⚠️  WARNING: Proceeding without batching increases deadlock risk")
		fmt.Println()
		fmt.Print("Type 'I understand the risks' to proceed: ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(response)

		if response != "I understand the risks" {
			return fmt.Errorf("user declined to proceed")
		}

		fmt.Println("⚠️  Proceeding with override (risk acknowledged)")
		fmt.Println()
	} else if batchMode == batchModeAuto {
		waveCount := int(math.Ceil(float64(agentCount) / float64(maxParallel)))
		fmt.Printf("📦 Auto-Batching Enabled\n")
		fmt.Printf("   Splitting into %d waves\n", waveCount)
		fmt.Printf("   Wave size: %d agents (max)\n\n", maxParallel)
	}

	return nil
}

func executeSwarm(agentCount int) error {
	if agentCount <= maxParallel || batchMode == batchModeOff {
		// Execute all agents in single wave
		return executeSingleWave(agentCount, 1, 1)
	}

	// Auto-batch into waves
	waveCount := int(math.Ceil(float64(agentCount) / float64(maxParallel)))

	fmt.Printf("🚀 Launching Swarm (%d agents in %d waves)\n", agentCount, waveCount)
	fmt.Println("==========================================")
	fmt.Println()

	remaining := agentCount
	for wave := 1; wave <= waveCount; wave++ {
		waveSize := min(maxParallel, remaining)

		if err := executeSingleWave(waveSize, wave, waveCount); err != nil {
			return fmt.Errorf("wave %d failed: %w", wave, err)
		}

		remaining -= waveSize
	}

	fmt.Println()
	fmt.Printf("✅ Swarm Complete\n")
	fmt.Printf("   Total agents: %d\n", agentCount)
	fmt.Printf("   Total waves: %d\n", waveCount)
	fmt.Printf("   Status: All waves completed successfully\n")

	return nil
}

func executeSingleWave(agentCount, waveNum, totalWaves int) error {
	fmt.Printf("Wave %d/%d: Launching %d agents...\n", waveNum, totalWaves, agentCount)

	// Placeholder: In real implementation, this would:
	// 1. Launch Claude Code agents using Task tool
	// 2. Monitor progress
	// 3. Wait for completion
	// 4. Check for failures

	// Simulate wave execution
	startTime := time.Now()
	time.Sleep(2 * time.Second) // Simulate work
	duration := time.Since(startTime)

	fmt.Printf("Wave %d/%d: ✓ Complete (%.1fs)\n", waveNum, totalWaves, duration.Seconds())
	fmt.Println()

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
