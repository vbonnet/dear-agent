package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/daemon"
	"github.com/vbonnet/dear-agent/agm/internal/messages"
)

var logger = slog.Default()

// Example demonstrating how to use daemon monitoring features
func main() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		logger.Error("Failed to get home directory", "error", err)
		os.Exit(1)
	}

	// Example 1: Check daemon health status
	fmt.Println("=== Example 1: Health Status Check ===")
	checkHealth(homeDir)

	// Example 2: Monitor metrics from running daemon
	fmt.Println("\n=== Example 2: Metrics Collection ===")
	demonstrateMetrics()

	// Example 3: Custom alert rules
	fmt.Println("\n=== Example 3: Custom Alert Rules ===")
	demonstrateCustomAlerts()
}

func checkHealth(homeDir string) {
	pidFile := filepath.Join(homeDir, ".agm", "daemon.pid")

	// Open message queue
	queue, err := messages.NewMessageQueue()
	if err != nil {
		logger.Warn("Failed to open queue", "error", err)
		return
	}
	defer queue.Close()

	// Get health status
	health, err := daemon.GetHealthStatus(pidFile, queue)
	if err != nil {
		logger.Warn("Failed to get health", "error", err)
		return
	}

	// Display health information
	fmt.Printf("Daemon Running: %v\n", health.Running)
	if health.Running {
		fmt.Printf("PID: %d\n", health.PID)
	}
	fmt.Printf("Health Level: %s\n", health.HealthStatusLevel)

	// Display queue stats
	if health.QueueStats != nil {
		fmt.Printf("Queue Stats:\n")
		for status, count := range health.QueueStats {
			fmt.Printf("  %s: %d\n", status, count)
		}
	}

	// Take action based on health level
	switch health.HealthStatusLevel {
	case "critical":
		fmt.Println("⚠ CRITICAL: Immediate attention required!")
	case "degraded":
		fmt.Println("⚠ WARNING: Performance degraded")
	case "healthy":
		fmt.Println("✓ System healthy")
	}
}

func demonstrateMetrics() {
	// Create a metrics collector
	mc := daemon.NewMetricsCollector()

	// Simulate some delivery attempts
	mc.RecordDeliveryAttempt(true, 100*time.Millisecond)
	mc.RecordDeliveryAttempt(true, 150*time.Millisecond)
	mc.RecordDeliveryAttempt(true, 200*time.Millisecond)
	mc.RecordDeliveryAttempt(false, 0) // Failed delivery

	// Simulate state detections
	mc.RecordStateDetection("DONE")
	mc.RecordStateDetection("DONE")
	mc.RecordStateDetection("WORKING")

	// Update queue depth
	mc.UpdateQueueDepth(25)

	// Record poll cycle
	mc.RecordPoll(500 * time.Millisecond)

	// Get metrics snapshot
	metrics := mc.GetMetrics()

	// Display metrics
	fmt.Printf("Total Deliveries: %d\n", metrics.TotalMessagesDelivered)
	fmt.Printf("Total Failures: %d\n", metrics.TotalMessagesFailed)
	fmt.Printf("Success Rate: %.2f%%\n", metrics.SuccessRate)
	fmt.Printf("Avg Latency: %v\n", metrics.AvgDeliveryLatency)
	fmt.Printf("Queue Depth: %d\n", metrics.CurrentQueueDepth)
	fmt.Printf("Last Poll: %v ago\n", time.Since(metrics.LastPollTime))

	// Display state detection stats
	fmt.Printf("State Detections:\n")
	for state, count := range metrics.StateDetectionAccuracy {
		fmt.Printf("  %s: %d\n", state, count)
	}
}

func demonstrateCustomAlerts() {
	// Create custom alert rules
	customRules := []daemon.AlertRule{
		{
			Name: "Very High Queue",
			Condition: func(m daemon.MetricsSnapshot) *daemon.Alert {
				if m.CurrentQueueDepth > 200 {
					return &daemon.Alert{
						Level:     daemon.AlertLevelCritical,
						Timestamp: time.Now(),
						Message:   "Queue depth extremely high",
						Metric:    "queue_depth",
						Value:     m.CurrentQueueDepth,
						Threshold: 200,
					}
				}
				return nil
			},
		},
		{
			Name: "Long Uptime",
			Condition: func(m daemon.MetricsSnapshot) *daemon.Alert {
				if m.Uptime > 7*24*time.Hour {
					return &daemon.Alert{
						Level:     daemon.AlertLevelInfo,
						Timestamp: time.Now(),
						Message:   "Daemon has been running for over a week",
						Metric:    "uptime",
						Value:     m.Uptime,
						Threshold: 7 * 24 * time.Hour,
					}
				}
				return nil
			},
		},
	}

	// Create metrics with high queue depth
	metrics := daemon.MetricsSnapshot{
		CurrentQueueDepth: 250,
		Uptime:            8 * 24 * time.Hour,
	}

	// Check alerts
	alerts := daemon.CheckAlerts(metrics, customRules)

	// Display triggered alerts
	fmt.Printf("Triggered %d alert(s):\n", len(alerts))
	for _, alert := range alerts {
		fmt.Printf("  [%s] %s\n", alert.Level, alert.Message)
		fmt.Printf("    Metric: %s = %v (threshold: %v)\n",
			alert.Metric, alert.Value, alert.Threshold)
	}
}
