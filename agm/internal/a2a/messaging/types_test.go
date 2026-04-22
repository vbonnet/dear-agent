package messaging

import (
	"testing"
	"time"
)

func TestNewRequest(t *testing.T) {
	msg := NewRequest("agent-1", "agent-2", "do stuff", "please do stuff", "run-tests")

	if msg.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if msg.CorrelationID != msg.ID {
		t.Errorf("correlation_id should equal id for new requests, got %q vs %q", msg.CorrelationID, msg.ID)
	}
	if msg.Type != TypeRequest {
		t.Errorf("type = %q, want %q", msg.Type, TypeRequest)
	}
	if msg.Sender != "agent-1" {
		t.Errorf("sender = %q, want %q", msg.Sender, "agent-1")
	}
	if msg.Recipient != "agent-2" {
		t.Errorf("recipient = %q, want %q", msg.Recipient, "agent-2")
	}
	if msg.RoutingMode != RouteDirect {
		t.Errorf("routing_mode = %q, want %q", msg.RoutingMode, RouteDirect)
	}
	if msg.Subject != "do stuff" {
		t.Errorf("subject = %q, want %q", msg.Subject, "do stuff")
	}
	if msg.Body != "please do stuff" {
		t.Errorf("body = %q, want %q", msg.Body, "please do stuff")
	}
	if msg.Action != "run-tests" {
		t.Errorf("action = %q, want %q", msg.Action, "run-tests")
	}
	if msg.Priority != PriorityNormal {
		t.Errorf("priority = %q, want %q", msg.Priority, PriorityNormal)
	}
	if msg.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}

func TestNewResponse(t *testing.T) {
	req := NewRequest("agent-1", "agent-2", "do stuff", "please", "act")
	req.Priority = PriorityHigh

	resp := NewResponse("agent-2", req, "done")

	if resp.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if resp.CorrelationID != req.CorrelationID {
		t.Errorf("correlation_id = %q, want %q (from request)", resp.CorrelationID, req.CorrelationID)
	}
	if resp.Type != TypeResponse {
		t.Errorf("type = %q, want %q", resp.Type, TypeResponse)
	}
	if resp.Sender != "agent-2" {
		t.Errorf("sender = %q, want %q", resp.Sender, "agent-2")
	}
	if resp.Recipient != "agent-1" {
		t.Errorf("recipient = %q, want original sender %q", resp.Recipient, "agent-1")
	}
	if resp.Subject != "Re: do stuff" {
		t.Errorf("subject = %q, want %q", resp.Subject, "Re: do stuff")
	}
	if resp.Body != "done" {
		t.Errorf("body = %q, want %q", resp.Body, "done")
	}
	if resp.Priority != PriorityHigh {
		t.Errorf("priority = %q, want %q (inherited from request)", resp.Priority, PriorityHigh)
	}
}

func TestNewNotification(t *testing.T) {
	msg := NewNotification("agent-1", "status update", "all good")

	if msg.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if msg.Type != TypeNotification {
		t.Errorf("type = %q, want %q", msg.Type, TypeNotification)
	}
	if msg.RoutingMode != RouteBroadcast {
		t.Errorf("routing_mode = %q, want %q", msg.RoutingMode, RouteBroadcast)
	}
	if msg.Recipient != "" {
		t.Errorf("recipient should be empty for broadcast, got %q", msg.Recipient)
	}
}

func TestNewDelegation(t *testing.T) {
	msg := NewDelegation("orch", "worker-1", "build task", "build the app", "go-build")

	if msg.Type != TypeDelegation {
		t.Errorf("type = %q, want %q", msg.Type, TypeDelegation)
	}
	if msg.CorrelationID != msg.ID {
		t.Errorf("correlation_id should equal id for new delegations")
	}
	if msg.Action != "go-build" {
		t.Errorf("action = %q, want %q", msg.Action, "go-build")
	}
	if msg.RoutingMode != RouteDirect {
		t.Errorf("routing_mode = %q, want %q", msg.RoutingMode, RouteDirect)
	}
}

func TestNewRoleMessage(t *testing.T) {
	msg := NewRoleMessage("orch", "worker", TypeRequest, "need help", "someone help")

	if msg.Type != TypeRequest {
		t.Errorf("type = %q, want %q", msg.Type, TypeRequest)
	}
	if msg.RoutingMode != RouteRole {
		t.Errorf("routing_mode = %q, want %q", msg.RoutingMode, RouteRole)
	}
	if msg.TargetRole != "worker" {
		t.Errorf("target_role = %q, want %q", msg.TargetRole, "worker")
	}
	if msg.Sender != "orch" {
		t.Errorf("sender = %q, want %q", msg.Sender, "orch")
	}
}

func TestValidate_ValidMessages(t *testing.T) {
	tests := []struct {
		name string
		msg  *Message
	}{
		{"direct request", NewRequest("a", "b", "subj", "body", "act")},
		{"broadcast notification", NewNotification("a", "subj", "body")},
		{"role message", NewRoleMessage("a", "worker", TypeRequest, "subj", "body")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.msg.Validate(); err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
		})
	}
}

func TestValidate_MissingID(t *testing.T) {
	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.ID = ""
	if err := msg.Validate(); err == nil {
		t.Error("expected error for missing ID")
	}
}

func TestValidate_MissingSender(t *testing.T) {
	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.Sender = ""
	if err := msg.Validate(); err == nil {
		t.Error("expected error for missing sender")
	}
}

func TestValidate_MissingSubject(t *testing.T) {
	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.Subject = ""
	if err := msg.Validate(); err == nil {
		t.Error("expected error for missing subject")
	}
}

func TestValidate_MissingBody(t *testing.T) {
	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.Body = ""
	if err := msg.Validate(); err == nil {
		t.Error("expected error for missing body")
	}
}

func TestValidate_InvalidType(t *testing.T) {
	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.Type = "unknown"
	if err := msg.Validate(); err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestValidate_DirectMissingRecipient(t *testing.T) {
	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.Recipient = ""
	if err := msg.Validate(); err == nil {
		t.Error("expected error for direct routing without recipient")
	}
}

func TestValidate_RoleMissingTargetRole(t *testing.T) {
	msg := NewRoleMessage("a", "worker", TypeRequest, "subj", "body")
	msg.TargetRole = ""
	if err := msg.Validate(); err == nil {
		t.Error("expected error for role routing without target_role")
	}
}

func TestValidate_InvalidRoutingMode(t *testing.T) {
	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.RoutingMode = "pigeon"
	if err := msg.Validate(); err == nil {
		t.Error("expected error for invalid routing_mode")
	}
}

func TestValidate_ResponseMissingCorrelationID(t *testing.T) {
	req := NewRequest("a", "b", "subj", "body", "act")
	resp := NewResponse("b", req, "ok")
	resp.CorrelationID = ""
	if err := resp.Validate(); err == nil {
		t.Error("expected error for response without correlation_id")
	}
}

func TestIsExpired_NoExpiry(t *testing.T) {
	msg := NewRequest("a", "b", "subj", "body", "act")
	if msg.IsExpired() {
		t.Error("message without expiry should not be expired")
	}
}

func TestIsExpired_FutureExpiry(t *testing.T) {
	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.ExpiresAt = time.Now().Add(time.Hour)
	if msg.IsExpired() {
		t.Error("message with future expiry should not be expired")
	}
}

func TestIsExpired_PastExpiry(t *testing.T) {
	msg := NewRequest("a", "b", "subj", "body", "act")
	msg.ExpiresAt = time.Now().Add(-time.Hour)
	if !msg.IsExpired() {
		t.Error("message with past expiry should be expired")
	}
}
