package metrics

import (
	"fmt"
	"time"
)

// Swarm represents a swarm operation
type Swarm struct {
	Timestamp           time.Time
	AgentCount          int
	BatchSize           int
	WaveCount           int
	StartTime           time.Time
	EndTime             *time.Time
	DurationSeconds     *int
	DeadlockOccurred    bool
	HardwareCPUPercent  *float64
	HardwareRAMPercent  *float64
	HardwareLoadAvg     *float64
	Notes               string
}

// LogSwarmStart records the start of a swarm operation
func (db *DB) LogSwarmStart(swarm *Swarm) (int64, error) {
	query := `
		INSERT INTO swarms (
			timestamp, agent_count, batch_size, wave_count, start_time,
			deadlock_occurred, hardware_cpu_percent, hardware_ram_percent, hardware_load_avg, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	result, err := db.conn.Exec(
		query,
		swarm.Timestamp.Format(time.RFC3339),
		swarm.AgentCount,
		swarm.BatchSize,
		swarm.WaveCount,
		swarm.StartTime.Format(time.RFC3339),
		0, // deadlock_occurred - default false
		swarm.HardwareCPUPercent,
		swarm.HardwareRAMPercent,
		swarm.HardwareLoadAvg,
		swarm.Notes,
	)

	if err != nil {
		return 0, fmt.Errorf("log swarm start: %w", err)
	}

	return result.LastInsertId()
}

// UpdateSwarmEnd updates a swarm operation with completion info
func (db *DB) UpdateSwarmEnd(id int64, endTime time.Time, deadlockOccurred bool) error {
	// Get start time to calculate duration
	var startTimeStr string
	err := db.conn.QueryRow("SELECT start_time FROM swarms WHERE id = ?", id).Scan(&startTimeStr)
	if err != nil {
		return fmt.Errorf("get swarm start time: %w", err)
	}

	startTime, _ := time.Parse(time.RFC3339, startTimeStr)
	duration := int(endTime.Sub(startTime).Seconds())

	query := `
		UPDATE swarms
		SET end_time = ?, duration_seconds = ?, deadlock_occurred = ?
		WHERE id = ?
	`

	_, err = db.conn.Exec(
		query,
		endTime.Format(time.RFC3339),
		duration,
		boolToInt(deadlockOccurred),
		id,
	)

	if err != nil {
		return fmt.Errorf("update swarm end: %w", err)
	}

	return nil
}

// GetSwarms retrieves swarm operations within a time range
func (db *DB) GetSwarms(start, end time.Time) ([]Swarm, error) {
	query := `
		SELECT timestamp, agent_count, batch_size, wave_count, start_time, end_time,
		       duration_seconds, deadlock_occurred, hardware_cpu_percent, hardware_ram_percent,
		       hardware_load_avg, notes
		FROM swarms
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp DESC
	`

	rows, err := db.conn.Query(query, start.Format(time.RFC3339), end.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("query swarms: %w", err)
	}
	defer rows.Close()

	var swarms []Swarm
	for rows.Next() {
		var swarm Swarm
		var timestamp, startTime string
		var endTime *string
		var deadlockInt int

		err := rows.Scan(
			&timestamp,
			&swarm.AgentCount,
			&swarm.BatchSize,
			&swarm.WaveCount,
			&startTime,
			&endTime,
			&swarm.DurationSeconds,
			&deadlockInt,
			&swarm.HardwareCPUPercent,
			&swarm.HardwareRAMPercent,
			&swarm.HardwareLoadAvg,
			&swarm.Notes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan swarm: %w", err)
		}

		swarm.Timestamp, _ = time.Parse(time.RFC3339, timestamp)
		swarm.StartTime, _ = time.Parse(time.RFC3339, startTime)
		if endTime != nil {
			t, _ := time.Parse(time.RFC3339, *endTime)
			swarm.EndTime = &t
		}
		swarm.DeadlockOccurred = deadlockInt == 1

		swarms = append(swarms, swarm)
	}

	return swarms, nil
}

// GetSwarmStats returns aggregate statistics for swarms in a time range
func (db *DB) GetSwarmStats(start, end time.Time) (map[string]interface{}, error) {
	query := `
		SELECT
			COUNT(*) as total_swarms,
			COALESCE(SUM(deadlock_occurred), 0) as deadlocked_swarms,
			COALESCE(AVG(agent_count), 0) as avg_agent_count,
			COALESCE(MAX(agent_count), 0) as max_agent_count,
			AVG(duration_seconds) as avg_duration
		FROM swarms
		WHERE timestamp >= ? AND timestamp <= ?
	`

	var totalSwarms, deadlockedSwarms int
	var avgAgentCount, maxAgentCount float64
	var avgDuration *float64

	err := db.conn.QueryRow(query, start.Format(time.RFC3339), end.Format(time.RFC3339)).Scan(
		&totalSwarms,
		&deadlockedSwarms,
		&avgAgentCount,
		&maxAgentCount,
		&avgDuration,
	)
	if err != nil {
		return nil, fmt.Errorf("query swarm stats: %w", err)
	}

	stats := map[string]interface{}{
		"total_swarms":      totalSwarms,
		"deadlocked_swarms": deadlockedSwarms,
		"avg_agent_count":   avgAgentCount,
		"max_agent_count":   int(maxAgentCount),
	}

	if avgDuration != nil {
		stats["avg_duration_seconds"] = int(*avgDuration)
	}

	return stats, nil
}

// boolToInt converts bool to int for SQLite
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
