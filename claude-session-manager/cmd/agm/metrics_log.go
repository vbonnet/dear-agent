package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/metrics"
)

var metricsLogCmd = &cobra.Command{
	Use:   "metrics-log",
	Short: "Log metrics to deadlock database",
	Long:  `Log deadlock incidents, swarm operations, or health snapshots to metrics database.`,
}

var logIncidentCmd = &cobra.Command{
	Use:   "incident [json]",
	Short: "Log a deadlock incident",
	Long: `Log a deadlock incident from JSON input.

JSON format:
{
  "session_name": "my-session",
  "pid": 12345,
  "process_state": "RNl+",
  "cpu_percent": 87.5,
  "runtime_seconds": 600,
  "wchan": "ep_poll",
  "recovery_method": "ESC",
  "time_to_recovery_seconds": 5,
  "swarm_size": 38,
  "hardware_cpu_percent": 92.0,
  "hardware_ram_percent": 65.0,
  "hardware_load_avg": 3.5,
  "notes": "Additional context"
}

Example:
  echo '{...}' | agm metrics-log incident
  agm metrics-log incident '{"session_name":"test",...}'
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogIncident,
}

var logSwarmCmd = &cobra.Command{
	Use:   "swarm [json]",
	Short: "Log a swarm operation",
	Long: `Log a swarm operation from JSON input.

JSON format:
{
  "agent_count": 35,
  "batch_size": 15,
  "wave_count": 3,
  "deadlock_occurred": false,
  "hardware_cpu_percent": 45.0,
  "hardware_ram_percent": 55.0,
  "hardware_load_avg": 1.5,
  "notes": "Additional context"
}

Example:
  echo '{...}' | agm metrics-log swarm
  agm metrics-log swarm '{"agent_count":35,...}'
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogSwarm,
}

var logHealthCmd = &cobra.Command{
	Use:   "health [json]",
	Short: "Log a health snapshot",
	Long: `Log a system health snapshot from JSON input.

JSON format:
{
  "active_sessions": 3,
  "deadlocked_processes": 0,
  "cpu_percent": 45.0,
  "ram_percent": 55.0,
  "disk_io_percent": 20.0,
  "load_avg_1min": 1.5,
  "load_avg_5min": 1.2,
  "load_avg_15min": 1.0,
  "api_rate_limit_remaining": 1000,
  "notes": "Additional context"
}

Example:
  echo '{...}' | agm metrics-log health
  agm metrics-log health '{"active_sessions":3,...}'
`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLogHealth,
}

func init() {
	metricsLogCmd.AddCommand(logIncidentCmd)
	metricsLogCmd.AddCommand(logSwarmCmd)
	metricsLogCmd.AddCommand(logHealthCmd)

	rootCmd.AddCommand(metricsLogCmd)
}

func readJSONInput(args []string) ([]byte, error) {
	var input []byte
	var err error

	if len(args) > 0 {
		// JSON provided as argument
		input = []byte(args[0])
	} else {
		// Read from stdin
		input, err = io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
	}

	return input, nil
}

func runLogIncident(cmd *cobra.Command, args []string) error {
	input, err := readJSONInput(args)
	if err != nil {
		return err
	}

	// Parse JSON
	var data map[string]interface{}
	if err := json.Unmarshal(input, &data); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	// Create incident
	incident := &metrics.Incident{
		Timestamp:             time.Now(),
		SessionName:           getString(data, "session_name"),
		PID:                   getInt(data, "pid"),
		ProcessState:          getString(data, "process_state"),
		CPUPercent:            getFloat(data, "cpu_percent"),
		RuntimeSeconds:        getInt(data, "runtime_seconds"),
		WCHAN:                 getString(data, "wchan"),
		RecoveryMethod:        getString(data, "recovery_method"),
		TimeToRecoverySeconds: getInt(data, "time_to_recovery_seconds"),
		Notes:                 getString(data, "notes"),
	}

	// Optional fields
	if v, ok := data["swarm_size"]; ok && v != nil {
		swarmSize := int(v.(float64))
		incident.SwarmSize = &swarmSize
	}
	if v, ok := data["hardware_cpu_percent"]; ok && v != nil {
		cpu := v.(float64)
		incident.HardwareCPUPercent = &cpu
	}
	if v, ok := data["hardware_ram_percent"]; ok && v != nil {
		ram := v.(float64)
		incident.HardwareRAMPercent = &ram
	}
	if v, ok := data["hardware_load_avg"]; ok && v != nil {
		load := v.(float64)
		incident.HardwareLoadAvg = &load
	}

	// Open database and log
	db, err := metrics.Open("")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.LogIncident(incident); err != nil {
		return fmt.Errorf("log incident: %w", err)
	}

	fmt.Printf("✓ Incident logged: %s (PID %d, %s, %.1f%% CPU)\n",
		incident.SessionName,
		incident.PID,
		incident.RecoveryMethod,
		incident.CPUPercent)

	return nil
}

