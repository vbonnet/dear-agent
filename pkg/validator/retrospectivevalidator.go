// Package validator provides validation for S11 enhanced retrospective files.
//
// It validates enhanced retrospective files based on the G-Eval scoring schema,
// checking for structural issues, statistical anomalies, and quality patterns.
package validator

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
)

// RetrospectiveValidationError represents a validation error found in a retrospective.
type RetrospectiveValidationError struct {
	Field    string // Field name (e.g., "D1.clarity", "schema_version")
	Message  string // Detailed error message
	Severity string // "error" or "warning"
}

// RetrospectiveValidator validates S11 enhanced retrospective files.
type RetrospectiveValidator struct {
	filePath string
	errors   []RetrospectiveValidationError
	warnings []RetrospectiveValidationError
}

// PhaseScores represents the scores for a single phase.
type PhaseScores struct {
	Clarity      int     `json:"clarity"`
	Completeness int     `json:"completeness"`
	Efficiency   int     `json:"efficiency"`
	Quality      int     `json:"quality"`
	Impact       int     `json:"impact"`
	Overall      float64 `json:"overall"`
}

// PhaseReasoning represents the reasoning for each dimension.
type PhaseReasoning struct {
	Clarity      string `json:"clarity"`
	Completeness string `json:"completeness"`
	Efficiency   string `json:"efficiency"`
	Quality      string `json:"quality"`
	Impact       string `json:"impact"`
}

// PhaseData represents complete phase data.
type PhaseData struct {
	Scores      PhaseScores    `json:"scores"`
	Reasoning   PhaseReasoning `json:"reasoning"`
	CotAnalysis PhaseReasoning `json:"cot_analysis"`
	RootCause   []any          `json:"root_cause"`
}

// ProjectMetadata represents project metadata.
type ProjectMetadata struct {
	DateRange       string   `json:"date_range"`
	TeamMembers     []string `json:"team_members"`
	TotalDuration   string   `json:"total_duration"`
	PhasesCompleted []string `json:"phases_completed"`
}

// DimensionAverages represents average scores per dimension.
type DimensionAverages struct {
	Clarity      float64 `json:"clarity"`
	Completeness float64 `json:"completeness"`
	Efficiency   float64 `json:"efficiency"`
	Quality      float64 `json:"quality"`
	Impact       float64 `json:"impact"`
}

// ProjectOverall represents overall project scores.
type ProjectOverall struct {
	Score             float64           `json:"score"`
	DimensionAverages DimensionAverages `json:"dimension_averages"`
}

// IQROutliers represents IQR outlier detection results.
type IQROutliers struct {
	IQROutliers       int     `json:"iqr_outliers"`
	ExtremePercentage float64 `json:"extreme_percentage"`
	FlagExtremes      bool    `json:"flag_extremes"`
}

// CalibrationDrift represents calibration drift detection results.
type CalibrationDrift struct {
	ProjectOverall float64 `json:"project_overall"`
	FlagInflated   bool    `json:"flag_inflated"`
}

// ValidationData represents validation metadata.
type ValidationData struct {
	Outliers          IQROutliers      `json:"outliers"`
	ConsistencyChecks []any            `json:"consistency_checks"`
	CotQuality        []any            `json:"cot_quality"`
	CalibrationDrift  CalibrationDrift `json:"calibration_drift"`
}

// RetrospectiveData represents the complete retrospective JSON structure.
type RetrospectiveData struct {
	SchemaVersion  string               `json:"schema_version"`
	Project        string               `json:"project"`
	Metadata       ProjectMetadata      `json:"metadata"`
	Phases         map[string]PhaseData `json:"phases"`
	ProjectOverall ProjectOverall       `json:"project_overall"`
	Validation     ValidationData       `json:"validation"`
}

// Expected phases
var expectedPhases = []string{"D1", "D2", "D3", "D4", "S4", "S5", "S6", "S7", "S8", "S9", "S10"}

// NewRetrospectiveValidator creates a new retrospective validator.
func NewRetrospectiveValidator(filePath string) *RetrospectiveValidator {
	return &RetrospectiveValidator{
		filePath: filePath,
		errors:   []RetrospectiveValidationError{},
		warnings: []RetrospectiveValidationError{},
	}
}

// Validate runs all validation rules and returns errors and warnings.
func (v *RetrospectiveValidator) Validate() ([]RetrospectiveValidationError, []RetrospectiveValidationError, error) {
	v.errors = []RetrospectiveValidationError{}
	v.warnings = []RetrospectiveValidationError{}

	// Read file
	content, err := os.ReadFile(v.filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read file: %w", err)
	}

	// Extract JSON from markdown
	data, err := v.extractJSONFromMarkdown(string(content))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract JSON: %w", err)
	}

	// Run validation checks
	v.validateSchema(data)
	v.validateIQROutliers(data)
	v.validateExtremePercentage(data)
	v.validateCotQuality(data)
	v.validateCalibrationDrift(data)

	return v.errors, v.warnings, nil
}

