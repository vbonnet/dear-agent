package outputformatter

import (
	"strings"
	"testing"
)

// Mock result for testing
type mockResult struct {
	status   StatusLevel
	message  string
	category string
}

func (m mockResult) Status() StatusLevel { return m.status }
func (m mockResult) Message() string     { return m.message }
func (m mockResult) Category() string    { return m.category }

func TestSummaryGenerator_Generate(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    Summary
	}{
		{
			name:    "empty results",
			results: []Result{},
			want:    Summary{Total: 0},
		},
		{
			name: "only passed results",
			results: []Result{
				mockResult{StatusOK, "Check 1", "core"},
				mockResult{StatusSuccess, "Check 2", "core"},
			},
			want: Summary{Total: 2, Passed: 2},
		},
		{
			name: "mixed results",
			results: []Result{
				mockResult{StatusOK, "Check 1", "core"},
				mockResult{StatusWarning, "Check 2", "deps"},
				mockResult{StatusError, "Check 3", "hooks"},
				mockResult{StatusInfo, "Check 4", "core"},
			},
			want: Summary{Total: 4, Passed: 1, Warnings: 1, Errors: 1, Info: 1},
		},
		{
			name: "status aliases",
			results: []Result{
				mockResult{StatusSuccess, "Check 1", "core"}, // Alias for OK
				mockResult{StatusFailed, "Check 2", "deps"},  // Alias for Error
			},
			want: Summary{Total: 2, Passed: 1, Errors: 1},
		},
		{
			name: "unknown status",
			results: []Result{
				mockResult{StatusUnknown, "Check 1", "core"},
			},
			want: Summary{Total: 1, Unknown: 1},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewSummaryGenerator(NewIconMapper(false))
			got := gen.Generate(tt.results)
			if got != tt.want {
				t.Errorf("Generate() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestSummaryGenerator_Format(t *testing.T) {
	tests := []struct {
		name         string
		summary      Summary
		noColor      bool
		wantContains []string
	}{
		{
			name:    "emoji format with all statuses",
			summary: Summary{Passed: 5, Info: 2, Warnings: 3, Errors: 1},
			noColor: false,
			wantContains: []string{
				"✅ 5 checks passed",
				"ℹ️  2 info",
				"⚠️  3 warnings",
				"❌ 1 errors",
			},
		},
		{
			name:    "plain text format",
			summary: Summary{Passed: 3, Warnings: 1},
			noColor: true,
			wantContains: []string{
				"[OK] 3 checks passed",
				"[WARN] 1 warnings",
			},
		},
		{
			name:    "empty summary",
			summary: Summary{},
			noColor: false,
			wantContains: []string{
				"No results",
			},
		},
		{
			name:    "only passed",
			summary: Summary{Passed: 10},
			noColor: false,
			wantContains: []string{
				"✅ 10 checks passed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewSummaryGenerator(NewIconMapper(tt.noColor))
			got := gen.Format(tt.summary)
			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("Format() missing %q in output:\n%s", want, got)
				}
			}
		})
	}
}

func TestSummaryGenerator_FormatCompact(t *testing.T) {
	tests := []struct {
		name    string
		summary Summary
		want    string
	}{
		{
			name:    "all statuses",
			summary: Summary{Passed: 5, Info: 2, Warnings: 3, Errors: 1, Unknown: 1},
			want:    "5 passed, 2 info, 3 warnings, 1 errors, 1 unknown",
		},
		{
			name:    "only passed",
			summary: Summary{Passed: 10},
			want:    "10 passed",
		},
		{
			name:    "passed and warnings",
			summary: Summary{Passed: 8, Warnings: 2},
			want:    "8 passed, 2 warnings",
		},
		{
			name:    "empty summary",
			summary: Summary{},
			want:    "no results",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := NewSummaryGenerator(NewIconMapper(false))
			got := gen.FormatCompact(tt.summary)
			if got != tt.want {
				t.Errorf("FormatCompact() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetIssues(t *testing.T) {
	tests := []struct {
		name    string
		results []Result
		want    int // Number of issues expected
	}{
		{
			name: "no issues",
			results: []Result{
				mockResult{StatusOK, "Check 1", "core"},
				mockResult{StatusInfo, "Check 2", "core"},
			},
			want: 0,
		},
		{
			name: "only warnings",
			results: []Result{
				mockResult{StatusWarning, "Check 1", "core"},
				mockResult{StatusWarning, "Check 2", "deps"},
			},
			want: 2,
		},
		{
			name: "only errors",
			results: []Result{
				mockResult{StatusError, "Check 1", "core"},
				mockResult{StatusFailed, "Check 2", "deps"}, // Alias
			},
			want: 2,
		},
		{
			name: "mixed with issues",
			results: []Result{
				mockResult{StatusOK, "Check 1", "core"},
				mockResult{StatusWarning, "Check 2", "deps"},
				mockResult{StatusInfo, "Check 3", "core"},
				mockResult{StatusError, "Check 4", "hooks"},
			},
			want: 2, // Only warning and error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetIssues(tt.results)
			if len(got) != tt.want {
				t.Errorf("GetIssues() returned %d issues, want %d", len(got), tt.want)
			}
			// Verify all returned results are actually issues
			for _, issue := range got {
				status := issue.Status()
				if status != StatusWarning && status != StatusError && status != StatusFailed {
					t.Errorf("GetIssues() returned non-issue status %q", status)
				}
			}
		})
	}
}
