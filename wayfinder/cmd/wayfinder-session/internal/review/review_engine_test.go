package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/wayfinder/cmd/wayfinder-session/internal/status"
)

func TestNewReviewEngine(t *testing.T) {
	tmpDir := t.TempDir()

	st := &status.StatusV2{
		ProjectName: "test-project",
		RiskLevel:   "M",
	}

	engine := NewReviewEngine(tmpDir, st)

	if engine == nil {
		t.Fatal("Expected non-nil engine")
	}

	if engine.projectDir != tmpDir {
		t.Errorf("Expected projectDir %s, got %s", tmpDir, engine.projectDir)
	}

	if engine.status != st {
		t.Error("Expected status to be set")
	}

	if engine.riskAdapter == nil {
		t.Error("Expected risk adapter to be initialized")
	}

	if engine.config == nil {
		t.Error("Expected config to be initialized")
	}
}

func TestDefaultReviewConfig(t *testing.T) {
	config := DefaultReviewConfig()

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"MinAssertionDensity", config.MinAssertionDensity, 0.5},
		{"MinCoveragePercent", config.MinCoveragePercent, 80.0},
		{"TestExecutionSeconds", config.TestExecutionSeconds, 300},
		{"XSMaxLOC", config.XSMaxLOC, 50},
		{"SMaxLOC", config.SMaxLOC, 200},
		{"MMaxLOC", config.MMaxLOC, 500},
		{"LMaxLOC", config.LMaxLOC, 1000},
		{"XLMinLOC", config.XLMinLOC, 1001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("Expected %s = %v, got %v", tt.name, tt.expected, tt.got)
			}
		})
	}
}

func TestShouldTriggerPerTaskReview(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	tests := []struct {
		name     string
		task     *status.Task
		expected bool
	}{
		{
			name: "XS risk - no per-task review",
			task: &status.Task{
				ID:           "task-1",
				Description:  "fix typo",
				Deliverables: []string{"test.txt"},
			},
			expected: false,
		},
		{
			name: "L risk - requires per-task review",
			task: &status.Task{
				ID:           "task-2",
				Description:  "implement authentication",
				Deliverables: []string{"auth/handler.go"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test files
			for _, deliverable := range tt.task.Deliverables {
				path := filepath.Join(tmpDir, deliverable)
				os.MkdirAll(filepath.Dir(path), 0755)

				// Create file with enough content for risk calculation
				content := "package test\n"
				for i := 0; i < 100; i++ {
					content += "// line " + string(rune(i)) + "\n"
				}
				os.WriteFile(path, []byte(content), 0644)
			}

			result := engine.ShouldTriggerPerTaskReview(tt.task)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v for task %s", tt.expected, result, tt.name)
			}
		})
	}
}

func TestShouldTriggerBatchReview(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	tests := []struct {
		name     string
		tasks    []*status.Task
		expected bool
	}{
		{
			name:     "Empty tasks - no batch review",
			tasks:    []*status.Task{},
			expected: false,
		},
		{
			name: "All low risk - trigger batch review",
			tasks: []*status.Task{
				{
					ID:           "task-1",
					Description:  "fix typo",
					Deliverables: []string{"doc.md"},
				},
				{
					ID:           "task-2",
					Description:  "update comment",
					Deliverables: []string{"code.go"},
				},
			},
			expected: true,
		},
		{
			name: "All already reviewed - no batch review",
			tasks: []*status.Task{
				{
					ID:           "task-1",
					Description:  "fix typo",
					Deliverables: []string{"doc.md"},
					TestsStatus:  stringPtr("reviewed"),
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test files
			for _, task := range tt.tasks {
				for _, deliverable := range task.Deliverables {
					path := filepath.Join(tmpDir, deliverable)
					os.MkdirAll(filepath.Dir(path), 0755)
					os.WriteFile(path, []byte("test content\n"), 0644)
				}
			}

			result := engine.ShouldTriggerBatchReview(tt.tasks)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v for %s", tt.expected, result, tt.name)
			}
		})
	}
}

