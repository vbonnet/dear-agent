package metrics

import (
	"fmt"
	"time"
)

// Incident represents a deadlock incident
type Incident struct {
	Timestamp           time.Time
	SessionName         string
	PID                 int
	ProcessState        string
	CPUPercent          float64
	RuntimeSeconds      int
	WCHAN               string
	RecoveryMethod      string
	TimeToRecoverySeconds int
	SwarmSize           *int
	HardwareCPUPercent  *float64
	HardwareRAMPercent  *float64
	HardwareLoadAvg     *float64
	Notes               string
}

// LogIncident records a deadlock incident to the database
func (db *DB) LogIncident(incident *Incident) error {
	query := `
		INSERT INTO incidents (
			timestamp, session_name, pid, process_state, cpu_percent,
			runtime_seconds, wchan, recovery_method, time_to_recovery_seconds,
			swarm_size, hardware_cpu_percent, hardware_ram_percent, hardware_load_avg, notes
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := db.conn.Exec(
		query,
		incident.Timestamp.Format(time.RFC3339),
		incident.SessionName,
		incident.PID,
		incident.ProcessState,
		incident.CPUPercent,
		incident.RuntimeSeconds,
		incident.WCHAN,
		incident.RecoveryMethod,
		incident.TimeToRecoverySeconds,
		incident.SwarmSize,
		incident.HardwareCPUPercent,
		incident.HardwareRAMPercent,
		incident.HardwareLoadAvg,
		incident.Notes,
	)

	if err != nil {
		return fmt.Errorf("log incident: %w", err)
	}

	return nil
}

// GetIncidents retrieves incidents within a time range
func (db *DB) GetIncidents(start, end time.Time) ([]Incident, error) {
	query := `
		SELECT timestamp, session_name, pid, process_state, cpu_percent,
		       runtime_seconds, wchan, recovery_method, time_to_recovery_seconds,
		       swarm_size, hardware_cpu_percent, hardware_ram_percent, hardware_load_avg, notes
		FROM incidents
		WHERE timestamp >= ? AND timestamp <= ?
		ORDER BY timestamp DESC
	`

	rows, err := db.conn.Query(query, start.Format(time.RFC3339), end.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("query incidents: %w", err)
	}
	defer rows.Close()

	var incidents []Incident
	for rows.Next() {
		var incident Incident
		var timestamp string

		err := rows.Scan(
			&timestamp,
			&incident.SessionName,
			&incident.PID,
			&incident.ProcessState,
			&incident.CPUPercent,
			&incident.RuntimeSeconds,
			&incident.WCHAN,
			&incident.RecoveryMethod,
			&incident.TimeToRecoverySeconds,
			&incident.SwarmSize,
			&incident.HardwareCPUPercent,
			&incident.HardwareRAMPercent,
			&incident.HardwareLoadAvg,
			&incident.Notes,
		)
		if err != nil {
			return nil, fmt.Errorf("scan incident: %w", err)
		}

		incident.Timestamp, _ = time.Parse(time.RFC3339, timestamp)
		incidents = append(incidents, incident)
	}

	return incidents, nil
}

// GetIncidentCount returns the count of incidents in a time range
func (db *DB) GetIncidentCount(start, end time.Time) (int, error) {
	query := `SELECT COUNT(*) FROM incidents WHERE timestamp >= ? AND timestamp <= ?`

	var count int
	err := db.conn.QueryRow(query, start.Format(time.RFC3339), end.Format(time.RFC3339)).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count incidents: %w", err)
	}

	return count, nil
}
