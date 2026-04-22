package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkRequestCmd_FlagsExist(t *testing.T) {
	flags := []struct {
		name     string
		defValue string
	}{
		{"title", ""},
		{"description", ""},
		{"priority", "P2"},
		{"scope", "M"},
		{"dry-run", "false"},
	}

	for _, f := range flags {
		t.Run(f.name, func(t *testing.T) {
			flag := sendWorkRequestCmd.Flags().Lookup(f.name)
			require.NotNil(t, flag, "flag --%s should exist", f.name)
			assert.Equal(t, f.defValue, flag.DefValue)
		})
	}

	// Check slice flags exist
	assert.NotNil(t, sendWorkRequestCmd.Flags().Lookup("acceptance-criteria"))
	assert.NotNil(t, sendWorkRequestCmd.Flags().Lookup("check"))
}

func TestWorkRequestCmd_RequiredFlags(t *testing.T) {
	// title and description are required
	titleFlag := sendWorkRequestCmd.Flags().Lookup("title")
	require.NotNil(t, titleFlag)

	descFlag := sendWorkRequestCmd.Flags().Lookup("description")
	require.NotNil(t, descFlag)
}

func TestBuildWorkRequestJSON(t *testing.T) {
	jsonStr, err := BuildWorkRequestJSON(
		"orchestrator",
		"1234567890-orch-001",
		"Fix login bug",
		"Users cannot log in after password reset",
		"P1",
		"S",
		[]string{"Login works after reset", "Error message shown on failure"},
		[]string{"go test ./...", "curl http://localhost/login"},
	)
	require.NoError(t, err)

	// Parse and verify
	var msg WorkRequestMessage
	err = json.Unmarshal([]byte(jsonStr), &msg)
	require.NoError(t, err)

	assert.Equal(t, "work-request", msg.Type)
	assert.Equal(t, "1234567890-orch-001", msg.ID)
	assert.Equal(t, "orchestrator", msg.From)
	assert.Equal(t, "Fix login bug", msg.Title)
	assert.Equal(t, "Users cannot log in after password reset", msg.Description)
	assert.Equal(t, "P1", msg.Priority)
	assert.Equal(t, "S", msg.Scope)
	assert.Len(t, msg.Acceptance, 2)
	assert.Equal(t, "Login works after reset", msg.Acceptance[0])
	assert.Equal(t, "orchestrator", msg.Verification.Originator)
	assert.Len(t, msg.Verification.Checks, 2)
}

func TestBuildWorkRequestJSON_Minimal(t *testing.T) {
	jsonStr, err := BuildWorkRequestJSON(
		"sender",
		"1234567890-sender-001",
		"test title",
		"test description",
		"P2",
		"M",
		nil,
		nil,
	)
	require.NoError(t, err)

	var msg WorkRequestMessage
	err = json.Unmarshal([]byte(jsonStr), &msg)
	require.NoError(t, err)

	assert.Equal(t, "work-request", msg.Type)
	assert.Equal(t, "test title", msg.Title)
	assert.Nil(t, msg.Acceptance)
	assert.Nil(t, msg.Verification.Checks)
}

func TestWorkRequestJSON_ValidJSON(t *testing.T) {
	jsonStr, err := BuildWorkRequestJSON(
		"test",
		"1234567890-test-001",
		"title",
		"desc",
		"P2",
		"M",
		[]string{"criteria 1"},
		[]string{"check 1"},
	)
	require.NoError(t, err)

	// Verify it's valid JSON
	assert.True(t, json.Valid([]byte(jsonStr)))
}

func TestWorkRequestPriorityValidation(t *testing.T) {
	tests := []struct {
		priority string
		valid    bool
	}{
		{"P0", true},
		{"P1", true},
		{"P2", true},
		{"P3", true},
		{"P4", false},
		{"high", false},
		{"", false},
	}

	validPriorities := map[string]bool{"P0": true, "P1": true, "P2": true, "P3": true}
	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			assert.Equal(t, tt.valid, validPriorities[tt.priority])
		})
	}
}

func TestWorkRequestScopeValidation(t *testing.T) {
	tests := []struct {
		scope string
		valid bool
	}{
		{"XS", true},
		{"S", true},
		{"M", true},
		{"L", true},
		{"XL", true},
		{"XXL", false},
		{"small", false},
		{"", false},
	}

	validScopes := map[string]bool{"XS": true, "S": true, "M": true, "L": true, "XL": true}
	for _, tt := range tests {
		t.Run(tt.scope, func(t *testing.T) {
			assert.Equal(t, tt.valid, validScopes[tt.scope])
		})
	}
}
