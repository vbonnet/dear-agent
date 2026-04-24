package jsonrpc

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/a2a/protocol"
)

// Message represents a JSON-RPC 2.0 message for A2A protocol
type Message struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method,omitempty"`
	Params  *Params     `json:"params,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
	ID      interface{} `json:"id"`
}

// Params represents the parameters for an A2A agent message
type Params struct {
	AgentID       string    `json:"agent_id"`
	Timestamp     time.Time `json:"timestamp"`
	Status        string    `json:"status"`
	MessageNumber int       `json:"message_number"`
	Context       string    `json:"context"`
	Proposal      string    `json:"proposal"`
	Questions     []string  `json:"questions"`
	Blockers      []string  `json:"blockers"`
	NextSteps     []string  `json:"next_steps"`
}

// Error represents a JSON-RPC error
type Error struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

const (
	ErrorParseError     = -32700
	ErrorInvalidRequest = -32600
	ErrorMethodNotFound = -32601
	ErrorInvalidParams  = -32602
	ErrorInternalError  = -32603
)

// ToJSONRPC converts A2A protocol message to JSON-RPC format
func ToJSONRPC(msg *protocol.Message) *Message {
	return &Message{
		JSONRPC: "2.0",
		Method:  "agent.message",
		Params: &Params{
			AgentID:       msg.AgentID,
			Timestamp:     msg.Timestamp,
			Status:        msg.Status.String(),
			MessageNumber: msg.MessageNumber,
			Context:       msg.Context,
			Proposal:      msg.Proposal,
			Questions:     msg.Questions,
			Blockers:      msg.Blockers,
			NextSteps:     msg.NextSteps,
		},
		ID: fmt.Sprintf("msg-%d", msg.MessageNumber),
	}
}

// FromJSONRPC converts JSON-RPC message to A2A protocol message
func FromJSONRPC(rpcMsg Message) (*protocol.Message, error) {
	if err := Validate(rpcMsg); err != nil {
		return nil, err
	}

	status, err := protocol.ValidateStatus(rpcMsg.Params.Status)
	if err != nil {
		return nil, fmt.Errorf("invalid status field: %w", err)
	}

	msg := &protocol.Message{
		AgentID:       rpcMsg.Params.AgentID,
		Timestamp:     rpcMsg.Params.Timestamp,
		Status:        status,
		MessageNumber: rpcMsg.Params.MessageNumber,
		Context:       rpcMsg.Params.Context,
		Proposal:      rpcMsg.Params.Proposal,
		Questions:     rpcMsg.Params.Questions,
		Blockers:      rpcMsg.Params.Blockers,
		NextSteps:     rpcMsg.Params.NextSteps,
	}

	if err := msg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid A2A message: %w", err)
	}

	return msg, nil
}

// Validate validates JSON-RPC message structure
func Validate(msg Message) error {
	if msg.JSONRPC != "2.0" {
		return fmt.Errorf("invalid JSON-RPC version: %s (expected '2.0')", msg.JSONRPC)
	}

	if msg.Method != "" {
		if msg.Method != "agent.message" {
			return fmt.Errorf("invalid method: %s (expected 'agent.message')", msg.Method)
		}
		if msg.Params == nil {
			return fmt.Errorf("params required for method call")
		}
		if msg.Params.AgentID == "" {
			return fmt.Errorf("agent_id is required")
		}
		if msg.Params.Context == "" {
			return fmt.Errorf("context is required")
		}
		if msg.Params.Proposal == "" {
			return fmt.Errorf("proposal is required")
		}
		if msg.Params.MessageNumber < 1 {
			return fmt.Errorf("message_number must be >= 1")
		}
	}

	if msg.Result != nil && msg.Error != nil {
		return fmt.Errorf("message cannot have both result and error")
	}

	return nil
}

// SuccessResponse creates a JSON-RPC success response
func SuccessResponse(id interface{}, result interface{}) *Message {
	return &Message{
		JSONRPC: "2.0",
		Result:  result,
		ID:      id,
	}
}

// ErrorResponse creates a JSON-RPC error response
func ErrorResponse(id interface{}, code int, message string) *Message {
	return &Message{
		JSONRPC: "2.0",
		Error: &Error{
			Code:    code,
			Message: message,
		},
		ID: id,
	}
}

// ErrorResponseWithData creates a JSON-RPC error response with additional data
func ErrorResponseWithData(id interface{}, code int, message string, data interface{}) *Message {
	return &Message{
		JSONRPC: "2.0",
		Error: &Error{
			Code:    code,
			Message: message,
			Data:    data,
		},
		ID: id,
	}
}

// Marshal converts Message to JSON bytes
func Marshal(msg *Message) ([]byte, error) {
	return json.MarshalIndent(msg, "", "  ")
}

// Unmarshal parses JSON bytes into Message
func Unmarshal(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
