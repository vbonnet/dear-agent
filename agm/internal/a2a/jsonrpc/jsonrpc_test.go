package jsonrpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vbonnet/dear-agent/agm/internal/a2a/protocol"
)

func TestToJSONRPC(t *testing.T) {
	msg := &protocol.Message{
		AgentID:       "agent-1",
		Timestamp:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:        protocol.StatusPending,
		MessageNumber: 1,
		Context:       "Test context",
		Proposal:      "Test proposal",
		Questions:     []string{"Q1"},
		Blockers:      []string{},
		NextSteps:     []string{"Step 1"},
	}

	rpc := ToJSONRPC(msg)

	assert.Equal(t, "2.0", rpc.JSONRPC)
	assert.Equal(t, "agent.message", rpc.Method)
	assert.Equal(t, "msg-1", rpc.ID)
	require.NotNil(t, rpc.Params)
	assert.Equal(t, "agent-1", rpc.Params.AgentID)
	assert.Equal(t, "pending", rpc.Params.Status)
	assert.Equal(t, 1, rpc.Params.MessageNumber)
}

func TestFromJSONRPC(t *testing.T) {
	rpc := Message{
		JSONRPC: "2.0",
		Method:  "agent.message",
		Params: &Params{
			AgentID:       "agent-2",
			Timestamp:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			Status:        "awaiting-response",
			MessageNumber: 2,
			Context:       "Review needed",
			Proposal:      "Please review this",
			Questions:     []string{"Approved?"},
			Blockers:      []string{},
			NextSteps:     []string{"Wait"},
		},
		ID: "msg-2",
	}

	msg, err := FromJSONRPC(rpc)
	require.NoError(t, err)
	assert.Equal(t, "agent-2", msg.AgentID)
	assert.Equal(t, protocol.StatusAwaitingResponse, msg.Status)
	assert.Equal(t, 2, msg.MessageNumber)
}

func TestValidate(t *testing.T) {
	// Valid request
	msg := Message{
		JSONRPC: "2.0",
		Method:  "agent.message",
		Params: &Params{
			AgentID:       "a",
			Context:       "c",
			Proposal:      "p",
			MessageNumber: 1,
		},
		ID: "1",
	}
	assert.NoError(t, Validate(msg))

	// Wrong version
	msg.JSONRPC = "1.0"
	assert.Error(t, Validate(msg))

	// Wrong method
	msg.JSONRPC = "2.0"
	msg.Method = "wrong"
	assert.Error(t, Validate(msg))

	// Both result and error
	msg2 := Message{
		JSONRPC: "2.0",
		Result:  "ok",
		Error:   &Error{Code: -1, Message: "err"},
		ID:      "1",
	}
	assert.Error(t, Validate(msg2))
}

func TestMarshalUnmarshal(t *testing.T) {
	msg := SuccessResponse("1", map[string]string{"status": "ok"})
	data, err := Marshal(msg)
	require.NoError(t, err)

	parsed, err := Unmarshal(data)
	require.NoError(t, err)
	assert.Equal(t, "2.0", parsed.JSONRPC)
	assert.NotNil(t, parsed.Result)
}

func TestErrorResponse(t *testing.T) {
	msg := ErrorResponse("1", ErrorInternalError, "something broke")
	assert.Equal(t, "2.0", msg.JSONRPC)
	require.NotNil(t, msg.Error)
	assert.Equal(t, ErrorInternalError, msg.Error.Code)
	assert.Equal(t, "something broke", msg.Error.Message)
}
