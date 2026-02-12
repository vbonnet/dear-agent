package main

import (
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/metrics"
)

var deadlockReportCmd = &cobra.Command{
	Use:   "deadlock-report",
	Short: "Generate deadlock metrics and trends report",
	Long: `Generate a report of deadlock incidents, swarm operations, and system health.

Shows:
- Deadlock incident count and trends over time
- Swarm operation statistics (success rate, avg size)
- System health averages (CPU, RAM, load)
- Correlation analysis (swarm size vs deadlock risk)

Examples:
  # Last 7 days (default)
  agm deadlock-report

  # Last 30 days
  agm deadlock-report --days 30

  # Custom date range
  agm deadlock-report --start 2026-02-01 --end 2026-02-12

  # Detailed incident list
  agm deadlock-report --verbose
`,
	RunE: runDeadlockReport,
}

var (
	reportDays    int
	reportStart   string
	reportEnd     string
	reportVerbose bool
)

func init() {
	deadlockReportCmd.Flags().IntVar(&reportDays, "days", 7, "Number of days to report on")
	deadlockReportCmd.Flags().StringVar(&reportStart, "start", "", "Start date (YYYY-MM-DD)")
	deadlockReportCmd.Flags().StringVar(&reportEnd, "end", "", "End date (YYYY-MM-DD)")
	deadlockReportCmd.Flags().BoolVarP(&reportVerbose, "verbose", "v", false, "Show detailed incident list")

	rootCmd.AddCommand(deadlockReportCmd)
}

