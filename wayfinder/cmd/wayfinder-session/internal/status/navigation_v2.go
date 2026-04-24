package status

import "fmt"

// NextWaypoint returns the next waypoint in the V2 sequence
// Returns error if:
// - Already at final waypoint (RETRO)
// - Current waypoint is invalid
// Returns current waypoint if it's not completed yet
func (s *StatusV2) NextWaypoint() (string, error) {
	allWaypoints := AllWaypointsV2Schema()

	// Handle empty current waypoint - start at CHARTER
	if s.CurrentWaypoint == "" {
		return WaypointV2Charter, nil
	}

	// Find current waypoint index
	currentIdx := -1
	for i, waypoint := range allWaypoints {
		if waypoint == s.CurrentWaypoint {
			currentIdx = i
			break
		}
	}

	if currentIdx == -1 {
		return "", fmt.Errorf("invalid current waypoint: %s", s.CurrentWaypoint)
	}

	// Check if at final waypoint
	if currentIdx == len(allWaypoints)-1 {
		return "", fmt.Errorf("already at final waypoint RETRO")
	}

	// Check if current waypoint is completed
	if !s.isWaypointCompleted(s.CurrentWaypoint) {
		return s.CurrentWaypoint, nil
	}

	// Return next waypoint
	return allWaypoints[currentIdx+1], nil
}

// isWaypointCompleted checks if a waypoint is marked as completed or skipped
func (s *StatusV2) isWaypointCompleted(waypointName string) bool {
	for _, w := range s.WaypointHistory {
		if w.Name == waypointName {
			return w.Status == WaypointStatusV2Completed || w.Status == WaypointStatusV2Skipped
		}
	}
	return false
}

// GetWaypointHistory returns the waypoint history entry for a given waypoint name
func (s *StatusV2) GetWaypointHistory(waypointName string) *WaypointHistory {
	for i := range s.WaypointHistory {
		if s.WaypointHistory[i].Name == waypointName {
			return &s.WaypointHistory[i]
		}
	}
	return nil
}

// AddWaypointHistory adds or updates a waypoint in the history
func (s *StatusV2) AddWaypointHistory(waypoint WaypointHistory) {
	// Check if waypoint already exists
	for i := range s.WaypointHistory {
		if s.WaypointHistory[i].Name == waypoint.Name {
			s.WaypointHistory[i] = waypoint
			return
		}
	}
	// Add new waypoint
	s.WaypointHistory = append(s.WaypointHistory, waypoint)
}

// GetRoadmapWaypoint returns the roadmap waypoint entry for a given waypoint ID
func (s *StatusV2) GetRoadmapWaypoint(waypointID string) *RoadmapPhase {
	if s.Roadmap == nil {
		return nil
	}
	for i := range s.Roadmap.Phases {
		if s.Roadmap.Phases[i].ID == waypointID {
			return &s.Roadmap.Phases[i]
		}
	}
	return nil
}

// UpdateRoadmapWaypointStatus updates the status of a roadmap waypoint
func (s *StatusV2) UpdateRoadmapWaypointStatus(waypointID, status string) {
	if s.Roadmap == nil {
		return
	}
	for i := range s.Roadmap.Phases {
		if s.Roadmap.Phases[i].ID == waypointID {
			s.Roadmap.Phases[i].Status = status
			return
		}
	}
}

// ============================================================================
// Backward Compatibility Aliases
// ============================================================================

// NextPhase is a backward-compatibility alias for NextWaypoint
func (s *StatusV2) NextPhase() (string, error) {
	return s.NextWaypoint()
}

// GetPhaseHistory is a backward-compatibility alias for GetWaypointHistory
func (s *StatusV2) GetPhaseHistory(waypointName string) *WaypointHistory {
	return s.GetWaypointHistory(waypointName)
}

// AddPhaseHistory is a backward-compatibility alias for AddWaypointHistory
func (s *StatusV2) AddPhaseHistory(waypoint WaypointHistory) {
	s.AddWaypointHistory(waypoint)
}

// GetRoadmapPhase is a backward-compatibility alias for GetRoadmapWaypoint
func (s *StatusV2) GetRoadmapPhase(waypointID string) *RoadmapPhase {
	return s.GetRoadmapWaypoint(waypointID)
}

// UpdateRoadmapPhaseStatus is a backward-compatibility alias for UpdateRoadmapWaypointStatus
func (s *StatusV2) UpdateRoadmapPhaseStatus(waypointID, status string) {
	s.UpdateRoadmapWaypointStatus(waypointID, status)
}
