package protocol

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMessage(t *testing.T) {
	msg := NewMessage("agent-1", StatusPending)
	assert.Equal(t, "agent-1", msg.AgentID)
	assert.Equal(t, StatusPending, msg.Status)
	assert.NotZero(t, msg.Timestamp)
	assert.Empty(t, msg.Questions)
	assert.Empty(t, msg.Blockers)
	assert.Empty(t, msg.NextSteps)
}

func TestMessage_Validate(t *testing.T) {
	tests := []struct {
		name    string
		msg     *Message
		wantErr string
	}{
		{
			name:    "missing agent ID",
			msg:     &Message{Status: StatusPending, MessageNumber: 1, Context: "c", Proposal: "p"},
			wantErr: "agent ID is required",
		},
		{
			name:    "invalid status",
			msg:     &Message{AgentID: "a", Status: "bad", MessageNumber: 1, Context: "c", Proposal: "p"},
			wantErr: "invalid status",
		},
		{
			name:    "zero message number",
			msg:     &Message{AgentID: "a", Status: StatusPending, MessageNumber: 0, Context: "c", Proposal: "p"},
			wantErr: "message number must be >= 1",
		},
		{
			name:    "missing context",
			msg:     &Message{AgentID: "a", Status: StatusPending, MessageNumber: 1, Proposal: "p"},
			wantErr: "context is required",
		},
		{
			name:    "missing proposal",
			msg:     &Message{AgentID: "a", Status: StatusPending, MessageNumber: 1, Context: "c"},
			wantErr: "proposal/response is required",
		},
		{
			name: "valid message",
			msg:  &Message{AgentID: "a", Status: StatusPending, MessageNumber: 1, Context: "c", Proposal: "p"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.msg.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestMessage_Format(t *testing.T) {
	msg := &Message{
		AgentID:       "test-agent",
		Timestamp:     time.Date(2026, 3, 30, 10, 0, 0, 0, time.UTC),
		Status:        StatusAwaitingResponse,
		MessageNumber: 1,
		Context:       "Working on migration",
		Proposal:      "Move A2A to Go",
		Questions:     []string{"Is this approved?"},
		Blockers:      []string{},
		NextSteps:     []string{"Start coding"},
	}

	formatted := msg.Format()
	assert.Contains(t, formatted, "**Agent ID**: test-agent")
	assert.Contains(t, formatted, "**Status**: awaiting-response")
	assert.Contains(t, formatted, "Working on migration")
	assert.Contains(t, formatted, "Move A2A to Go")
	assert.Contains(t, formatted, "Is this approved?")
	assert.Contains(t, formatted, "Start coding")
}

func TestMessage_EstimateTokens(t *testing.T) {
	msg := &Message{
		AgentID:       "a",
		Timestamp:     time.Now(),
		Status:        StatusPending,
		MessageNumber: 1,
		Context:       strings.Repeat("x", 400),
		Proposal:      strings.Repeat("y", 400),
	}
	tokens := msg.EstimateTokens()
	assert.Greater(t, tokens, 100)
}
