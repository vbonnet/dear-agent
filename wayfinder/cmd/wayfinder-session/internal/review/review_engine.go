package review

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// ReviewEngine orchestrates multi-persona reviews with risk-adaptive strategies
type ReviewEngine struct {
	projectDir  string
	status      *status.StatusV2
	riskAdapter *RiskAdapter
	config      *ReviewConfig
}

// ReviewConfig holds configuration for the review engine
type ReviewConfig struct {
	// Quality gates
	MinAssertionDensity float64
	MinCoveragePercent  float64

	// Timeouts
	TestExecutionSeconds   int
	ReviewExecutionSeconds int

	// Risk thresholds
	XSMaxLOC int
	SMaxLOC  int
	MMaxLOC  int
	LMaxLOC  int
	XLMinLOC int

	// Review triggers
	PerTaskMinRisk     RiskLevel
	BatchReviewMaxRisk RiskLevel

	// Retry limits
	MaxTestRetries        int
	MaxIntegrationRetries int
}

// DefaultReviewConfig returns the default review configuration
func DefaultReviewConfig() *ReviewConfig {
	return &ReviewConfig{
		MinAssertionDensity:    0.5,
		MinCoveragePercent:     80.0,
		TestExecutionSeconds:   300,
		ReviewExecutionSeconds: 600,
		XSMaxLOC:               50,
		SMaxLOC:                200,
		MMaxLOC:                500,
		LMaxLOC:                1000,
		XLMinLOC:               1001,
		PerTaskMinRisk:         RiskLevelL,
		BatchReviewMaxRisk:     RiskLevelM,
		MaxTestRetries:         3,
		MaxIntegrationRetries:  2,
	}
}

// NewReviewEngine creates a new review engine instance
func NewReviewEngine(projectDir string, st *status.StatusV2) *ReviewEngine {
	config := DefaultReviewConfig()
	return &ReviewEngine{
		projectDir:  projectDir,
		status:      st,
		riskAdapter: NewRiskAdapter(config),
		config:      config,
	}
}

// ReviewResult represents the aggregated result of a review
type ReviewResult struct {
	TaskID          string          `json:"task_id"`
	RiskLevel       RiskLevel       `json:"risk_level"`
	ReviewType      ReviewType      `json:"review_type"` // per_task or batch
	Timestamp       time.Time       `json:"timestamp"`
	PersonaResults  []PersonaResult `json:"persona_results"`
	AggregateScore  float64         `json:"aggregate_score"` // 0-100
	BlockingIssues  []ReviewIssue   `json:"blocking_issues"`
	Recommendations []string        `json:"recommendations"`
	Passed          bool            `json:"passed"`
	Metrics         ReviewMetrics   `json:"metrics"`
}

// ReviewMetrics contains quantitative review metrics
type ReviewMetrics struct {
	TotalIssues          int     `json:"total_issues"`
	P0Issues             int     `json:"p0_issues"`
	P1Issues             int     `json:"p1_issues"`
	P2Issues             int     `json:"p2_issues"`
	P3Issues             int     `json:"p3_issues"`
	SecurityScore        float64 `json:"security_score"`
	PerformanceScore     float64 `json:"performance_score"`
	MaintainabilityScore float64 `json:"maintainability_score"`
	UXScore              float64 `json:"ux_score"`
	ReliabilityScore     float64 `json:"reliability_score"`
	ReviewDurationMS     int64   `json:"review_duration_ms"`
}

// ReviewIssue represents a single issue found during review
type ReviewIssue struct {
	Persona     PersonaType   `json:"persona"`
	Severity    IssueSeverity `json:"severity"`
	Category    string        `json:"category"`
	Message     string        `json:"message"`
	FilePath    string        `json:"file_path,omitempty"`
	LineNumber  int           `json:"line_number,omitempty"`
	Suggestion  string        `json:"suggestion,omitempty"`
	CodeSnippet string        `json:"code_snippet,omitempty"`
}

// IssueSeverity defines the severity levels for review issues
type IssueSeverity string

const (
	SeverityP0 IssueSeverity = "P0" // Critical - MUST fix before deploy
	SeverityP1 IssueSeverity = "P1" // High - MUST fix before deploy
	SeverityP2 IssueSeverity = "P2" // Medium - Should fix (non-blocking)
	SeverityP3 IssueSeverity = "P3" // Low - Optional fix
)

// ReviewType defines whether review is per-task or batch
type ReviewType string

const (
	ReviewTypePerTask ReviewType = "per_task"
	ReviewTypeBatch   ReviewType = "batch"
)

// ShouldTriggerPerTaskReview determines if a task requires immediate review
func (e *ReviewEngine) ShouldTriggerPerTaskReview(task *status.Task) bool {
	riskLevel := e.riskAdapter.CalculateRiskLevel(task, e.projectDir)
	return riskLevel >= e.config.PerTaskMinRisk
}

