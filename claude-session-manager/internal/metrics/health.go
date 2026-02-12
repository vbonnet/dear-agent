package metrics

import (
	"fmt"
	"time"
)

// HealthSnapshot represents a system health snapshot
type HealthSnapshot struct {
	Timestamp              time.Time
	ActiveSessions         int
	DeadlockedProcesses    int
	CPUPercent             float64
	RAMPercent             float64
	DiskIOPercent          *float64
	LoadAvg1Min            *float64
	LoadAvg5Min            *float64
	LoadAvg15Min           *float64
	APIRateLimitRemaining  *int
	Notes                  string
}

// LogHealthSnapshot records a system health snapshot
func (db *DB) LogHealthSnapshot(snapshot *HealthSnapshot) error {
	query := `
		INSERT INTO health_snapshots (
			timestamp, active_sessions, deadlocked_processes, cpu_percent, ram_percent,
			disk_io_percent, load_avg_1min, load_avg_5min, load_avg_15min,
			api_rate_limit_remaining, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.conn.Exec(
		query,
		snapshot.Timestamp.Format(time.RFC3339),
		snapshot.ActiveSessions,
		snapshot.DeadlockedProcesses,
		snapshot.CPUPercent,
		snapshot.RAMPercent,
		snapshot.DiskIOPercent,
		snapshot.LoadAvg1Min,
		snapshot.LoadAvg5Min,
		snapshot.LoadAvg15Min,
		snapshot.APIRateLimitRemaining,
		snapshot.Notes,
	)

	if err != nil {
		return fmt.Errorf("log health snapshot: %w", err)
	}

	return nil
}

// GetHealthSnapshots retrieves health snapshots within a time range
func (db *DB) GetHealthSnapshots(start, end time.Time) ([]HealthSnapshot, error) {
	query := `
		SELECT timestamp, active_sessions, deadlocked_processes, cpu_percent, ram_percent,
		       disk_io_percent, load_avg_1min, load_avg_5min, load_avg_15min,
		       api_rate_limit_remaining, notes
		FROM health_snapshots
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp DESC
	`

	rows, err := db.conn.Query(query, start.Format(time.RFC3339), end.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("query health snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []HealthSnapshot
	for rows.Next() {
		var snapshot HealthSnapshot
		var timestamp string

		err := rows.Scan(
			&timestamp,
			&snapshot.ActiveSessions,
			&snapshot.DeadlockedProcesses,
			&snapshot.CPUPercent,
			&snapshot.RAMPercent,
			&snapshot.DiskIOPercent,
			&snapshot.LoadAvg1Min,
			&snapshot.LoadAvg5Min,
			&snapshot.LoadAvg15Min,
			&snapshot.APIRateLimitRemaining,
			&snapshot.Notes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan health snapshot: %w", err)
		}

		snapshot.Timestamp, _ = time.Parse(time.RFC3339, timestamp)
		snapshots = append(snapshots, snapshot)
	}

	return snapshots, nil
}

// GetHealthAverage returns average health metrics for a time range
func (db *DB) GetHealthAverage(start, end time.Time) (map[string]interface{}, error) {
	query := `
		SELECT
			COALESCE(AVG(active_sessions), 0) as avg_active_sessions,
			COALESCE(AVG(deadlocked_processes), 0) as avg_deadlocked_processes,
			COALESCE(AVG(cpu_percent), 0) as avg_cpu_percent,
			COALESCE(AVG(ram_percent), 0) as avg_ram_percent,
			AVG(load_avg_1min) as avg_load_avg
		FROM health_snapshots
		WHERE timestamp >= ? AND timestamp <= ?
	`

	var avgActiveSessions, avgDeadlockedProcesses, avgCPU, avgRAM float64
	var avgLoad *float64

	err := db.conn.QueryRow(query, start.Format(time.RFC3339), end.Format(time.RFC3339)).Scan(
		&avgActiveSessions,
		&avgDeadlockedProcesses,
		&avgCPU,
		&avgRAM,
		&avgLoad,
	)
	if err != nil {
		return nil, fmt.Errorf("query health average: %w", err)
	}

	stats := map[string]interface{}{
		"avg_active_sessions":      int(avgActiveSessions),
		"avg_deadlocked_processes": int(avgDeadlockedProcesses),
		"avg_cpu_percent":          avgCPU,
		"avg_ram_percent":          avgRAM,
	}

	if avgLoad != nil {
		stats["avg_load_avg"] = *avgLoad
	}

	return stats, nil
}
