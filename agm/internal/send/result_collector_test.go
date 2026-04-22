package send

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGenerateReport_Empty(t *testing.T) {
	report := GenerateReport([]*DeliveryResult{})

	if report.TotalRecipients != 0 {
		t.Errorf("expected 0 total recipients, got %d", report.TotalRecipients)
	}

	if report.SuccessCount != 0 {
		t.Errorf("expected 0 successes, got %d", report.SuccessCount)
	}

	if report.FailureCount != 0 {
		t.Errorf("expected 0 failures, got %d", report.FailureCount)
	}
}

func TestGenerateReport_AllSuccess(t *testing.T) {
	results := []*DeliveryResult{
		{Recipient: "session1", Success: true, MessageID: "msg-001", Duration: 100 * time.Millisecond},
		{Recipient: "session2", Success: true, MessageID: "msg-002", Duration: 200 * time.Millisecond},
		{Recipient: "session3", Success: true, MessageID: "msg-003", Duration: 150 * time.Millisecond},
	}

	report := GenerateReport(results)

	if report.TotalRecipients != 3 {
		t.Errorf("expected 3 total recipients, got %d", report.TotalRecipients)
	}

	if report.SuccessCount != 3 {
		t.Errorf("expected 3 successes, got %d", report.SuccessCount)
	}

	if report.FailureCount != 0 {
		t.Errorf("expected 0 failures, got %d", report.FailureCount)
	}

	expectedDuration := 450 * time.Millisecond
	if report.TotalDuration != expectedDuration {
		t.Errorf("expected total duration %v, got %v", expectedDuration, report.TotalDuration)
	}
}

func TestGenerateReport_AllFailures(t *testing.T) {
	results := []*DeliveryResult{
		{Recipient: "session1", Success: false, Error: fmt.Errorf("error 1"), Duration: 50 * time.Millisecond},
		{Recipient: "session2", Success: false, Error: fmt.Errorf("error 2"), Duration: 75 * time.Millisecond},
	}

	report := GenerateReport(results)

	if report.TotalRecipients != 2 {
		t.Errorf("expected 2 total recipients, got %d", report.TotalRecipients)
	}

	if report.SuccessCount != 0 {
		t.Errorf("expected 0 successes, got %d", report.SuccessCount)
	}

	if report.FailureCount != 2 {
		t.Errorf("expected 2 failures, got %d", report.FailureCount)
	}
}

func TestGenerateReport_Mixed(t *testing.T) {
	results := []*DeliveryResult{
		{Recipient: "session1", Success: true, MessageID: "msg-001", Duration: 100 * time.Millisecond},
		{Recipient: "session2", Success: false, Error: fmt.Errorf("failed"), Duration: 50 * time.Millisecond},
		{Recipient: "session3", Success: true, MessageID: "msg-003", Duration: 150 * time.Millisecond},
	}

	report := GenerateReport(results)

	if report.TotalRecipients != 3 {
		t.Errorf("expected 3 total recipients, got %d", report.TotalRecipients)
	}

	if report.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", report.SuccessCount)
	}

	if report.FailureCount != 1 {
		t.Errorf("expected 1 failure, got %d", report.FailureCount)
	}
}

func TestGenerateReport_Sorting(t *testing.T) {
	results := []*DeliveryResult{
		{Recipient: "session3", Success: false, Error: fmt.Errorf("failed")},
		{Recipient: "session1", Success: true, MessageID: "msg-001"},
		{Recipient: "session2", Success: true, MessageID: "msg-002"},
	}

	report := GenerateReport(results)

	// Results should be sorted: successes first (alphabetically), then failures
	if report.Results[0].Recipient != "session1" {
		t.Errorf("expected first result 'session1', got '%s'", report.Results[0].Recipient)
	}

	if report.Results[1].Recipient != "session2" {
		t.Errorf("expected second result 'session2', got '%s'", report.Results[1].Recipient)
	}

	if report.Results[2].Recipient != "session3" {
		t.Errorf("expected third result 'session3', got '%s'", report.Results[2].Recipient)
	}
}

