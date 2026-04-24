package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatus_IsValid(t *testing.T) {
	tests := []struct {
		status Status
		valid  bool
	}{
		{StatusPending, true},
		{StatusAwaitingResponse, true},
		{StatusConsensusReached, true},
		{StatusEscalateToHuman, true},
		{StatusCoordinationNeeded, true},
		{StatusHandoffComplete, true},
		{NewBlockedStatus("dependency"), true},
		{NewBlockedStatus("review"), true},
		{Status("invalid"), false},
		{Status(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			assert.Equal(t, tt.valid, tt.status.IsValid())
		})
	}
}

func TestStatus_IsBlocked(t *testing.T) {
	assert.True(t, NewBlockedStatus("dep").IsBlocked())
	assert.False(t, StatusPending.IsBlocked())
	assert.False(t, StatusConsensusReached.IsBlocked())
}

func TestStatus_BlockedReason(t *testing.T) {
	s := NewBlockedStatus("missing-data")
	assert.Equal(t, "missing-data", s.BlockedReason())
	assert.Equal(t, "", StatusPending.BlockedReason())
}

func TestValidateStatus(t *testing.T) {
	s, err := ValidateStatus("pending")
	require.NoError(t, err)
	assert.Equal(t, StatusPending, s)

	s, err = ValidateStatus("blocked-on-review")
	require.NoError(t, err)
	assert.True(t, s.IsBlocked())

	_, err = ValidateStatus("garbage")
	assert.Error(t, err)
}
