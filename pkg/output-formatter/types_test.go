package outputformatter

import "testing"

func TestSummary_IsHealthy(t *testing.T) {
	tests := []struct {
		name    string
		summary Summary
		want    bool
	}{
		{
			name:    "healthy with only passed",
			summary: Summary{Passed: 10},
			want:    true,
		},
		{
			name:    "healthy with passed and info",
			summary: Summary{Passed: 5, Info: 3},
			want:    true,
		},
		{
			name:    "not healthy with warnings",
			summary: Summary{Passed: 5, Warnings: 1},
			want:    false,
		},
		{
			name:    "not healthy with errors",
			summary: Summary{Passed: 5, Errors: 1},
			want:    false,
		},
		{
			name:    "not healthy with both warnings and errors",
			summary: Summary{Passed: 5, Warnings: 2, Errors: 1},
			want:    false,
		},
		{
			name:    "empty summary is healthy",
			summary: Summary{},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.summary.IsHealthy()
			if got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSummary_HasIssues(t *testing.T) {
	tests := []struct {
		name    string
		summary Summary
		want    bool
	}{
		{
			name:    "no issues with only passed",
			summary: Summary{Passed: 10},
			want:    false,
		},
		{
			name:    "has issues with warnings",
			summary: Summary{Passed: 5, Warnings: 1},
			want:    true,
		},
		{
			name:    "has issues with errors",
			summary: Summary{Errors: 1},
			want:    true,
		},
		{
			name:    "has issues with both",
			summary: Summary{Warnings: 2, Errors: 1},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.summary.HasIssues()
			if got != tt.want {
				t.Errorf("HasIssues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSummary_ExitCode(t *testing.T) {
	tests := []struct {
		name    string
		summary Summary
		want    int
	}{
		{
			name:    "exit 0 for healthy",
			summary: Summary{Passed: 10},
			want:    0,
		},
		{
			name:    "exit 1 for warnings",
			summary: Summary{Passed: 5, Warnings: 2},
			want:    1,
		},
		{
			name:    "exit 2 for errors",
			summary: Summary{Passed: 5, Errors: 1},
			want:    2,
		},
		{
			name:    "exit 2 for errors even with warnings",
			summary: Summary{Warnings: 5, Errors: 1},
			want:    2,
		},
		{
			name:    "exit 0 for info only",
			summary: Summary{Info: 5},
			want:    0,
		},
		{
			name:    "exit 0 for empty",
			summary: Summary{},
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.summary.ExitCode()
			if got != tt.want {
				t.Errorf("ExitCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSummary_OverallStatus(t *testing.T) {
	tests := []struct {
		name    string
		summary Summary
		want    string
	}{
		{
			name:    "healthy with passed",
			summary: Summary{Passed: 10},
			want:    "Healthy",
		},
		{
			name:    "healthy with info",
			summary: Summary{Info: 5},
			want:    "Healthy",
		},
		{
			name:    "degraded with warnings",
			summary: Summary{Passed: 5, Warnings: 2},
			want:    "Degraded",
		},
		{
			name:    "critical with errors",
			summary: Summary{Passed: 5, Errors: 1},
			want:    "Critical",
		},
		{
			name:    "critical even with warnings",
			summary: Summary{Warnings: 5, Errors: 1},
			want:    "Critical",
		},
		{
			name:    "unknown for empty",
			summary: Summary{},
			want:    "Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.summary.OverallStatus()
			if got != tt.want {
				t.Errorf("OverallStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