// ShouldTriggerBatchReview determines if batch review should run
func (e *ReviewEngine) ShouldTriggerBatchReview(completedTasks []*status.Task) bool {
	// Only review tasks that haven't been reviewed yet
	var unreviewed []*status.Task
	for _, task := range completedTasks {
		if task.TestsStatus == nil || *task.TestsStatus != "reviewed" {
			unreviewed = append(unreviewed, task)
		}
	}

	if len(unreviewed) == 0 {
		return false
	}

	// Check if all unreviewed tasks are low/medium risk
	for _, task := range unreviewed {
		riskLevel := e.riskAdapter.CalculateRiskLevel(task, e.projectDir)
		if riskLevel > e.config.BatchReviewMaxRisk {
			return false
		}
	}

	return true
}

// ReviewTask performs a per-task review for high-risk tasks
func (e *ReviewEngine) ReviewTask(task *status.Task) (*ReviewResult, error) {
	startTime := time.Now()

	riskLevel := e.riskAdapter.CalculateRiskLevel(task, e.projectDir)

	// Determine which personas to invoke based on risk level
	personas := e.selectPersonasForTask(task, riskLevel)

	// Execute reviews for each persona
	var personaResults []PersonaResult
	var allIssues []ReviewIssue

	for _, persona := range personas {
		result, err := e.executePersonaReview(persona, task)
		if err != nil {
			// Log error but continue with other personas
			fmt.Fprintf(os.Stderr, "Warning: %s review failed: %v\n", persona, err)
			continue
		}

		personaResults = append(personaResults, result)
		allIssues = append(allIssues, result.Issues...)
	}

	// Calculate aggregate metrics
	metrics := e.calculateMetrics(allIssues)
	metrics.ReviewDurationMS = time.Since(startTime).Milliseconds()

	// Extract blocking issues (P0/P1, or P2 for XL risk)
	blockingIssues := e.extractBlockingIssues(allIssues, riskLevel)

	// Calculate aggregate score
	aggregateScore := e.calculateAggregateScore(personaResults)

	// Determine if review passed
	passed := len(blockingIssues) == 0

	// Collect recommendations
	recommendations := e.collectRecommendations(personaResults)

	return &ReviewResult{
		TaskID:          task.ID,
		RiskLevel:       riskLevel,
		ReviewType:      ReviewTypePerTask,
		Timestamp:       time.Now(),
		PersonaResults:  personaResults,
		AggregateScore:  aggregateScore,
		BlockingIssues:  blockingIssues,
		Recommendations: recommendations,
		Passed:          passed,
		Metrics:         metrics,
	}, nil
}

// ReviewBatch performs a batch review for low/medium risk tasks
func (e *ReviewEngine) ReviewBatch(tasks []*status.Task) (*ReviewResult, error) {
	startTime := time.Now()

	// Aggregate all task files for batch review
	var allFiles []string
	var taskIDs []string
	var maxRisk RiskLevel = RiskLevelXS

	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
		allFiles = append(allFiles, task.Deliverables...)

		riskLevel := e.riskAdapter.CalculateRiskLevel(task, e.projectDir)
		if riskLevel > maxRisk {
			maxRisk = riskLevel
		}
	}

	// Select personas for batch review (lighter set for efficiency)
	personas := []PersonaType{
		PersonaSecurity,
		PersonaPerformance,
		PersonaMaintainability,
	}

	// Execute batch reviews
	var personaResults []PersonaResult
	var allIssues []ReviewIssue

	for _, persona := range personas {
		result, err := e.executeBatchPersonaReview(persona, allFiles)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: %s batch review failed: %v\n", persona, err)
			continue
		}

		personaResults = append(personaResults, result)
		allIssues = append(allIssues, result.Issues...)
	}

	// Calculate metrics
	metrics := e.calculateMetrics(allIssues)
	metrics.ReviewDurationMS = time.Since(startTime).Milliseconds()

	// Extract blocking issues
	blockingIssues := e.extractBlockingIssues(allIssues, maxRisk)

	// Calculate aggregate score
	aggregateScore := e.calculateAggregateScore(personaResults)

	// Collect recommendations
	recommendations := e.collectRecommendations(personaResults)

	return &ReviewResult{
		TaskID:          fmt.Sprintf("batch-%s", taskIDs[0]),
		RiskLevel:       maxRisk,
		ReviewType:      ReviewTypeBatch,
		Timestamp:       time.Now(),
		PersonaResults:  personaResults,
		AggregateScore:  aggregateScore,
		BlockingIssues:  blockingIssues,
		Recommendations: recommendations,
		Passed:          len(blockingIssues) == 0,
		Metrics:         metrics,
	}, nil
}