func TestReviewTask(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	// Create test file
	testFile := filepath.Join(tmpDir, "test.go")
	testContent := `package main

func main() {
	password := "hardcoded123"
	println(password)
}
`
	os.WriteFile(testFile, []byte(testContent), 0644)

	task := &status.Task{
		ID:           "task-1",
		Description:  "implement feature",
		Deliverables: []string{"test.go"},
	}

	result, err := engine.ReviewTask(task)
	if err != nil {
		t.Fatalf("ReviewTask failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.TaskID != task.ID {
		t.Errorf("Expected task ID %s, got %s", task.ID, result.TaskID)
	}

	if result.ReviewType != ReviewTypePerTask {
		t.Errorf("Expected review type %s, got %s", ReviewTypePerTask, result.ReviewType)
	}

	if len(result.PersonaResults) == 0 {
		t.Error("Expected at least one persona result")
	}

	// Should detect hardcoded password (security issue)
	foundSecurityIssue := false
	for _, personaResult := range result.PersonaResults {
		if personaResult.Persona == PersonaSecurity && len(personaResult.Issues) > 0 {
			foundSecurityIssue = true
			break
		}
	}

	if !foundSecurityIssue {
		t.Error("Expected security persona to detect hardcoded password")
	}
}

func TestReviewBatch(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	// Create test files
	file1 := filepath.Join(tmpDir, "file1.go")
	file2 := filepath.Join(tmpDir, "file2.go")

	os.WriteFile(file1, []byte("package main\n// simple file\n"), 0644)
	os.WriteFile(file2, []byte("package main\n// another file\n"), 0644)

	tasks := []*status.Task{
		{
			ID:           "task-1",
			Description:  "fix bug",
			Deliverables: []string{"file1.go"},
		},
		{
			ID:           "task-2",
			Description:  "update docs",
			Deliverables: []string{"file2.go"},
		},
	}

	result, err := engine.ReviewBatch(tasks)
	if err != nil {
		t.Fatalf("ReviewBatch failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	if result.ReviewType != ReviewTypeBatch {
		t.Errorf("Expected review type %s, got %s", ReviewTypeBatch, result.ReviewType)
	}

	// Batch reviews should have fewer personas (efficiency)
	if len(result.PersonaResults) > 3 {
		t.Errorf("Batch review should use limited personas, got %d", len(result.PersonaResults))
	}
}

func TestSelectPersonasForTask(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	tests := []struct {
		name        string
		riskLevel   RiskLevel
		minPersonas int
		maxPersonas int
	}{
		{
			name:        "XS risk - minimal personas",
			riskLevel:   RiskLevelXS,
			minPersonas: 3,
			maxPersonas: 3,
		},
		{
			name:        "M risk - includes UX",
			riskLevel:   RiskLevelM,
			minPersonas: 4,
			maxPersonas: 4,
		},
		{
			name:        "L risk - includes reliability",
			riskLevel:   RiskLevelL,
			minPersonas: 5,
			maxPersonas: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &status.Task{ID: "test"}
			personas := engine.selectPersonasForTask(task, tt.riskLevel)

			if len(personas) < tt.minPersonas {
				t.Errorf("Expected at least %d personas, got %d", tt.minPersonas, len(personas))
			}

			if len(personas) > tt.maxPersonas {
				t.Errorf("Expected at most %d personas, got %d", tt.maxPersonas, len(personas))
			}
		})
	}
}

func TestCalculateMetrics(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	issues := []ReviewIssue{
		{Persona: PersonaSecurity, Severity: SeverityP0, Message: "Critical security issue"},
		{Persona: PersonaSecurity, Severity: SeverityP1, Message: "High security issue"},
		{Persona: PersonaPerformance, Severity: SeverityP2, Message: "Medium perf issue"},
		{Persona: PersonaMaintainability, Severity: SeverityP3, Message: "Low maint issue"},
	}

	metrics := engine.calculateMetrics(issues)

	if metrics.TotalIssues != 4 {
		t.Errorf("Expected 4 total issues, got %d", metrics.TotalIssues)
	}

	if metrics.P0Issues != 1 {
		t.Errorf("Expected 1 P0 issue, got %d", metrics.P0Issues)
	}

	if metrics.P1Issues != 1 {
		t.Errorf("Expected 1 P1 issue, got %d", metrics.P1Issues)
	}

	if metrics.P2Issues != 1 {
		t.Errorf("Expected 1 P2 issue, got %d", metrics.P2Issues)
	}

	if metrics.P3Issues != 1 {
		t.Errorf("Expected 1 P3 issue, got %d", metrics.P3Issues)
	}

	// Security score should be reduced due to issues
	if metrics.SecurityScore >= 100.0 {
		t.Errorf("Expected security score < 100 due to issues, got %.1f", metrics.SecurityScore)
	}
}

func TestExtractBlockingIssues(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	issues := []ReviewIssue{
		{Severity: SeverityP0, Message: "Critical"},
		{Severity: SeverityP1, Message: "High"},
		{Severity: SeverityP2, Message: "Medium"},
		{Severity: SeverityP3, Message: "Low"},
	}

	tests := []struct {
		name      string
		riskLevel RiskLevel
		expected  int
	}{
		{
			name:      "M risk - P0 and P1 block",
			riskLevel: RiskLevelM,
			expected:  2,
		},
		{
			name:      "XL risk - P0, P1, P2 block",
			riskLevel: RiskLevelXL,
			expected:  3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocking := engine.extractBlockingIssues(issues, tt.riskLevel)

			if len(blocking) != tt.expected {
				t.Errorf("Expected %d blocking issues, got %d", tt.expected, len(blocking))
			}
		})
	}
}

