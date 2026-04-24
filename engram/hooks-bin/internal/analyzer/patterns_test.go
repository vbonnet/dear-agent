package analyzer

import (
	"testing"
	"time"
)

func makeTestDenials() []ClassifiedDenial {
	now := time.Now()
	return []ClassifiedDenial{
		// Pattern "cd command" — 3 FPs, 1 TP
		{
			Denial:          DenialEntry{Timestamp: now, Command: "git -C /path status", PatternName: "cd command", PatternIndex: 2},
			Outcome:         OutcomeRetrySuccess,
			IsFalsePositive: true,
			Confidence:      0.9,
			WastedCalls:     2,
		},
		{
			Denial:          DenialEntry{Timestamp: now, Command: "go -C /path test", PatternName: "cd command", PatternIndex: 2},
			Outcome:         OutcomeRetrySuccess,
			IsFalsePositive: true,
			Confidence:      0.85,
			WastedCalls:     1,
		},
		{
			Denial:          DenialEntry{Timestamp: now, Command: "make -C /path build", PatternName: "cd command", PatternIndex: 2},
			Outcome:         OutcomeSwitchedTool,
			IsFalsePositive: true,
			Confidence:      0.8,
			WastedCalls:     1,
		},
		{
			Denial:          DenialEntry{Timestamp: now, Command: "cd /tmp && ls", PatternName: "cd command", PatternIndex: 2},
			Outcome:         OutcomeRetrySuccess,
			IsFalsePositive: false,
			Confidence:      0.95,
		},
		// Pattern "command chaining (&&)" — 1 FP, 2 TPs
		{
			Denial:          DenialEntry{Timestamp: now, Command: "echo hello && echo world", PatternName: "command chaining (&&)", PatternIndex: 3},
			Outcome:         OutcomeRetrySuccess,
			IsFalsePositive: false,
			Confidence:      0.95,
		},
		{
			Denial:          DenialEntry{Timestamp: now, Command: "make && make test", PatternName: "command chaining (&&)", PatternIndex: 3},
			Outcome:         OutcomeRetrySuccess,
			IsFalsePositive: false,
			Confidence:      0.9,
		},
		{
			Denial:          DenialEntry{Timestamp: now, Command: "npm install", PatternName: "command chaining (&&)", PatternIndex: 3},
			Outcome:         OutcomeRetrySuccess,
			IsFalsePositive: true,
			Confidence:      0.7,
			WastedCalls:     1,
		},
		// Unknown outcome denial.
		{
			Denial:          DenialEntry{Timestamp: now, Command: "something", PatternName: "cd command", PatternIndex: 2},
			Outcome:         OutcomeUnknown,
			IsFalsePositive: false,
			Confidence:      0.0,
		},
	}
}

func TestAnalyzePatterns_Grouping(t *testing.T) {
	denials := makeTestDenials()
	results := AnalyzePatterns(denials)

	if len(results) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(results))
	}

	// Results should be sorted by FP rate descending.
	// "cd command": 3 FP / 5 total = 0.6
	// "command chaining (&&)": 1 FP / 3 total = 0.333...
	first := results[0]
	if first.PatternName != "cd command" {
		t.Errorf("expected first pattern 'cd command', got %q", first.PatternName)
	}
	if first.TotalDenials != 5 {
		t.Errorf("expected 5 total denials for cd command, got %d", first.TotalDenials)
	}
	if first.FalsePositives != 3 {
		t.Errorf("expected 3 FPs, got %d", first.FalsePositives)
	}
	if first.TruePositives != 1 {
		t.Errorf("expected 1 TP, got %d", first.TruePositives)
	}
	if first.Uncertain != 1 {
		t.Errorf("expected 1 uncertain, got %d", first.Uncertain)
	}

	second := results[1]
	if second.PatternName != "command chaining (&&)" {
		t.Errorf("expected second pattern 'command chaining (&&)', got %q", second.PatternName)
	}
	if second.FalsePositives != 1 {
		t.Errorf("expected 1 FP for chaining, got %d", second.FalsePositives)
	}
	if second.TruePositives != 2 {
		t.Errorf("expected 2 TPs for chaining, got %d", second.TruePositives)
	}
}

func TestAnalyzePatterns_FPRate(t *testing.T) {
	denials := makeTestDenials()
	results := AnalyzePatterns(denials)

	// cd command: 3/5 = 0.6
	cd := results[0]
	expectedRate := 3.0 / 5.0
	if cd.FalsePositiveRate != expectedRate {
		t.Errorf("expected FP rate %f, got %f", expectedRate, cd.FalsePositiveRate)
	}

	// Sorted descending: first should have higher FP rate.
	if results[0].FalsePositiveRate < results[1].FalsePositiveRate {
		t.Error("results not sorted by FP rate descending")
	}
}

func TestAnalyzePatterns_ExampleLimits(t *testing.T) {
	// Create 10 FPs for one pattern.
	now := time.Now()
	var denials []ClassifiedDenial
	for i := 0; i < 10; i++ {
		denials = append(denials, ClassifiedDenial{
			Denial:          DenialEntry{Timestamp: now, Command: "cmd", PatternName: "test-pattern"},
			Outcome:         OutcomeRetrySuccess,
			IsFalsePositive: true,
			Confidence:      0.9,
		})
	}
	// Add 5 TPs.
	for i := 0; i < 5; i++ {
		denials = append(denials, ClassifiedDenial{
			Denial:          DenialEntry{Timestamp: now, Command: "bad-cmd", PatternName: "test-pattern"},
			Outcome:         OutcomeRetryDenied,
			IsFalsePositive: false,
			Confidence:      0.9,
		})
	}

	results := AnalyzePatterns(denials)
	if len(results) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(results))
	}
	if len(results[0].ExampleFPs) != 5 {
		t.Errorf("expected max 5 example FPs, got %d", len(results[0].ExampleFPs))
	}
	if len(results[0].ExampleTPs) != 3 {
		t.Errorf("expected max 3 example TPs, got %d", len(results[0].ExampleTPs))
	}
}

func TestAnalyzePatterns_Empty(t *testing.T) {
	results := AnalyzePatterns(nil)
	if len(results) != 0 {
		t.Errorf("expected 0 patterns for nil input, got %d", len(results))
	}
}

func TestProposePatternFixes_NoFixNeeded(t *testing.T) {
	// Low FP rate, should not propose fix.
	analyses := []PatternAnalysis{
		{
			PatternName:       "test",
			PatternRegex:      `\btest\b`,
			TotalDenials:      10,
			FalsePositives:    1,
			TruePositives:     9,
			FalsePositiveRate: 0.1,
		},
	}
	result := ProposePatternFixes(analyses, nil)
	if result[0].ProposedFix != nil {
		t.Error("should not propose fix for low FP rate pattern")
	}
}

func TestTryWordBoundaryPrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`\bcd\s+`, `(^|\s)cd\s+`},
		{`&&`, `&&`}, // no \b prefix, unchanged
		{`\b(cat|cp)\b`, `(^|\s)(cat|cp)\b`},
	}
	for _, tt := range tests {
		got := tryWordBoundaryPrefix(tt.input)
		if got != tt.expected {
			t.Errorf("tryWordBoundaryPrefix(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