// extractJSONFromMarkdown extracts JSON block from markdown using regex.
func (v *RetrospectiveValidator) extractJSONFromMarkdown(content string) (*RetrospectiveData, error) {
	// Extract JSON block (between ```json and ```)
	pattern := regexp.MustCompile("(?s)```json\\n(.*?)\\n```")
	matches := pattern.FindStringSubmatch(content)

	if matches == nil {
		return nil, fmt.Errorf("no JSON block found in markdown file")
	}

	// Parse JSON
	var data RetrospectiveData
	if err := json.Unmarshal([]byte(matches[1]), &data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	return &data, nil
}

// validateSchema validates JSON structure and required fields.
func (v *RetrospectiveValidator) validateSchema(data *RetrospectiveData) {
	// Check required top-level fields
	if data.SchemaVersion == "" {
		v.addError("schema_version", "Missing required field: schema_version")
	}
	if data.Project == "" {
		v.addError("project", "Missing required field: project")
	}

	// Check phases
	for _, phase := range expectedPhases {
		phaseData, exists := data.Phases[phase]
		if !exists {
			v.addError(phase, fmt.Sprintf("Missing required phase: %s", phase))
			continue
		}

		// Check scores
		v.validatePhaseScores(phase, phaseData.Scores)

		// Check reasoning
		if len(phaseData.Reasoning.Clarity) < 10 {
			v.addError(fmt.Sprintf("%s.reasoning.clarity", phase),
				fmt.Sprintf("Reasoning too short (minimum 10 chars, got %d)", len(phaseData.Reasoning.Clarity)))
		}

		// Check CoT analysis (required per S5 research)
		v.validateCoTAnalysis(phase, phaseData.CotAnalysis)
	}
}

// validatePhaseScores validates individual phase scores.
func (v *RetrospectiveValidator) validatePhaseScores(phase string, scores PhaseScores) {
	// Validate each dimension score (1-5 range)
	scoreMap := map[string]int{
		"clarity":      scores.Clarity,
		"completeness": scores.Completeness,
		"efficiency":   scores.Efficiency,
		"quality":      scores.Quality,
		"impact":       scores.Impact,
	}

	for dim, score := range scoreMap {
		if score < 1 || score > 5 {
			v.addError(fmt.Sprintf("%s.%s", phase, dim),
				fmt.Sprintf("Score must be integer 1-5, got %d", score))
		}
	}
}

// validateCoTAnalysis validates Chain-of-Thought analysis.
func (v *RetrospectiveValidator) validateCoTAnalysis(phase string, cot PhaseReasoning) {
	cotMap := map[string]string{
		"clarity":      cot.Clarity,
		"completeness": cot.Completeness,
		"efficiency":   cot.Efficiency,
		"quality":      cot.Quality,
		"impact":       cot.Impact,
	}

	for dim, text := range cotMap {
		if len(text) < 50 {
			v.addError(fmt.Sprintf("%s.cot_analysis.%s", phase, dim),
				fmt.Sprintf("CoT analysis too short (minimum 50 chars, got %d)", len(text)))
		}
	}
}

// validateIQROutliers validates using IQR-based outlier detection.
func (v *RetrospectiveValidator) validateIQROutliers(data *RetrospectiveData) {
	// Extract all 55 dimension scores (11 phases × 5 dimensions)
	var dimensionScores []int

	for _, phase := range expectedPhases {
		phaseData, exists := data.Phases[phase]
		if !exists {
			continue
		}

		dimensionScores = append(dimensionScores,
			phaseData.Scores.Clarity,
			phaseData.Scores.Completeness,
			phaseData.Scores.Efficiency,
			phaseData.Scores.Quality,
			phaseData.Scores.Impact,
		)
	}

	if len(dimensionScores) < 55 {
		v.addError("iqr_outliers", fmt.Sprintf("Expected 55 scores, got %d", len(dimensionScores)))
		return
	}

	// Calculate IQR
	sortedScores := make([]int, len(dimensionScores))
	copy(sortedScores, dimensionScores)
	sort.Ints(sortedScores)

	n := len(sortedScores)
	q1Idx := n / 4
	q3Idx := 3 * n / 4
	q1 := float64(sortedScores[q1Idx])
	q3 := float64(sortedScores[q3Idx])
	iqr := q3 - q1

	// Identify outliers (scores outside Q1 - 1.5*IQR or Q3 + 1.5*IQR)
	lowerBound := q1 - 1.5*iqr
	upperBound := q3 + 1.5*iqr

	outlierCount := 0
	for _, score := range dimensionScores {
		if float64(score) < lowerBound || float64(score) > upperBound {
			outlierCount++
		}
	}

	// Warn on >5 outliers (>9% of scores)
	if outlierCount > 5 {
		v.addWarning("iqr_outliers",
			fmt.Sprintf("Found %d outliers (>9%% threshold). Valid range: [%.1f, %.1f]",
				outlierCount, lowerBound, upperBound))
	}
}

// validateExtremePercentage validates extreme percentage (supplement to IQR).
func (v *RetrospectiveValidator) validateExtremePercentage(data *RetrospectiveData) {
	var dimensionScores []int

	for _, phase := range expectedPhases {
		phaseData, exists := data.Phases[phase]
		if !exists {
			continue
		}

		dimensionScores = append(dimensionScores,
			phaseData.Scores.Clarity,
			phaseData.Scores.Completeness,
			phaseData.Scores.Efficiency,
			phaseData.Scores.Quality,
			phaseData.Scores.Impact,
		)
	}

	if len(dimensionScores) == 0 {
		return
	}

	// Count extremes (5s and 1-2s)
	highScores := 0
	lowScores := 0
	for _, score := range dimensionScores {
		if score == 5 {
			highScores++
		}
		if score <= 2 {
			lowScores++
		}
	}

	total := len(dimensionScores)
	extremePct := float64(highScores+lowScores) / float64(total)

	// Warn on >80% extreme scores
	if extremePct > 0.80 {
		v.addWarning("extreme_percentage",
			fmt.Sprintf("Extreme percentage %.1f%% (>80%% threshold). High scores: %d, Low scores: %d",
				extremePct*100, highScores, lowScores))
	}
}

// validateCotQuality validates CoT quality (flag 5/5 scores with shallow reasoning).
func (v *RetrospectiveValidator) validateCotQuality(data *RetrospectiveData) {
	for _, phase := range expectedPhases {
		phaseData, exists := data.Phases[phase]
		if !exists {
			continue
		}

		// Check each dimension
		scores := map[string]int{
			"clarity":      phaseData.Scores.Clarity,
			"completeness": phaseData.Scores.Completeness,
			"efficiency":   phaseData.Scores.Efficiency,
			"quality":      phaseData.Scores.Quality,
			"impact":       phaseData.Scores.Impact,
		}

		cots := map[string]string{
			"clarity":      phaseData.CotAnalysis.Clarity,
			"completeness": phaseData.CotAnalysis.Completeness,
			"efficiency":   phaseData.CotAnalysis.Efficiency,
			"quality":      phaseData.CotAnalysis.Quality,
			"impact":       phaseData.CotAnalysis.Impact,
		}

		for dim, score := range scores {
			cotText := cots[dim]

			// Flag 5/5 scores with <75 char CoT
			if score == 5 && len(cotText) < 75 {
				v.addWarning(fmt.Sprintf("%s.%s", phase, dim),
					fmt.Sprintf("Score 5/5 but CoT <75 chars (got %d)", len(cotText)))
			}

			// Flag any score with <50 char CoT (already checked in schema, but double-check)
			if len(cotText) < 50 {
				v.addError(fmt.Sprintf("%s.%s", phase, dim),
					fmt.Sprintf("CoT <50 chars minimum (got %d)", len(cotText)))
			}
		}
	}
}

// validateCalibrationDrift validates calibration drift (flag if project overall >4.5).
func (v *RetrospectiveValidator) validateCalibrationDrift(data *RetrospectiveData) {
	projectOverall := data.ProjectOverall.Score

	if projectOverall > 4.5 {
		v.addWarning("project_overall",
			fmt.Sprintf("Project overall score %.2f >4.5 (rare, possible inflation)", projectOverall))
	}
}

// addError adds an error to the validation results.
func (v *RetrospectiveValidator) addError(field, message string) {
	v.errors = append(v.errors, RetrospectiveValidationError{
		Field:    field,
		Message:  message,
		Severity: "error",
	})
}

// addWarning adds a warning to the validation results.
func (v *RetrospectiveValidator) addWarning(field, message string) {
	v.warnings = append(v.warnings, RetrospectiveValidationError{
		Field:    field,
		Message:  message,
		Severity: "warning",
	})
}

// ValidateRetrospectiveFile validates a single retrospective markdown file.
func ValidateRetrospectiveFile(filePath string) ([]RetrospectiveValidationError, []RetrospectiveValidationError, error) {
	validator := NewRetrospectiveValidator(filePath)
	return validator.Validate()
}
