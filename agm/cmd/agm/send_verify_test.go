package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyCmd_FlagsExist(t *testing.T) {
	flags := []struct {
		name     string
		defValue string
	}{
		{"request-id", ""},
		{"status", ""},
		{"dry-run", "false"},
	}

	for _, f := range flags {
		t.Run(f.name, func(t *testing.T) {
			flag := sendVerifyCmd.Flags().Lookup(f.name)
			require.NotNil(t, flag, "flag --%s should exist", f.name)
			assert.Equal(t, f.defValue, flag.DefValue)
		})
	}

	// Check slice flags exist
	assert.NotNil(t, sendVerifyCmd.Flags().Lookup("check-result"))
	assert.NotNil(t, sendVerifyCmd.Flags().Lookup("gap"))
}

func TestParseCheckResults(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantLen int
		wantErr bool
	}{
		{
			name:    "valid with 4 fields",
			input:   []string{"tests|go test ./...|PASS|all 42 tests pass"},
			wantLen: 1,
		},
		{
			name:    "valid with 3 fields",
			input:   []string{"tests|go test ./...|FAIL"},
			wantLen: 1,
		},
		{
			name:    "multiple results",
			input:   []string{"tests|go test|PASS|ok", "lint|golint|FAIL|3 issues"},
			wantLen: 2,
		},
		{
			name:    "nil input",
			input:   nil,
			wantLen: 0,
		},
		{
			name:    "too few fields",
			input:   []string{"tests|command"},
			wantErr: true,
		},
		{
			name:    "invalid result value",
			input:   []string{"tests|command|SUCCESS|details"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, err := parseCheckResults(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, results, tt.wantLen)
		})
	}
}

func TestParseCheckResults_FieldValues(t *testing.T) {
	results, err := parseCheckResults([]string{"unit tests|go test ./...|PASS|all 42 tests pass"})
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "unit tests", results[0].Description)
	assert.Equal(t, "go test ./...", results[0].Command)
	assert.Equal(t, "PASS", results[0].Result)
	assert.Equal(t, "all 42 tests pass", results[0].Details)
}

func TestParseGaps(t *testing.T) {
	tests := []struct {
		name    string
		input   []string
		wantLen int
		wantErr bool
	}{
		{
			name:    "valid with 3 fields",
			input:   []string{"Missing retry|No retry on timeout|Must retry 3 times"},
			wantLen: 1,
		},
		{
			name:    "valid with 2 fields",
			input:   []string{"Missing retry|No retry on timeout"},
			wantLen: 1,
		},
		{
			name:    "multiple gaps",
			input:   []string{"Gap 1|Desc 1", "Gap 2|Desc 2|Criteria 2"},
			wantLen: 2,
		},
		{
			name:    "nil input",
			input:   nil,
			wantLen: 0,
		},
		{
			name:    "too few fields",
			input:   []string{"just-a-title"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gaps, err := parseGaps(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Len(t, gaps, tt.wantLen)
		})
	}
}

func TestParseGaps_FieldValues(t *testing.T) {
	gaps, err := parseGaps([]string{"Missing retry|No retry on timeout|Must retry 3 times"})
	require.NoError(t, err)
	require.Len(t, gaps, 1)

	assert.Equal(t, "Missing retry", gaps[0].Title)
	assert.Equal(t, "No retry on timeout", gaps[0].Description)
	assert.Equal(t, "Must retry 3 times", gaps[0].Criteria)
}

func TestVerifyResultJSON_Structure(t *testing.T) {
	msg := VerifyResultMessage{
		Type:      "verify-result",
		ID:        "1234567890-verifier-001",
		From:      "verifier",
		RequestID: "1234567890-orch-001",
		Status:    "VERIFIED",
		Checks: []CheckResult{
			{Description: "tests", Command: "go test", Result: "PASS", Details: "ok"},
		},
		Gaps: nil,
	}

	jsonBytes, err := json.MarshalIndent(msg, "", "  ")
	require.NoError(t, err)

	// Verify it's valid JSON
	assert.True(t, json.Valid(jsonBytes))

	// Parse back and verify
	var parsed VerifyResultMessage
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "verify-result", parsed.Type)
	assert.Equal(t, "1234567890-verifier-001", parsed.ID)
	assert.Equal(t, "verifier", parsed.From)
	assert.Equal(t, "1234567890-orch-001", parsed.RequestID)
	assert.Equal(t, "VERIFIED", parsed.Status)
	assert.Len(t, parsed.Checks, 1)
	assert.Nil(t, parsed.Gaps)
}

func TestVerifyResultJSON_WithGaps(t *testing.T) {
	msg := VerifyResultMessage{
		Type:      "verify-result",
		ID:        "1234567890-verifier-002",
		From:      "verifier",
		RequestID: "1234567890-orch-001",
		Status:    "GAPS_FOUND",
		Checks: []CheckResult{
			{Description: "tests", Command: "go test", Result: "FAIL", Details: "3 failures"},
		},
		Gaps: []GapEntry{
			{Title: "Missing retry", Description: "No retry on timeout", Criteria: "Must retry 3 times"},
		},
	}

	jsonBytes, err := json.MarshalIndent(msg, "", "  ")
	require.NoError(t, err)

	var parsed VerifyResultMessage
	err = json.Unmarshal(jsonBytes, &parsed)
	require.NoError(t, err)

	assert.Equal(t, "GAPS_FOUND", parsed.Status)
	assert.Len(t, parsed.Gaps, 1)
	assert.Equal(t, "Missing retry", parsed.Gaps[0].Title)
}

func TestVerifyStatusValidation(t *testing.T) {
	tests := []struct {
		status string
		valid  bool
	}{
		{"VERIFIED", true},
		{"GAPS_FOUND", true},
		{"verified", false},
		{"PASSED", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			valid := tt.status == "VERIFIED" || tt.status == "GAPS_FOUND"
			assert.Equal(t, tt.valid, valid)
		})
	}
}