func runDeadlockReport(cmd *cobra.Command, args []string) error {
	// Determine time range
	var start, end time.Time
	if reportStart != "" && reportEnd != "" {
		var err error
		start, err = time.Parse("2006-01-02", reportStart)
		if err != nil {
			return fmt.Errorf("invalid start date: %w", err)
		}
		end, err = time.Parse("2006-01-02", reportEnd)
		if err != nil {
			return fmt.Errorf("invalid end date: %w", err)
		}
		end = end.Add(24 * time.Hour) // Include the full end day
	} else {
		end = time.Now()
		start = end.AddDate(0, 0, -reportDays)
	}

	// Open metrics database
	db, err := metrics.Open("")
	if err != nil {
		return fmt.Errorf("open metrics database: %w", err)
	}
	defer db.Close()

	// Print report header
	fmt.Printf("Deadlock Metrics Report\n")
	fmt.Printf("=======================\n\n")
	fmt.Printf("Time Range: %s to %s (%d days)\n\n",
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
		int(end.Sub(start).Hours()/24))

	// Get deadlock incidents
	incidents, err := db.GetIncidents(start, end)
	if err != nil {
		return fmt.Errorf("get incidents: %w", err)
	}

	fmt.Printf("📊 Deadlock Incidents\n")
	fmt.Printf("   Total incidents: %d\n", len(incidents))

	if len(incidents) > 0 {
		// Analyze recovery methods
		recoveryMethods := make(map[string]int)
		for _, inc := range incidents {
			recoveryMethods[inc.RecoveryMethod]++
		}

		fmt.Printf("   Recovery methods:\n")
		for method, count := range recoveryMethods {
			fmt.Printf("      - %s: %d (%.1f%%)\n", method, count, float64(count)/float64(len(incidents))*100)
		}

		// Average recovery time
		totalRecovery := 0
		for _, inc := range incidents {
			totalRecovery += inc.TimeToRecoverySeconds
		}
		avgRecovery := totalRecovery / len(incidents)
		fmt.Printf("   Avg recovery time: %ds\n", avgRecovery)

		fmt.Println()
	}

	// Get swarm statistics
	swarmStats, err := db.GetSwarmStats(start, end)
	if err != nil {
		return fmt.Errorf("get swarm stats: %w", err)
	}

	fmt.Printf("🚀 Swarm Operations\n")
	totalSwarms := swarmStats["total_swarms"].(int)
	deadlockedSwarms := swarmStats["deadlocked_swarms"].(int)
	fmt.Printf("   Total swarms: %d\n", totalSwarms)

	if totalSwarms > 0 {
		fmt.Printf("   Deadlocked swarms: %d (%.1f%%)\n",
			deadlockedSwarms,
			float64(deadlockedSwarms)/float64(totalSwarms)*100)
		fmt.Printf("   Avg agent count: %.1f\n", swarmStats["avg_agent_count"])
		fmt.Printf("   Max agent count: %d\n", swarmStats["max_agent_count"])
		if avgDuration, ok := swarmStats["avg_duration_seconds"]; ok {
			fmt.Printf("   Avg duration: %ds\n", avgDuration)
		}
		fmt.Println()
	} else {
		fmt.Printf("   No swarm operations recorded\n\n")
	}

	// Get system health averages
	healthStats, err := db.GetHealthAverage(start, end)
	if err != nil {
		return fmt.Errorf("get health stats: %w", err)
	}

	fmt.Printf("💻 System Health (Averages)\n")
	fmt.Printf("   Active sessions: %d\n", healthStats["avg_active_sessions"])
	fmt.Printf("   Deadlocked processes: %d\n", healthStats["avg_deadlocked_processes"])
	fmt.Printf("   CPU usage: %.1f%%\n", healthStats["avg_cpu_percent"])
	fmt.Printf("   RAM usage: %.1f%%\n", healthStats["avg_ram_percent"])
	if avgLoad, ok := healthStats["avg_load_avg"]; ok {
		fmt.Printf("   Load average: %.2f\n", avgLoad)
	}
	fmt.Println()

	// Verbose: Show detailed incident list
	if reportVerbose && len(incidents) > 0 {
		fmt.Printf("📋 Detailed Incident List\n\n")

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "Timestamp\tSession\tPID\tState\tCPU%\tRuntime\tRecovery\tTime")
		fmt.Fprintln(w, "---------\t-------\t---\t-----\t----\t-------\t--------\t----")

		for _, inc := range incidents {
			fmt.Fprintf(w, "%s\t%s\t%d\t%s\t%.1f%%\t%ds\t%s\t%ds\n",
				inc.Timestamp.Format("2006-01-02 15:04"),
				inc.SessionName,
				inc.PID,
				inc.ProcessState,
				inc.CPUPercent,
				inc.RuntimeSeconds,
				inc.RecoveryMethod,
				inc.TimeToRecoverySeconds,
			)
		}

		w.Flush()
		fmt.Println()
	}

	// Analysis and recommendations
	fmt.Printf("📈 Trends & Recommendations\n")

	if len(incidents) == 0 {
		fmt.Printf("   ✅ No deadlocks in the past %d days - excellent!\n", reportDays)
	} else {
		incidentsPerDay := float64(len(incidents)) / float64(reportDays)
		fmt.Printf("   Incident rate: %.2f deadlocks/day\n", incidentsPerDay)

		if incidentsPerDay > 1.0 {
			fmt.Printf("   ⚠️  High incident rate - consider:\n")
			fmt.Printf("      - Reducing swarm batch sizes\n")
			fmt.Printf("      - Enabling auto-batching\n")
			fmt.Printf("      - Checking hardware resources\n")
		} else if incidentsPerDay > 0.5 {
			fmt.Printf("   ⚠️  Moderate incident rate - monitor closely\n")
		} else {
			fmt.Printf("   ✅ Low incident rate - current limits working well\n")
		}
	}

	if totalSwarms > 0 && deadlockedSwarms > 0 {
		swarmFailureRate := float64(deadlockedSwarms) / float64(totalSwarms)
		if swarmFailureRate > 0.1 {
			fmt.Printf("   ⚠️  Swarm failure rate %.1f%% - consider reducing batch size\n", swarmFailureRate*100)
		}
	}

	return nil
}
