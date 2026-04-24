package analytics

import "math"

// PhaseROIMetrics contains Wayfinder phase ROI calculations.
//
// Quality score formula (S6 design / D4 requirements):
//
//	quality_score = 1.0 - (rework_count * 0.2) - (error_count * 0.1)
//	Clamped to [0.0, 1.0]
//
// Interpretation:
//   - 1.0: Perfect execution (no rework, no errors)
//   - 0.8: 1 rework event
//   - 0.9: 1 error event
//   - 0.0: ≥5 rework events or ≥10 errors
type PhaseROIMetrics struct {
	ErrorCount   int     `json:"error_count"`   // Errors during phase
	ReworkCount  int     `json:"rework_count"`  // Rework iterations (phase restart)
	QualityScore float64 `json:"quality_score"` // Computed quality metric
}

// ComputeQualityScore calculates quality score from rework and error counts.
//
// Formula: 1.0 - (rework * 0.2) - (error * 0.1)
// Clamped to [0.0, 1.0]
//
// Examples:
//   - 0 rework, 0 errors → 1.0
//   - 1 rework, 0 errors → 0.8
//   - 0 rework, 1 error  → 0.9
//   - 2 rework, 3 errors → 0.3
//   - 5 rework, 0 errors → 0.0
func ComputeQualityScore(reworkCount, errorCount int) float64 {
	score := 1.0 - (float64(reworkCount) * 0.2) - (float64(errorCount) * 0.1)

	// Clamp to [0.0, 1.0]
	return math.Max(0.0, math.Min(1.0, score))
}

// CalculatePhaseMetrics computes ROI metrics for a completed phase.
//
// Parameters:
//   - reworkCount: Number of times phase was restarted/rewound
//   - errorCount: Number of errors encountered during phase
//
// Returns:
//   - PhaseROIMetrics with calculated quality score
func CalculatePhaseMetrics(reworkCount, errorCount int) PhaseROIMetrics {
	return PhaseROIMetrics{
		ErrorCount:   errorCount,
		ReworkCount:  reworkCount,
		QualityScore: ComputeQualityScore(reworkCount, errorCount),
	}
}

// SessionROIMetrics contains aggregated ROI metrics for an entire session.
type SessionROIMetrics struct {
	TotalErrors    int      `json:"total_errors"`    // Sum of errors across all phases
	TotalRework    int      `json:"total_rework"`    // Sum of rework across all phases
	ReworkPhases   []string `json:"rework_phases"`   // Phases that required rework
	AverageQuality float64  `json:"average_quality"` // Mean quality across phases
	OverallQuality float64  `json:"overall_quality"` // Quality using total rework/errors
}

// AggregateSessionMetrics computes session-level ROI metrics from phase metrics.
//
// Parameters:
//   - phaseMetrics: Map of phase name → PhaseROIMetrics
//
// Returns:
//   - SessionROIMetrics with aggregated statistics
func AggregateSessionMetrics(phaseMetrics map[string]PhaseROIMetrics) SessionROIMetrics {
	if len(phaseMetrics) == 0 {
		return SessionROIMetrics{}
	}

	totalErrors := 0
	totalRework := 0
	qualitySum := 0.0
	reworkPhases := []string{}

	for phaseName, metrics := range phaseMetrics {
		totalErrors += metrics.ErrorCount
		totalRework += metrics.ReworkCount
		qualitySum += metrics.QualityScore

		// Track phases with rework
		if metrics.ReworkCount > 0 {
			reworkPhases = append(reworkPhases, phaseName)
		}
	}

	// Calculate average quality across phases
	averageQuality := qualitySum / float64(len(phaseMetrics))

	// Calculate overall quality using total rework/errors
	overallQuality := ComputeQualityScore(totalRework, totalErrors)

	return SessionROIMetrics{
		TotalErrors:    totalErrors,
		TotalRework:    totalRework,
		ReworkPhases:   reworkPhases,
		AverageQuality: averageQuality,
		OverallQuality: overallQuality,
	}
}
