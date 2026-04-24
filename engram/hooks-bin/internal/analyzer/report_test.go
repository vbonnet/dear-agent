package analyzer

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func makeTestReport() Report {
	now := time.Now()
	stats := HookLogStats{
		TotalInvocations: 100,
		TotalDenials:     40,
		TotalApprovals:   60,
		DenialsByPattern: map[string]int{
			"cd command":            20,
			"command chaining (&&)": 20,
		},
		TimeRange:        [2]time.Time{now.Add(-24 * time.Hour), now},
		UniqueSessionIDs: 5,
	}

	patterns := []PatternAnalysis{
		{
			PatternName:       "cd command",
			PatternIndex:      2,
			PatternRegex:      `\bcd\s+`,
			TotalDenials:      20,
			FalsePositives:    15,
			TruePositives:     5,
			FalsePositiveRate: 0.75,
			ExampleFPs: []ClassifiedDenial{
				{
					Denial:          DenialEntry{Command: "git -C /path status"},
					IsFalsePositive: true,
					WastedCalls:     2,
				},
				{
					Denial:          DenialEntry{Command: "go -C /path test"},
					IsFalsePositive: true,
					WastedCalls:     1,
				},
			},
		},
		{
			PatternName:       "command chaining (&&)",
			PatternIndex:      3,
			PatternRegex:      `&&`,
			TotalDenials:      20,
			FalsePositives:    2,
			TruePositives:     18,
			FalsePositiveRate: 0.1,
		},
	}

	return GenerateReport(stats, patterns, "/tmp/test.log", 500)
}

func TestGenerateReport_Fields(t *testing.T) {
	r := makeTestReport()

	if r.LogPath != "/tmp/test.log" {
		t.Errorf("expected log path /tmp/test.log, got %s", r.LogPath)
	}
	if r.LogLineCount != 500 {
		t.Errorf("expected 500 lines, got %d", r.LogLineCount)
	}
	if r.Stats.TotalDenials != 40 {
		t.Errorf("expected 40 denials, got %d", r.Stats.TotalDenials)
	}
	if len(r.Patterns) != 2 {
		t.Errorf("expected 2 patterns, got %d", len(r.Patterns))
	}
}

func TestGenerateReport_OverallFPRate(t *testing.T) {
	r := makeTestReport()
	// 15 + 2 = 17 FPs, 5 + 18 = 23 TPs => 17 / (17+23) = 0.425
	expected := 17.0 / 40.0
	if r.OverallFPRate != expected {
		t.Errorf("expected overall FP rate %f, got %f", expected, r.OverallFPRate)
	}
}

func TestGenerateReport_WastedCalls(t *testing.T) {
	r := makeTestReport()
	// 2 + 1 = 3 from ExampleFPs
	if r.TotalWastedCalls != 3 {
		t.Errorf("expected 3 wasted calls, got %d", r.TotalWastedCalls)
	}
}

func TestFormatTextReport_NonEmpty(t *testing.T) {
	r := makeTestReport()
	text := FormatTextReport(r, 10)

	if text == "" {
		t.Fatal("expected non-empty text report")
	}
	if !strings.Contains(text, "Hook Analyzer Report") {
		t.Error("report should contain header")
	}
	if !strings.Contains(text, "cd command") {
		t.Error("report should contain pattern name")
	}
	if !strings.Contains(text, "git -C /path status") {
		t.Error("report should contain example FP command")
	}
	if !strings.Contains(text, "Total denials") {
		t.Error("report should contain summary stats")
	}
}

func TestFormatTextReport_TopN(t *testing.T) {
	r := makeTestReport()
	text := FormatTextReport(r, 1)

	// Should only show the first pattern.
	if !strings.Contains(text, "cd command") {
		t.Error("top 1 report should contain first pattern")
	}
	// The second pattern should not appear in the table (it may appear in stats).
	lines := strings.Split(text, "\n")
	tableFound := false
	for _, line := range lines {
		if strings.Contains(line, "command chaining") && strings.Contains(line, "20") {
			tableFound = true
		}
	}
	if tableFound {
		t.Error("top 1 report should not show second pattern in table")
	}
}

func TestFormatTextReport_Empty(t *testing.T) {
	r := Report{}
	text := FormatTextReport(r, 10)
	if !strings.Contains(text, "No patterns to display") {
		t.Error("empty report should say no patterns")
	}
}

func TestFormatJSONReport_ValidJSON(t *testing.T) {
	r := makeTestReport()
	data, err := FormatJSONReport(r)
	if err != nil {
		t.Fatalf("FormatJSONReport error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty JSON")
	}

	// Verify it's valid JSON by unmarshaling.
	var parsed Report
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if parsed.LogPath != r.LogPath {
		t.Errorf("JSON roundtrip: expected log path %s, got %s", r.LogPath, parsed.LogPath)
	}
	if len(parsed.Patterns) != 2 {
		t.Errorf("JSON roundtrip: expected 2 patterns, got %d", len(parsed.Patterns))
	}
}
