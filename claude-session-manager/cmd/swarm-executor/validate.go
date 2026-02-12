package main

import (
	"fmt"
	"math"

	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate swarm size without executing",
	Long: `Validate swarm size against safe limits without executing.

This command performs pre-flight validation and reports:
- Whether swarm is within safe limits
- Deadlock risk assessment
- Recommended batching strategy
- Wave breakdown if batching needed

Use this to check swarm size before execution.
`,
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().StringVar(&queuePath, "queue", "", "Path to task queue YAML (required)")
	validateCmd.MarkFlagRequired("queue")

	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	fmt.Println("Swarm Size Validation Report")
	fmt.Println("============================")
	fmt.Println()

	// Get agent count
	agentCount := getAgentCount(queuePath)

	// Validation results
	fmt.Printf("📊 Swarm Analysis\n")
	fmt.Printf("   Queue file: %s\n", queuePath)
	fmt.Printf("   Agent count: %d\n", agentCount)
	fmt.Printf("   Safe limit: %d agents\n", maxParallel)
	fmt.Println()

	// Risk assessment
	riskLevel := assessRisk(agentCount, maxParallel)
	fmt.Printf("🔍 Risk Assessment\n")
	fmt.Printf("   Risk level: %s\n", riskLevel)

	if agentCount <= maxParallel {
		fmt.Printf("   Status: ✅ SAFE - Within recommended limits\n")
		fmt.Printf("   Action: Launch normally (no batching needed)\n")
		fmt.Println()
		return nil
	}

	// Exceeds limit - calculate batching
	exceeds := agentCount - maxParallel
	exceedsPercent := float64(exceeds) / float64(maxParallel) * 100
	waveCount := int(math.Ceil(float64(agentCount) / float64(maxParallel)))

	fmt.Printf("   Status: ⚠️  EXCEEDS LIMIT by %d agents (%.0f%%)\n", exceeds, exceedsPercent)
	fmt.Printf("   Recommendation: Auto-batch into waves\n")
	fmt.Println()

	// Wave breakdown
	fmt.Printf("📦 Recommended Batching Strategy\n")
	fmt.Printf("   Total waves: %d\n", waveCount)
	fmt.Printf("   Wave size: %d agents (max)\n", maxParallel)
	fmt.Println()

	remaining := agentCount
	for wave := 1; wave <= waveCount; wave++ {
		waveSize := min(maxParallel, remaining)
		fmt.Printf("   Wave %d: %d agents\n", wave, waveSize)
		remaining -= waveSize
	}

	fmt.Println()
	fmt.Printf("⏱️  Estimated Impact\n")

	// Time estimation (assuming 10 min per wave avg)
	avgWaveTime := 10.0
	unbatchedTime := 10.0
	batchedTime := avgWaveTime * float64(waveCount)
	overhead := batchedTime - unbatchedTime

	fmt.Printf("   Unbatched: ~%.0f minutes (high deadlock risk)\n", unbatchedTime)
	fmt.Printf("   Batched: ~%.0f minutes (%.0f min overhead, safe)\n", batchedTime, overhead)
	fmt.Println()

	// Execution command
	fmt.Printf("💡 Next Steps\n")
	fmt.Printf("   To execute with auto-batching:\n")
	fmt.Printf("     swarm-executor execute --queue %s\n", queuePath)
	fmt.Println()

	return nil
}

func assessRisk(agentCount, maxParallel int) string {
	if agentCount <= 10 {
		return "VERY LOW (0-10 agents)"
	} else if agentCount <= maxParallel {
		return "LOW (within safe limit)"
	} else if agentCount <= 20 {
		return "MODERATE (16-20 agents)"
	} else if agentCount <= 30 {
		return "HIGH (21-30 agents)"
	} else {
		return "VERY HIGH (30+ agents, known failure zone)"
	}
}