func TestGetFailedRecipients(t *testing.T) {
	report := &DeliveryReport{
		Results: []*DeliveryResult{
			{Recipient: "session1", Success: true},
			{Recipient: "session2", Success: false, Error: fmt.Errorf("failed")},
			{Recipient: "session3", Success: true},
			{Recipient: "session4", Success: false, Error: fmt.Errorf("failed")},
		},
	}

	failed := report.GetFailedRecipients()

	if len(failed) != 2 {
		t.Errorf("expected 2 failed recipients, got %d", len(failed))
	}

	// Build map for easier checking
	failedMap := make(map[string]bool)
	for _, r := range failed {
		failedMap[r] = true
	}

	if !failedMap["session2"] || !failedMap["session4"] {
		t.Errorf("expected session2 and session4, got %v", failed)
	}
}

func TestGetFailedRecipients_Empty(t *testing.T) {
	report := &DeliveryReport{
		Results: []*DeliveryResult{
			{Recipient: "session1", Success: true},
			{Recipient: "session2", Success: true},
		},
	}

	failed := report.GetFailedRecipients()

	if len(failed) != 0 {
		t.Errorf("expected 0 failed recipients, got %d", len(failed))
	}
}

func TestHasFailures(t *testing.T) {
	tests := []struct {
		name     string
		report   *DeliveryReport
		expected bool
	}{
		{
			name: "no failures",
			report: &DeliveryReport{
				SuccessCount: 3,
				FailureCount: 0,
			},
			expected: false,
		},
		{
			name: "has failures",
			report: &DeliveryReport{
				SuccessCount: 2,
				FailureCount: 1,
			},
			expected: true,
		},
		{
			name: "all failures",
			report: &DeliveryReport{
				SuccessCount: 0,
				FailureCount: 3,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if result := tt.report.HasFailures(); result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		expected string
	}{
		{10 * time.Millisecond, "10ms"},
		{100 * time.Millisecond, "100ms"},
		{500 * time.Millisecond, "500ms"},
		{1 * time.Second, "1.0s"},
		{1500 * time.Millisecond, "1.5s"},
		{2345 * time.Millisecond, "2.3s"},
		{10 * time.Second, "10.0s"},
	}

	for _, tt := range tests {
		result := formatDuration(tt.duration)
		if result != tt.expected {
			t.Errorf("formatDuration(%v) = %s, want %s", tt.duration, result, tt.expected)
		}
	}
}

func TestPrintReport_Empty(t *testing.T) {
	report := &DeliveryReport{}

	// Capture stdout
	output := captureOutput(func() {
		report.PrintReport()
	})

	if !strings.Contains(output, "No deliveries to report") {
		t.Errorf("expected 'No deliveries to report', got: %s", output)
	}
}

func TestPrintReport_AllSuccess(t *testing.T) {
	report := &DeliveryReport{
		TotalRecipients: 2,
		SuccessCount:    2,
		FailureCount:    0,
		Results: []*DeliveryResult{
			{Recipient: "session1", Success: true, MessageID: "msg-001", Duration: 100 * time.Millisecond},
			{Recipient: "session2", Success: true, MessageID: "msg-002", Duration: 200 * time.Millisecond},
		},
		TotalDuration: 300 * time.Millisecond,
	}

	output := captureOutput(func() {
		report.PrintReport()
	})

	// Should contain summary
	if !strings.Contains(output, "2 recipients") {
		t.Errorf("expected '2 recipients' in output")
	}

	if !strings.Contains(output, "2 succeeded, 0 failed") {
		t.Errorf("expected '2 succeeded, 0 failed' in output")
	}

	// Should contain success section
	if !strings.Contains(output, "Success") {
		t.Errorf("expected 'Success' section in output")
	}

	if !strings.Contains(output, "session1") {
		t.Errorf("expected 'session1' in output")
	}

	if !strings.Contains(output, "msg-001") {
		t.Errorf("expected 'msg-001' in output")
	}
}

func TestPrintReport_AllFailures(t *testing.T) {
	report := &DeliveryReport{
		TotalRecipients: 2,
		SuccessCount:    0,
		FailureCount:    2,
		Results: []*DeliveryResult{
			{Recipient: "session1", Success: false, Error: fmt.Errorf("connection failed"), Duration: 50 * time.Millisecond},
			{Recipient: "session2", Success: false, Error: fmt.Errorf("timeout"), Duration: 75 * time.Millisecond},
		},
		TotalDuration: 125 * time.Millisecond,
	}

	output := captureOutput(func() {
		report.PrintReport()
	})

	// Should contain summary
	if !strings.Contains(output, "0 succeeded, 2 failed") {
		t.Errorf("expected '0 succeeded, 2 failed' in output")
	}

	// Should contain failed section
	if !strings.Contains(output, "Failed") {
		t.Errorf("expected 'Failed' section in output")
	}

	if !strings.Contains(output, "connection failed") {
		t.Errorf("expected 'connection failed' in output")
	}

	if !strings.Contains(output, "timeout") {
		t.Errorf("expected 'timeout' in output")
	}
}

func TestPrintReport_Mixed(t *testing.T) {
	report := &DeliveryReport{
		TotalRecipients: 3,
		SuccessCount:    2,
		FailureCount:    1,
		Results: []*DeliveryResult{
			{Recipient: "session1", Success: true, MessageID: "msg-001", Duration: 100 * time.Millisecond},
			{Recipient: "session2", Success: false, Error: fmt.Errorf("failed"), Duration: 50 * time.Millisecond},
			{Recipient: "session3", Success: true, MessageID: "msg-003", Duration: 150 * time.Millisecond},
		},
		TotalDuration: 300 * time.Millisecond,
	}

	output := captureOutput(func() {
		report.PrintReport()
	})

	// Should contain both sections
	if !strings.Contains(output, "Success") {
		t.Errorf("expected 'Success' section in output")
	}

	if !strings.Contains(output, "Failed") {
		t.Errorf("expected 'Failed' section in output")
	}

	// Should show counts
	if !strings.Contains(output, "2 succeeded, 1 failed") {
		t.Errorf("expected '2 succeeded, 1 failed' in output")
	}
}

func TestPrintReport_SingularPlural(t *testing.T) {
	// Test singular form
	report1 := &DeliveryReport{
		TotalRecipients: 1,
		SuccessCount:    1,
		FailureCount:    0,
		Results: []*DeliveryResult{
			{Recipient: "session1", Success: true, MessageID: "msg-001", Duration: 100 * time.Millisecond},
		},
		TotalDuration: 100 * time.Millisecond,
	}

	output1 := captureOutput(func() {
		report1.PrintReport()
	})

	if !strings.Contains(output1, "1 recipient") && !strings.Contains(output1, "1 succeeded") {
		t.Errorf("expected singular form in output: %s", output1)
	}

	// Test plural form
	report2 := &DeliveryReport{
		TotalRecipients: 2,
		SuccessCount:    2,
		FailureCount:    0,
		Results: []*DeliveryResult{
			{Recipient: "session1", Success: true, MessageID: "msg-001", Duration: 100 * time.Millisecond},
			{Recipient: "session2", Success: true, MessageID: "msg-002", Duration: 100 * time.Millisecond},
		},
		TotalDuration: 200 * time.Millisecond,
	}

	output2 := captureOutput(func() {
		report2.PrintReport()
	})

	if !strings.Contains(output2, "2 recipients") {
		t.Errorf("expected plural form in output: %s", output2)
	}
}

// captureOutput captures stdout during function execution
func captureOutput(f func()) string {
	// Save original stdout
	origStdout := os.Stdout

	// Create pipe
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Run function
	f()

	// Close writer and restore stdout
	w.Close()
	os.Stdout = origStdout

	// Read captured output
	var buf bytes.Buffer
	io.Copy(&buf, r)

	return buf.String()
}