// selectPersonasForTask determines which personas should review based on risk
func (e *ReviewEngine) selectPersonasForTask(task *status.Task, riskLevel RiskLevel) []PersonaType {
	// Base personas for all reviews
	personas := []PersonaType{
		PersonaSecurity,
		PersonaPerformance,
		PersonaMaintainability,
	}

	// Add UX persona for M+ risk
	if riskLevel >= RiskLevelM {
		personas = append(personas, PersonaUX)
	}

	// Add Reliability persona for L+ risk
	if riskLevel >= RiskLevelL {
		personas = append(personas, PersonaReliability)
	}

	return personas
}

// executePersonaReview invokes a single persona's review logic
func (e *ReviewEngine) executePersonaReview(persona PersonaType, task *status.Task) (PersonaResult, error) {
	// Get persona configuration
	config := GetPersonaConfig(persona)

	// Build file paths from task deliverables
	var files []string
	for _, deliverable := range task.Deliverables {
		fullPath := filepath.Join(e.projectDir, deliverable)
		files = append(files, fullPath)
	}

	// Execute review (could be via external tool or internal logic)
	return e.invokePersonaReviewTool(persona, files, config)
}

// executeBatchPersonaReview performs batch review for multiple files
func (e *ReviewEngine) executeBatchPersonaReview(persona PersonaType, files []string) (PersonaResult, error) {
	config := GetPersonaConfig(persona)
	return e.invokePersonaReviewTool(persona, files, config)
}

// invokePersonaReviewTool calls the actual review tool or runs internal checks
func (e *ReviewEngine) invokePersonaReviewTool(persona PersonaType, files []string, config PersonaConfig) (PersonaResult, error) {
	// Try external tool first (if available)
	result, err := e.tryExternalReviewTool(persona, files)
	if err == nil {
		return result, nil
	}

	// Fall back to internal checks
	return e.runInternalPersonaChecks(persona, files, config)
}

// tryExternalReviewTool attempts to use external review tools
func (e *ReviewEngine) tryExternalReviewTool(persona PersonaType, files []string) (PersonaResult, error) {
	// Example: Try to call golangci-lint for Go files
	if persona == PersonaMaintainability && e.hasGoFiles(files) {
		return e.runGolangciLint(files)
	}

	return PersonaResult{}, fmt.Errorf("no external tool available for %s", persona)
}

// runGolangciLint executes golangci-lint for maintainability checks
func (e *ReviewEngine) runGolangciLint(files []string) (PersonaResult, error) {
	cmd := exec.Command("golangci-lint", "run", "./...")
	cmd.Dir = e.projectDir

	output, err := cmd.CombinedOutput()

	result := PersonaResult{
		Persona:    PersonaMaintainability,
		Timestamp:  time.Now(),
		Score:      100.0,
		Confidence: ConfidenceHigh,
		Issues:     []ReviewIssue{},
	}

	if err != nil {
		// Parse golangci-lint output for issues
		// This is simplified - real implementation would parse JSON output
		result.Score = 70.0
		result.Issues = append(result.Issues, ReviewIssue{
			Persona:  PersonaMaintainability,
			Severity: SeverityP2,
			Category: "linting",
			Message:  string(output),
		})
	}

	return result, nil
}

// hasGoFiles checks if any files are Go source files
func (e *ReviewEngine) hasGoFiles(files []string) bool {
	for _, f := range files {
		if filepath.Ext(f) == ".go" {
			return true
		}
	}
	return false
}

// runInternalPersonaChecks performs internal review logic
func (e *ReviewEngine) runInternalPersonaChecks(persona PersonaType, files []string, config PersonaConfig) (PersonaResult, error) {
	result := PersonaResult{
		Persona:    persona,
		Timestamp:  time.Now(),
		Score:      100.0,
		Confidence: ConfidenceMedium,
		Issues:     []ReviewIssue{},
		Summary:    fmt.Sprintf("%s review completed (internal checks)", persona),
	}

	// Run persona-specific checks based on focus areas
	for _, file := range files {
		issues, err := e.checkFile(file, persona, config)
		if err != nil {
			continue
		}
		result.Issues = append(result.Issues, issues...)
	}

	// Adjust score based on issues
	if len(result.Issues) > 0 {
		result.Score = e.calculatePersonaScore(result.Issues)
	}

	return result, nil
}

// checkFile performs file-level checks based on persona config
func (e *ReviewEngine) checkFile(filePath string, persona PersonaType, config PersonaConfig) ([]ReviewIssue, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	content := string(data)
	var issues []ReviewIssue

	// Run pattern-based checks
	for pattern, severity := range config.Patterns {
		if matched, issue := checkPattern(content, pattern, severity, filePath, persona); matched {
			issues = append(issues, issue)
		}
	}

	return issues, nil
}

