package validate

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/manifest"
)

func TestReport_JSONSerialization(t *testing.T) {
	now := time.Now()
	report := &Report{
		ValidatedAt:   now,
		TotalSessions: 5,
		Resumable:     3,
		Failed:        2,
		Unknown:       0,
		Sessions: []SessionResult{
			{
				Name:   "test-session",
				UUID:   "abc123",
				Status: "resumable",
				Issues: nil,
				Fixed:  false,
			},
			{
				Name:   "failed-session",
				UUID:   "def456",
				Status: "failed",
				Issues: []Issue{
					{
						Type:        IssueVersionMismatch,
						Message:     "Version mismatch detected",
						Fix:         "Run agm admin doctor --validate --fix",
						AutoFixable: true,
					},
				},
				Fixed: false,
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Failed to marshal report: %v", err)
	}

	// Unmarshal back
	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal report: %v", err)
	}

	// Verify fields
	if decoded.TotalSessions != 5 {
		t.Errorf("Expected TotalSessions=5, got %d", decoded.TotalSessions)
	}
	if decoded.Resumable != 3 {
		t.Errorf("Expected Resumable=3, got %d", decoded.Resumable)
	}
	if decoded.Failed != 2 {
		t.Errorf("Expected Failed=2, got %d", decoded.Failed)
	}
	if len(decoded.Sessions) != 2 {
		t.Errorf("Expected 2 sessions, got %d", len(decoded.Sessions))
	}
}

func TestSessionResult_ManifestNotSerialized(t *testing.T) {
	m := &manifest.Manifest{
		Name: "test-session",
	}

	result := &SessionResult{
		Name:     "test-session",
		UUID:     "abc123",
		Status:   "resumable",
		Manifest: m, // Should not be serialized
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal session result: %v", err)
	}

	// Verify manifest is not in JSON
	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal to map: %v", err)
	}

	if _, exists := decoded["Manifest"]; exists {
		t.Error("Manifest should not be serialized (json:\"-\" tag)")
	}
}

func TestIssue_AllFields(t *testing.T) {
	issue := Issue{
		Type:        IssueCompactedJSONL,
		Message:     "JSONL file has summaries out of order",
		Fix:         "Reorder JSONL entries with summaries at end",
		AutoFixable: true,
	}

	data, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("Failed to marshal issue: %v", err)
	}

	var decoded Issue
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal issue: %v", err)
	}

	if decoded.Type != IssueCompactedJSONL {
		t.Errorf("Expected type=%s, got %s", IssueCompactedJSONL, decoded.Type)
	}
	if !decoded.AutoFixable {
		t.Error("Expected AutoFixable=true")
	}
}

func TestIssueType_Constants(t *testing.T) {
	tests := []struct {
		name      string
		issueType IssueType
		expected  string
	}{
		{"version mismatch", IssueVersionMismatch, "version_mismatch"},
		{"empty session env", IssueEmptySessionEnv, "empty_session_env"},
		{"compacted JSONL", IssueCompactedJSONL, "compacted_jsonl"},
		{"JSONL missing", IssueJSONLMissing, "jsonl_missing"},
		{"cwd mismatch", IssueCwdMismatch, "cwd_mismatch"},
		{"lock contention", IssueLockContention, "lock_contention"},
		{"permissions", IssuePermissions, "permissions"},
		{"corrupted data", IssueCorruptedData, "corrupted_data"},
		{"missing dependency", IssueMissingDependency, "missing_dependency"},
		{"environment", IssueEnvironment, "environment"},
		{"session conflict", IssueSessionConflict, "session_conflict"},
		{"unknown", IssueUnknown, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.issueType) != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, string(tt.issueType))
			}
		})
	}
}

func TestOptions_DefaultValues(t *testing.T) {
	opts := &Options{}

	// Verify zero values
	if opts.AutoFix {
		t.Error("Expected AutoFix=false by default")
	}
	if opts.JSONOutput {
		t.Error("Expected JSONOutput=false by default")
	}
	if opts.TimeoutPerSession != 0 {
		t.Errorf("Expected TimeoutPerSession=0 by default, got %d", opts.TimeoutPerSession)
	}
}

