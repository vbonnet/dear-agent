package review

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

// TestEndToEndReviewWorkflow tests the complete review workflow
func TestEndToEndReviewWorkflow(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup project structure
	setupTestProject(t, tmpDir)

	st := &status.StatusV2{
		ProjectName:     "test-project",
		RiskLevel:       "M",
		CurrentWaypoint: "S8",
	}

	engine := NewReviewEngine(tmpDir, st)

	// Test 1: Low risk task - should use batch review
	lowRiskTask := &status.Task{
		ID:           "task-1",
		Title:        "Fix typo in documentation",
		Description:  "fix typo in readme",
		Deliverables: []string{"README.md"},
	}

	createFile(t, tmpDir, "README.md", "# Project\nSome dcoumentation with typo\n")

	if engine.ShouldTriggerPerTaskReview(lowRiskTask) {
		t.Error("Low risk task should not trigger per-task review")
	}

	// Test 2: High risk task - should use per-task review
	highRiskTask := &status.Task{
		ID:           "task-2",
		Title:        "Implement authentication system",
		Description:  "implement new authentication",
		Deliverables: []string{"auth/handler.go", "auth/middleware.go"},
	}

	authCode := `package auth

import (
	"database/sql"
	"fmt"
)

func Login(username, password string) error {
	// Security issue: SQL injection vulnerability
	query := fmt.Sprintf("SELECT * FROM users WHERE username='%s' AND password='%s'", username, password)
	db.Exec(query)
	return nil
}

func ValidateToken(token string) bool {
	// Security issue: hardcoded secret
	secret := "my-secret-key-123"
	return token == secret
}
`

	createFile(t, tmpDir, "auth/handler.go", authCode)
	createFile(t, tmpDir, "auth/middleware.go", "package auth\n// Middleware\n")

	if !engine.ShouldTriggerPerTaskReview(highRiskTask) {
		t.Error("High risk task should trigger per-task review")
	}

	// Perform per-task review
	result, err := engine.ReviewTask(highRiskTask)
	if err != nil {
		t.Fatalf("Per-task review failed: %v", err)
	}

	// Validate review result
	if result == nil {
		t.Fatal("Expected non-nil review result")
	}

	if result.ReviewType != ReviewTypePerTask {
		t.Errorf("Expected per-task review type, got %s", result.ReviewType)
	}

	// Should detect security issues
	if result.Metrics.SecurityScore >= 90.0 {
		t.Errorf("Expected low security score due to vulnerabilities, got %.1f", result.Metrics.SecurityScore)
	}

	// Should have blocking issues (P0/P1)
	if len(result.BlockingIssues) == 0 {
		t.Error("Expected blocking issues for security vulnerabilities")
	}

	// Should fail review
	if result.Passed {
		t.Error("Expected review to fail due to security issues")
	}

	// Test 3: Batch review for multiple low risk tasks
	batchTasks := []*status.Task{
		{
			ID:           "task-3",
			Description:  "update comments",
			Deliverables: []string{"utils.go"},
		},
		{
			ID:           "task-4",
			Description:  "fix formatting",
			Deliverables: []string{"helpers.go"},
		},
	}

	createFile(t, tmpDir, "utils.go", "package main\n// Utils\nfunc Helper() {}\n")
	createFile(t, tmpDir, "helpers.go", "package main\n// Helpers\nfunc Util() {}\n")

	if !engine.ShouldTriggerBatchReview(batchTasks) {
		t.Error("Should trigger batch review for low risk tasks")
	}

	batchResult, err := engine.ReviewBatch(batchTasks)
	if err != nil {
		t.Fatalf("Batch review failed: %v", err)
	}

	if batchResult.ReviewType != ReviewTypeBatch {
		t.Errorf("Expected batch review type, got %s", batchResult.ReviewType)
	}

	// Batch should have fewer personas
	if len(batchResult.PersonaResults) > 3 {
		t.Errorf("Batch review should use limited personas, got %d", len(batchResult.PersonaResults))
	}
}

// TestSecurityPersonaIntegration tests security persona review
func TestSecurityPersonaIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	vulnerableCode := `package main

import (
	"database/sql"
	"fmt"
	"os"
)

func ProcessUserInput(input string) {
	// P0: SQL injection
	query := "SELECT * FROM users WHERE name = '" + input + "'"
	db.Exec(query)

	// P0: Hardcoded credentials
	password := "admin123"
	apiKey := "sk_live_abc123"

	// P0: Command injection
	os.System("rm -rf " + input)

	// P1: eval usage
	eval(input)
}
`

	createFile(t, tmpDir, "vulnerable.go", vulnerableCode)

	task := &status.Task{
		ID:           "security-test",
		Description:  "test security detection",
		Deliverables: []string{"vulnerable.go"},
	}

	result, err := engine.ReviewTask(task)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Find security persona result
	var securityResult *PersonaResult
	for i := range result.PersonaResults {
		if result.PersonaResults[i].Persona == PersonaSecurity {
			securityResult = &result.PersonaResults[i]
			break
		}
	}

	if securityResult == nil {
		t.Fatal("Security persona should have reviewed the code")
	}

	// Should detect multiple security issues
	if len(securityResult.Issues) == 0 {
		t.Error("Security persona should detect vulnerabilities")
	}

	// Should have critical (P0) issues
	hasP0 := false
	for _, issue := range securityResult.Issues {
		if issue.Severity == SeverityP0 {
			hasP0 = true
			break
		}
	}

	if !hasP0 {
		t.Error("Should detect P0 security issues")
	}

	// Security score should be very low
	if securityResult.Score >= 50.0 {
		t.Errorf("Expected very low security score, got %.1f", securityResult.Score)
	}
}

