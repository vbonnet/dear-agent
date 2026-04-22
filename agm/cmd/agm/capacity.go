package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/vbonnet/dear-agent/agm/internal/capacity"
)

var capacityCmd = &cobra.Command{
	Use:   "capacity",
	Short: "Show system capacity for Claude sessions",
	Long: `Display current system resource usage, recommended maximum sessions,
and capacity zone (GREEN/YELLOW/RED/CRITICAL).

The calculation considers:
  - Available RAM (minus 4GB OS reserve, ~800MB per session)
  - CPU cores (max 2 sessions per core)
  - Hard cap of 20 sessions

Override with AGM_MAX_SESSIONS environment variable.

Capacity Zones:
  GREEN    (<60% RAM)  Launch freely
  YELLOW   (60-80%)    Be cautious
  RED      (>80%)      Stop launching
  CRITICAL (>90%)      Alert human`,
	RunE: runCapacity,
}

func init() {
	rootCmd.AddCommand(capacityCmd)
}

func runCapacity(cmd *cobra.Command, args []string) error {
	detector := capacity.NewDetector()
	info, err := detector.Detect()
	if err != nil {
		return fmt.Errorf("detecting system resources: %w", err)
	}

	calc := capacity.NewCalculator()
	result := calc.Calculate(info)

	// Format output
	fmt.Println("System Capacity Report")
	fmt.Println("══════════════════════")
	fmt.Println()

	// Hardware
	fmt.Println("Hardware:")
	fmt.Printf("  RAM:  %.1f GB total, %.1f GB available, %.1f GB used (%.1f%%)\n",
		info.TotalRAMGB(), info.AvailableRAMGB(),
		float64(info.UsedRAMBytes())/(1024*1024*1024), info.RAMUsagePercent())
	fmt.Printf("  CPUs: %d cores\n", info.NumCPUs)
	fmt.Println()

	// Sessions
	fmt.Println("Sessions:")
	fmt.Printf("  Running:   %d Claude processes\n", info.ClaudeProcesses)
	fmt.Printf("  Max:       %d (RAM-based: %d, CPU-based: %d, hard cap: %d)\n",
		result.MaxSessions, result.RAMBasedMax, result.CPUBasedMax, result.HardCap)
	fmt.Printf("  Available: %d slots\n", result.AvailableSlots)
	if result.EnvOverride > 0 {
		fmt.Printf("  Override:  AGM_MAX_SESSIONS=%d\n", result.EnvOverride)
	}
	fmt.Println()

	// Zone
	zoneIndicator := zoneColor(result.Zone)
	fmt.Printf("Zone: %s — %s\n", zoneIndicator, capacity.ZoneDescription(result.Zone))

	return nil
}

func zoneColor(z capacity.Zone) string {
	switch z {
	case capacity.ZoneGreen:
		return "\033[32m● GREEN\033[0m"
	case capacity.ZoneYellow:
		return "\033[33m● YELLOW\033[0m"
	case capacity.ZoneRed:
		return "\033[31m● RED\033[0m"
	case capacity.ZoneCritical:
		return "\033[31;1m⚠ CRITICAL\033[0m"
	default:
		return string(z)
	}
}
