package validation

import (
	"strings"
	"testing"
)

func TestParseCoverage(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    float64
		wantErr bool
	}{
		{
			name:    "standard coverage output",
			output:  "ok  \tpkg/test\t0.005s\tcoverage: 85.3% of statements",
			want:    85.3,
			wantErr: false,
		},
		{
			name:    "100% coverage",
			output:  "coverage: 100.0% of statements",
			want:    100.0,
			wantErr: false,
		},
		{
			name:    "low coverage",
			output:  "coverage: 12.5% of statements",
			want:    12.5,
			wantErr: false,
		},
		{
			name:    "no coverage in output",
			output:  "ok  \tpkg/test\t0.005s",
			want:    0,
			wantErr: true,
		},
		{
			name:    "malformed coverage",
			output:  "coverage: abc% of statements",
			want:    0,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCoverage(tt.output)

			if tt.wantErr && err == nil {
				t.Error("expected error but got nil")
			}

			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("coverage: got %.1f, want %.1f", got, tt.want)
			}
		})
	}
}

func TestParseTestFailures(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantLen int
	}{
		{
			name: "test failures",
			output: `--- FAIL: TestFoo (0.00s)
    foo_test.go:10: got 1, want 2
FAIL
FAIL	pkg/test	0.005s`,
			wantLen: 1, // Only --- FAIL: line (FAIL summary lines excluded)
		},
		{
			name: "panic",
			output: `panic: runtime error: index out of range
goroutine 1 [running]:
FAIL	pkg/test	0.001s`,
			wantLen: 1, // Only panic: line (FAIL summary line excluded)
		},
		{
			name:    "all tests pass",
			output:  "ok  \tpkg/test\t0.005s",
			wantLen: 0,
		},
		{
			name: "error message",
			output: `--- FAIL: TestBar (0.00s)
    Error: expected value not found
FAIL	pkg/test	0.003s`,
			wantLen: 2, // --- FAIL: and Error: (FAIL summary line excluded)
		},
		{
			name: "multiple test failures",
			output: `--- FAIL: TestOne (0.00s)
--- FAIL: TestTwo (0.00s)
FAIL	pkg/test	0.005s`,
			wantLen: 2, // Both --- FAIL: lines
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			failures := parseTestFailures(tt.output)

			if len(failures) != tt.wantLen {
				t.Errorf("failures count: got %d, want %d\nFailures: %v", len(failures), tt.wantLen, failures)
			}
		})
	}
}

func TestValidateS9_Integration(t *testing.T) {
	// This is an integration test that validates a real package
	// Use the internal config package with absolute path
	result, err := ValidateS9("../../internal/config", 50.0) // Low threshold for this test

	if err != nil {
		t.Fatalf("ValidateS9 failed: %v", err)
	}

	// If tests didn't run (package not found), skip test
	if !result.TestsPassed && len(result.Failures) > 0 && strings.Contains(result.Failures[0], "setup failed") {
		t.Skip("config package not available for integration test")
	}

	// Tests should pass for config package
	if !result.TestsPassed {
		t.Errorf("expected tests to pass, got failures: %v", result.Failures)
	}

	// Coverage should be present when tests pass
	if result.TestsPassed && result.Coverage == 0 {
		t.Error("expected non-zero coverage when tests pass")
	}

	// Valid if tests pass and coverage meets threshold
	expectedValid := result.TestsPassed && result.CoverageMet
	if result.Valid != expectedValid {
		t.Errorf("valid: got %v, want %v", result.Valid, expectedValid)
	}
}

func TestValidateS9_CoverageThreshold(t *testing.T) {
	// First check if we can run tests on config package
	checkResult, err := ValidateS9("../../internal/config", 1.0)
	if err != nil {
		t.Fatalf("ValidateS9 check failed: %v", err)
	}

	// Skip if package not available
	if !checkResult.TestsPassed && len(checkResult.Failures) > 0 && strings.Contains(checkResult.Failures[0], "setup failed") {
		t.Skip("config package not available for integration test")
	}

	tests := []struct {
		name      string
		threshold float64
		wantMet   bool // Whether we expect coverage to meet threshold
	}{
		{"very low threshold", 1.0, true},
		{"medium threshold", 50.0, true},
		{"impossible threshold", 999.0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use config package for testing
			result, err := ValidateS9("../../internal/config", tt.threshold)

			if err != nil {
				t.Fatalf("ValidateS9 failed: %v", err)
			}

			// Skip if package not available
			if !result.TestsPassed && len(result.Failures) > 0 && strings.Contains(result.Failures[0], "setup failed") {
				t.Skip("config package not available")
			}

			if result.CoverageMet != tt.wantMet {
				t.Errorf("coverage met: got %v, want %v (coverage: %.1f%%, threshold: %.1f%%)",
					result.CoverageMet, tt.wantMet, result.Coverage, tt.threshold)
			}

			if result.CoverageThreshold != tt.threshold {
				t.Errorf("threshold: got %.1f, want %.1f", result.CoverageThreshold, tt.threshold)
			}

			// Valid only if tests pass AND coverage meets threshold
			expectedValid := result.TestsPassed && result.CoverageMet
			if result.Valid != expectedValid {
				t.Errorf("valid: got %v, want %v", result.Valid, expectedValid)
			}
		})
	}
}

func TestValidateS9_NonexistentPackage(t *testing.T) {
	result, err := ValidateS9("./nonexistent/package", 80.0)

	if err != nil {
		t.Fatalf("ValidateS9 failed: %v", err)
	}

	if result.Valid {
		t.Error("expected invalid result for nonexistent package")
	}

	if result.TestsPassed {
		t.Error("expected tests to fail for nonexistent package")
	}

	if len(result.Failures) == 0 {
		t.Error("expected failure messages")
	}
}