func TestReport_EmptySessions(t *testing.T) {
	report := &Report{
		TotalSessions: 0,
		Resumable:     0,
		Failed:        0,
		Unknown:       0,
		Sessions:      []SessionResult{},
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("Failed to marshal empty report: %v", err)
	}

	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal empty report: %v", err)
	}

	if len(decoded.Sessions) != 0 {
		t.Errorf("Expected 0 sessions, got %d", len(decoded.Sessions))
	}
}

func TestSessionResult_OmitEmptyFields(t *testing.T) {
	// Session with no issues
	result := &SessionResult{
		Name:   "test",
		UUID:   "abc",
		Path:   "/path/to/session",
		Status: "resumable",
		// Issues is nil - should be omitted
		// Fixed is false - should be omitted
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	// Issues should be omitted when nil (omitempty tag)
	if _, exists := decoded["issues"]; exists {
		t.Error("Issues field should be omitted when nil")
	}

	// Fixed should be omitted when false (omitempty tag)
	if _, exists := decoded["fixed"]; exists {
		t.Error("Fixed field should be omitted when false")
	}
}

func TestReport_Invariants(t *testing.T) {
	now := time.Now()
	report := &Report{
		ValidatedAt:   now,
		TotalSessions: 5,
		Resumable:     3,
		Failed:        1,
		Unknown:       1,
		Sessions: []SessionResult{
			{Name: "s1", UUID: "1", Path: "/p1", Status: "resumable"},
			{Name: "s2", UUID: "2", Path: "/p2", Status: "resumable"},
			{Name: "s3", UUID: "3", Path: "/p3", Status: "resumable"},
			{Name: "s4", UUID: "4", Path: "/p4", Status: "failed"},
			{Name: "s5", UUID: "5", Path: "/p5", Status: "unknown"},
		},
	}

	// Sum must equal total
	sum := report.Resumable + report.Failed + report.Unknown
	if sum != report.TotalSessions {
		t.Errorf("Sum mismatch: %d + %d + %d = %d, want %d",
			report.Resumable, report.Failed, report.Unknown,
			sum, report.TotalSessions)
	}

	// Session count must match total
	if len(report.Sessions) != report.TotalSessions {
		t.Errorf("Session count mismatch: got %d, want %d",
			len(report.Sessions), report.TotalSessions)
	}
}

func TestReport_HasFailures(t *testing.T) {
	tests := []struct {
		name     string
		report   *Report
		expected bool
	}{
		{
			name: "all resumable",
			report: &Report{
				TotalSessions: 3,
				Resumable:     3,
				Failed:        0,
				Unknown:       0,
			},
			expected: false,
		},
		{
			name: "has failures",
			report: &Report{
				TotalSessions: 3,
				Resumable:     1,
				Failed:        2,
				Unknown:       0,
			},
			expected: true,
		},
		{
			name: "has unknown",
			report: &Report{
				TotalSessions: 3,
				Resumable:     2,
				Failed:        0,
				Unknown:       1,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.report.HasFailures(); got != tt.expected {
				t.Errorf("HasFailures() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestReport_SuccessRate(t *testing.T) {
	tests := []struct {
		name     string
		report   *Report
		expected float64
	}{
		{
			name: "100% success",
			report: &Report{
				TotalSessions: 5,
				Resumable:     5,
				Failed:        0,
				Unknown:       0,
			},
			expected: 100.0,
		},
		{
			name: "60% success",
			report: &Report{
				TotalSessions: 5,
				Resumable:     3,
				Failed:        2,
				Unknown:       0,
			},
			expected: 60.0,
		},
		{
			name: "zero sessions",
			report: &Report{
				TotalSessions: 0,
				Resumable:     0,
				Failed:        0,
				Unknown:       0,
			},
			expected: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.report.SuccessRate(); got != tt.expected {
				t.Errorf("SuccessRate() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIssueType_IsValid(t *testing.T) {
	validTypes := ValidIssueTypes()

	// Test all valid types
	for _, valid := range validTypes {
		if !valid.IsValid() {
			t.Errorf("IsValid() = false for valid type %s", valid)
		}
	}

	// Test invalid type
	invalid := IssueType("invalid_type")
	if invalid.IsValid() {
		t.Error("IsValid() = true for invalid type")
	}
}

func TestIssueType_String(t *testing.T) {
	it := IssueVersionMismatch
	if it.String() != "version_mismatch" {
		t.Errorf("String() = %s, want version_mismatch", it.String())
	}
}