func TestIsBlockingIssue(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	tests := []struct {
		name      string
		severity  IssueSeverity
		riskLevel RiskLevel
		expected  bool
	}{
		{"P0 always blocks", SeverityP0, RiskLevelXS, true},
		{"P1 always blocks", SeverityP1, RiskLevelS, true},
		{"P2 blocks XL only", SeverityP2, RiskLevelXL, true},
		{"P2 doesn't block M", SeverityP2, RiskLevelM, false},
		{"P3 never blocks", SeverityP3, RiskLevelXL, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			issue := ReviewIssue{Severity: tt.severity}
			result := engine.isBlockingIssue(issue, tt.riskLevel)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCalculateAggregateScore(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	results := []PersonaResult{
		{Score: 100.0},
		{Score: 80.0},
		{Score: 90.0},
	}

	score := engine.calculateAggregateScore(results)
	expected := 90.0

	if score != expected {
		t.Errorf("Expected aggregate score %.1f, got %.1f", expected, score)
	}
}

func TestCalculatePersonaScore(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	tests := []struct {
		name     string
		issues   []ReviewIssue
		minScore float64
		maxScore float64
	}{
		{
			name:     "No issues - perfect score",
			issues:   []ReviewIssue{},
			minScore: 100.0,
			maxScore: 100.0,
		},
		{
			name: "One P0 - major penalty",
			issues: []ReviewIssue{
				{Severity: SeverityP0},
			},
			minScore: 0.0,
			maxScore: 75.0,
		},
		{
			name: "Multiple P3 - minor penalty",
			issues: []ReviewIssue{
				{Severity: SeverityP3},
				{Severity: SeverityP3},
			},
			minScore: 90.0,
			maxScore: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := engine.calculatePersonaScore(tt.issues)

			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("Expected score between %.1f and %.1f, got %.1f",
					tt.minScore, tt.maxScore, score)
			}
		})
	}
}

func TestHasGoFiles(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	tests := []struct {
		name     string
		files    []string
		expected bool
	}{
		{
			name:     "Contains Go files",
			files:    []string{"main.go", "test.go"},
			expected: true,
		},
		{
			name:     "No Go files",
			files:    []string{"readme.md", "config.json"},
			expected: false,
		},
		{
			name:     "Mixed files with Go",
			files:    []string{"readme.md", "handler.go", "config.json"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.hasGoFiles(tt.files)

			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCollectRecommendations(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	results := []PersonaResult{
		{
			Issues: []ReviewIssue{
				{Suggestion: "Use prepared statements"},
				{Suggestion: "Add input validation"},
			},
		},
		{
			Issues: []ReviewIssue{
				{Suggestion: "Use prepared statements"}, // Duplicate
				{Suggestion: "Optimize query"},
			},
		},
	}

	recommendations := engine.collectRecommendations(results)

	// Should have 3 unique recommendations
	if len(recommendations) != 3 {
		t.Errorf("Expected 3 unique recommendations, got %d", len(recommendations))
	}

	// Check for duplicates
	seen := make(map[string]bool)
	for _, rec := range recommendations {
		if seen[rec] {
			t.Errorf("Found duplicate recommendation: %s", rec)
		}
		seen[rec] = true
	}
}

func TestSaveReportToFile(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	result := &ReviewResult{
		TaskID:         "task-1",
		RiskLevel:      RiskLevelM,
		ReviewType:     ReviewTypePerTask,
		Timestamp:      time.Now(),
		AggregateScore: 85.5,
		Passed:         true,
		Metrics: ReviewMetrics{
			TotalIssues: 2,
			P2Issues:    2,
		},
	}

	outputPath := filepath.Join(tmpDir, "review-report.txt")
	err := engine.SaveReportToFile(result, outputPath)

	if err != nil {
		t.Fatalf("SaveReportToFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Report file was not created")
	}

	// Verify file content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read report file: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "task-1") {
		t.Error("Report should contain task ID")
	}

	if !strings.Contains(contentStr, "PASSED") {
		t.Error("Report should contain PASSED status")
	}
}

func TestSaveReportJSON(t *testing.T) {
	tmpDir := t.TempDir()
	st := &status.StatusV2{ProjectName: "test"}
	engine := NewReviewEngine(tmpDir, st)

	result := &ReviewResult{
		TaskID:         "task-1",
		RiskLevel:      RiskLevelM,
		ReviewType:     ReviewTypePerTask,
		Timestamp:      time.Now(),
		AggregateScore: 85.5,
		Passed:         true,
	}

	outputPath := filepath.Join(tmpDir, "review-result.json")
	err := engine.SaveReportJSON(result, outputPath)

	if err != nil {
		t.Fatalf("SaveReportJSON failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("JSON file was not created")
	}

	// Verify valid JSON
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read JSON file: %v", err)
	}

	var parsed ReviewResult
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Errorf("Failed to parse JSON: %v", err)
	}

	if parsed.TaskID != "task-1" {
		t.Errorf("Expected task ID 'task-1', got '%s'", parsed.TaskID)
	}
}

// Helper functions

func stringPtr(s string) *string {
	return &s
}