// TestPerformancePersonaIntegration tests performance persona review
func TestPerformancePersonaIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	performanceIssues := `package main

import (
	"database/sql"
	"time"
	"regexp"
)

func ProcessItems(items []string) {
	// P1: N+1 query pattern
	for _, item := range items {
		db.Query("SELECT * FROM details WHERE item = ?", item)
	}

	// P1: Long sleep
	time.Sleep(5 * time.Second)

	// P2: Regex compilation in loop
	for _, item := range items {
		re := regexp.MustCompile("pattern")
		re.MatchString(item)
	}

	// P2: Append without capacity
	var results []string
	for i := 0; i < 10000; i++ {
		results = append(results, "item")
	}
}
`

	createFile(t, tmpDir, "perf.go", performanceIssues)

	task := &status.Task{
		ID:           "perf-test",
		Description:  "test performance detection",
		Deliverables: []string{"perf.go"},
	}

	result, err := engine.ReviewTask(task)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Find performance persona result
	var perfResult *PersonaResult
	for i := range result.PersonaResults {
		if result.PersonaResults[i].Persona == PersonaPerformance {
			perfResult = &result.PersonaResults[i]
			break
		}
	}

	if perfResult == nil {
		t.Fatal("Performance persona should have reviewed the code")
	}

	// Should detect performance issues
	if len(perfResult.Issues) == 0 {
		t.Error("Performance persona should detect issues")
	}

	// Performance score should be reduced
	if perfResult.Score >= 90.0 {
		t.Errorf("Expected reduced performance score, got %.1f", perfResult.Score)
	}
}

// TestMaintainabilityPersonaIntegration tests maintainability persona review
func TestMaintainabilityPersonaIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	maintainabilityIssues := `package main

// TODO: refactor this mess
// FIXME: this is broken
// HACK: temporary workaround
// XXX: dangerous code

func complexFunction() {
	// Single letter variable
	var x int

	// Many nested conditions (high complexity)
	if true {
		if true {
			if true {
				if true {
					if true {
						// deeply nested
					}
				}
			}
		}
	}
}
`

	createFile(t, tmpDir, "maint.go", maintainabilityIssues)

	task := &status.Task{
		ID:           "maint-test",
		Description:  "test maintainability detection",
		Deliverables: []string{"maint.go"},
	}

	result, err := engine.ReviewTask(task)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Find maintainability persona result
	var maintResult *PersonaResult
	for i := range result.PersonaResults {
		if result.PersonaResults[i].Persona == PersonaMaintainability {
			maintResult = &result.PersonaResults[i]
			break
		}
	}

	if maintResult == nil {
		t.Fatal("Maintainability persona should have reviewed the code")
	}

	// Should detect maintainability issues (TODO, FIXME, etc.)
	if len(maintResult.Issues) == 0 {
		t.Error("Maintainability persona should detect code smells")
	}
}

// TestReliabilityPersonaIntegration tests reliability persona review
func TestReliabilityPersonaIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	reliabilityIssues := `package main

import "os"

func unreliableCode() error {
	// P0: Division by zero risk
	x := 10 / 0

	// P1: Ignoring errors
	_ = processData()

	// P1: Swallowing errors
	if err := doSomething(); err != nil {
		// ignore error
		return nil
	}

	// P2: Infinite loop risk
	for {
		// no break condition
	}

	return nil
}

func processData() error {
	file, err := os.Open("test.txt")
	// P1: Not checking Close error
	defer file.Close()
	return err
}

func doSomething() error {
	return nil
}
`

	createFile(t, tmpDir, "security/reliability.go", reliabilityIssues)

	task := &status.Task{
		ID:           "reliability-test",
		Description:  "test reliability detection",
		Deliverables: []string{"security/reliability.go"},
	}

	result, err := engine.ReviewTask(task)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	// Find reliability persona result
	var relResult *PersonaResult
	for i := range result.PersonaResults {
		if result.PersonaResults[i].Persona == PersonaReliability {
			relResult = &result.PersonaResults[i]
			break
		}
	}

	if relResult == nil {
		t.Fatal("Reliability persona should have reviewed the code")
	}

	// Should detect reliability issues
	if len(relResult.Issues) == 0 {
		t.Error("Reliability persona should detect error handling issues")
	}
}