// calculateMetrics aggregates metrics from all issues
func (e *ReviewEngine) calculateMetrics(issues []ReviewIssue) ReviewMetrics {
	metrics := ReviewMetrics{}

	personaScores := make(map[PersonaType][]ReviewIssue)

	for _, issue := range issues {
		metrics.TotalIssues++

		switch issue.Severity {
		case SeverityP0:
			metrics.P0Issues++
		case SeverityP1:
			metrics.P1Issues++
		case SeverityP2:
			metrics.P2Issues++
		case SeverityP3:
			metrics.P3Issues++
		}

		personaScores[issue.Persona] = append(personaScores[issue.Persona], issue)
	}

	// Calculate persona-specific scores (severity-based scoring)
	for persona, personaIssues := range personaScores {
		score := e.calculatePersonaScore(personaIssues)

		switch persona {
		case PersonaSecurity:
			metrics.SecurityScore = score
		case PersonaPerformance:
			metrics.PerformanceScore = score
		case PersonaMaintainability:
			metrics.MaintainabilityScore = score
		case PersonaUX:
			metrics.UXScore = score
		case PersonaReliability:
			metrics.ReliabilityScore = score
		}
	}

	// Default to 100 for personas with no issues
	if metrics.SecurityScore == 0 && len(personaScores[PersonaSecurity]) == 0 {
		metrics.SecurityScore = 100.0
	}
	if metrics.PerformanceScore == 0 && len(personaScores[PersonaPerformance]) == 0 {
		metrics.PerformanceScore = 100.0
	}
	if metrics.MaintainabilityScore == 0 && len(personaScores[PersonaMaintainability]) == 0 {
		metrics.MaintainabilityScore = 100.0
	}
	if metrics.UXScore == 0 && len(personaScores[PersonaUX]) == 0 {
		metrics.UXScore = 100.0
	}
	if metrics.ReliabilityScore == 0 && len(personaScores[PersonaReliability]) == 0 {
		metrics.ReliabilityScore = 100.0
	}

	return metrics
}

// extractBlockingIssues filters issues that block deployment
func (e *ReviewEngine) extractBlockingIssues(issues []ReviewIssue, riskLevel RiskLevel) []ReviewIssue {
	var blocking []ReviewIssue

	for _, issue := range issues {
		if e.isBlockingIssue(issue, riskLevel) {
			blocking = append(blocking, issue)
		}
	}

	return blocking
}

// isBlockingIssue determines if an issue blocks deployment
func (e *ReviewEngine) isBlockingIssue(issue ReviewIssue, riskLevel RiskLevel) bool {
	// P0 and P1 always block
	if issue.Severity == SeverityP0 || issue.Severity == SeverityP1 {
		return true
	}

	// P2 blocks only for XL risk
	if issue.Severity == SeverityP2 && riskLevel == RiskLevelXL {
		return true
	}

	// P3 never blocks
	return false
}

// calculateAggregateScore computes overall review score
func (e *ReviewEngine) calculateAggregateScore(results []PersonaResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	total := 0.0
	for _, result := range results {
		total += result.Score
	}

	return total / float64(len(results))
}

// calculatePersonaScore calculates score based on issues
func (e *ReviewEngine) calculatePersonaScore(issues []ReviewIssue) float64 {
	score := 100.0

	for _, issue := range issues {
		switch issue.Severity {
		case SeverityP0:
			score -= 25.0
		case SeverityP1:
			score -= 15.0
		case SeverityP2:
			score -= 8.0
		case SeverityP3:
			score -= 3.0
		}
	}

	if score < 0 {
		return 0.0
	}
	return score
}

// collectRecommendations aggregates recommendations from all personas
func (e *ReviewEngine) collectRecommendations(results []PersonaResult) []string {
	var recommendations []string
	seen := make(map[string]bool)

	for _, result := range results {
		for _, issue := range result.Issues {
			if issue.Suggestion != "" && !seen[issue.Suggestion] {
				recommendations = append(recommendations, issue.Suggestion)
				seen[issue.Suggestion] = true
			}
		}
	}

	return recommendations
}

// GenerateReport creates a human-readable review report
func (e *ReviewEngine) GenerateReport(result *ReviewResult) string {
	return GenerateReviewReport(result)
}

// SaveReportToFile writes the review report to a file
func (e *ReviewEngine) SaveReportToFile(result *ReviewResult, outputPath string) error {
	report := e.GenerateReport(result)
	return os.WriteFile(outputPath, []byte(report), 0644)
}

// SaveReportJSON writes the review result as JSON
func (e *ReviewEngine) SaveReportJSON(result *ReviewResult, outputPath string) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal review result: %w", err)
	}

	return os.WriteFile(outputPath, data, 0644)
}