func runLogSwarm(cmd *cobra.Command, args []string) error {
	input, err := readJSONInput(args)
	if err != nil {
		return err
	}

	// Parse JSON
	var data map[string]interface{}
	if err := json.Unmarshal(input, &data); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	// Create swarm
	now := time.Now()
	swarm := &metrics.Swarm{
		Timestamp:        now,
		AgentCount:       getInt(data, "agent_count"),
		BatchSize:        getInt(data, "batch_size"),
		WaveCount:        getInt(data, "wave_count"),
		StartTime:        now,
		DeadlockOccurred: getBool(data, "deadlock_occurred"),
		Notes:            getString(data, "notes"),
	}

	// Optional fields
	if v, ok := data["hardware_cpu_percent"]; ok && v != nil {
		cpu := v.(float64)
		swarm.HardwareCPUPercent = &cpu
	}
	if v, ok := data["hardware_ram_percent"]; ok && v != nil {
		ram := v.(float64)
		swarm.HardwareRAMPercent = &ram
	}
	if v, ok := data["hardware_load_avg"]; ok && v != nil {
		load := v.(float64)
		swarm.HardwareLoadAvg = &load
	}

	// Open database and log
	db, err := metrics.Open("")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	id, err := db.LogSwarmStart(swarm)
	if err != nil {
		return fmt.Errorf("log swarm: %w", err)
	}

	fmt.Printf("✓ Swarm logged: %d agents, %d waves (ID: %d)\n",
		swarm.AgentCount,
		swarm.WaveCount,
		id)

	return nil
}

func runLogHealth(cmd *cobra.Command, args []string) error {
	input, err := readJSONInput(args)
	if err != nil {
		return err
	}

	// Parse JSON
	var data map[string]interface{}
	if err := json.Unmarshal(input, &data); err != nil {
		return fmt.Errorf("parse JSON: %w", err)
	}

	// Create health snapshot
	snapshot := &metrics.HealthSnapshot{
		Timestamp:           time.Now(),
		ActiveSessions:      getInt(data, "active_sessions"),
		DeadlockedProcesses: getInt(data, "deadlocked_processes"),
		CPUPercent:          getFloat(data, "cpu_percent"),
		RAMPercent:          getFloat(data, "ram_percent"),
		Notes:               getString(data, "notes"),
	}

	// Optional fields
	if v, ok := data["disk_io_percent"]; ok && v != nil {
		disk := v.(float64)
		snapshot.DiskIOPercent = &disk
	}
	if v, ok := data["load_avg_1min"]; ok && v != nil {
		load := v.(float64)
		snapshot.LoadAvg1Min = &load
	}
	if v, ok := data["load_avg_5min"]; ok && v != nil {
		load := v.(float64)
		snapshot.LoadAvg5Min = &load
	}
	if v, ok := data["load_avg_15min"]; ok && v != nil {
		load := v.(float64)
		snapshot.LoadAvg15Min = &load
	}
	if v, ok := data["api_rate_limit_remaining"]; ok && v != nil {
		limit := int(v.(float64))
		snapshot.APIRateLimitRemaining = &limit
	}

	// Open database and log
	db, err := metrics.Open("")
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	if err := db.LogHealthSnapshot(snapshot); err != nil {
		return fmt.Errorf("log health: %w", err)
	}

	fmt.Printf("✓ Health snapshot logged: %d sessions, %d deadlocked, %.1f%% CPU, %.1f%% RAM\n",
		snapshot.ActiveSessions,
		snapshot.DeadlockedProcesses,
		snapshot.CPUPercent,
		snapshot.RAMPercent)

	return nil
}

// Helper functions to extract values from JSON map
func getString(data map[string]interface{}, key string) string {
	if v, ok := data[key]; ok && v != nil {
		return v.(string)
	}
	return ""
}

func getInt(data map[string]interface{}, key string) int {
	if v, ok := data[key]; ok && v != nil {
		return int(v.(float64))
	}
	return 0
}

func getFloat(data map[string]interface{}, key string) float64 {
	if v, ok := data[key]; ok && v != nil {
		return v.(float64)
	}
	return 0.0
}

func getBool(data map[string]interface{}, key string) bool {
	if v, ok := data[key]; ok && v != nil {
		return v.(bool)
	}
	return false
}