// TestRiskAdaptiveStrategy tests risk-adaptive review strategy
func TestRiskAdaptiveStrategy(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	tests := []struct {
		name             string
		taskDesc         string
		files            map[string]string
		expectedStrategy string
		expectedPersonas int
	}{
		{
			name:     "XS risk - batch review",
			taskDesc: "fix typo",
			files: map[string]string{
				"doc.md": "# Documentation\nSome text\n",
			},
			expectedStrategy: "batch",
			expectedPersonas: 3, // Minimal personas
		},
		{
			name:     "L risk - per-task review",
			taskDesc: "implement authentication",
			files: map[string]string{
				"auth/handler.go": "package auth\n// Auth code\n",
			},
			expectedStrategy: "per_task",
			expectedPersonas: 5, // All personas
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create files
			var deliverables []string
			for path, content := range tt.files {
				createFile(t, tmpDir, path, content)
				deliverables = append(deliverables, path)
			}

			task := &status.Task{
				ID:           "test-task",
				Description:  tt.taskDesc,
				Deliverables: deliverables,
			}

			// Check risk level and strategy
			riskLevel := engine.riskAdapter.CalculateRiskLevel(task, tmpDir)
			strategy := engine.riskAdapter.GetReviewStrategy(riskLevel)

			if strategy != tt.expectedStrategy {
				t.Errorf("Expected strategy %s, got %s", tt.expectedStrategy, strategy)
			}

			// Check persona selection
			personas := engine.selectPersonasForTask(task, riskLevel)

			if len(personas) != tt.expectedPersonas {
				t.Errorf("Expected %d personas, got %d", tt.expectedPersonas, len(personas))
			}
		})
	}
}

// TestReviewReportGeneration tests review report generation
func TestReviewReportGeneration(t *testing.T) {
	result := &ReviewResult{
		TaskID:         "task-123",
		RiskLevel:      RiskLevelL,
		ReviewType:     ReviewTypePerTask,
		Timestamp:      time.Now(),
		AggregateScore: 75.5,
		Passed:         false,
		PersonaResults: []PersonaResult{
			{
				Persona: PersonaSecurity,
				Score:   60.0,
				Issues: []ReviewIssue{
					{
						Severity:   SeverityP0,
						Category:   "sql_injection",
						Message:    "SQL injection vulnerability detected",
						FilePath:   "handler.go",
						LineNumber: 42,
						Suggestion: "Use prepared statements",
					},
				},
			},
		},
		BlockingIssues: []ReviewIssue{
			{
				Severity: SeverityP0,
				Message:  "Critical security issue",
			},
		},
		Recommendations: []string{
			"Use prepared statements",
			"Add input validation",
		},
		Metrics: ReviewMetrics{
			TotalIssues:      3,
			P0Issues:         1,
			P1Issues:         1,
			P2Issues:         1,
			SecurityScore:    60.0,
			PerformanceScore: 85.0,
			ReviewDurationMS: 1234,
		},
	}

	// Test text report generation
	textReport := GenerateReviewReport(result)

	if textReport == "" {
		t.Error("Expected non-empty text report")
	}

	// Verify key elements in report
	expectedElements := []string{
		"task-123",
		"FAILED",
		"Security",
		"SQL injection",
		"BLOCKING ISSUES",
		"RECOMMENDATIONS",
	}

	for _, elem := range expectedElements {
		if !contains(textReport, elem) {
			t.Errorf("Report should contain '%s'", elem)
		}
	}

	// Test markdown report generation
	mdReport := GenerateMarkdownReport(result)

	if mdReport == "" {
		t.Error("Expected non-empty markdown report")
	}

	// Verify markdown formatting
	mdElements := []string{
		"# Multi-Persona Review Report",
		"## Overview",
		"## Metrics Summary",
		"**REVIEW FAILED**",
	}

	for _, elem := range mdElements {
		if !contains(mdReport, elem) {
			t.Errorf("Markdown report should contain '%s'", elem)
		}
	}

	// Test summary generation
	summary := GenerateSummary(result)

	if summary == "" {
		t.Error("Expected non-empty summary")
	}

	if !contains(summary, "FAILED") {
		t.Error("Summary should indicate FAILED status")
	}
}

// Helper functions

func setupTestProject(t *testing.T, dir string) {
	// Create basic project structure
	dirs := []string{"auth", "api", "models", "utils"}
	for _, d := range dirs {
		os.MkdirAll(filepath.Join(dir, d), 0755)
	}
}

func createFile(t *testing.T, baseDir, path, content string) {
	fullPath := filepath.Join(baseDir, path)
	os.MkdirAll(filepath.Dir(fullPath), 0755)

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test file %s: %v", path, err)
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) >= len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
