package main

import (
	"encoding/json"
	"testing"

	"github.com/vbonnet/dear-agent/agm/internal/ops"
)

func TestSessionGetFieldMaskKeepsSessionEnvelope(t *testing.T) {
	origFields := fieldsFlag
	origOutputMode := outputMode
	t.Cleanup(func() {
		fieldsFlag = origFields
		outputMode = origOutputMode
	})
	fieldsFlag = []string{"id", "name", "status"}
	outputMode = ModeAgent

	result := &ops.GetSessionResult{
		Operation: "get_session",
		Session: ops.SessionDetail{
			ID:      "session-123",
			Name:    "codex-smoke",
			Status:  "active",
			Harness: "codex-cli",
		},
	}

	payload, ok := buildSessionGetFieldMaskResponse(result, fieldsFlag)
	if !ok {
		t.Fatal("expected session detail field mask response")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal field-masked payload: %v", err)
	}

	var parsed map[string]map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal field-masked payload: %v", err)
	}

	sessionFields := parsed["session"]
	if len(sessionFields) != 3 {
		t.Fatalf("expected 3 session fields, got %d: %v", len(sessionFields), sessionFields)
	}
	if sessionFields["id"] != "session-123" || sessionFields["name"] != "codex-smoke" || sessionFields["status"] != "active" {
		t.Fatalf("unexpected session field mask output: %v", sessionFields)
	}
	if _, ok := sessionFields["harness"]; ok {
		t.Fatalf("did not expect unrequested harness field in %v", sessionFields)
	}
}

func TestSessionGetFieldMaskSupportsMixedEnvelopeAndSessionFields(t *testing.T) {
	result := &ops.GetSessionResult{
		Operation: "get_session",
		Session: ops.SessionDetail{
			ID:      "session-123",
			Name:    "codex-smoke",
			Status:  "active",
			Harness: "codex-cli",
		},
	}

	payload, ok := buildSessionGetFieldMaskResponse(result, []string{"operation", "id", "name"})
	if !ok {
		t.Fatal("expected mixed top-level/session field mask response")
	}
	if payload["operation"] != "get_session" {
		t.Fatalf("operation = %v, want get_session", payload["operation"])
	}

	sessionFields, ok := payload["session"].(map[string]any)
	if !ok {
		t.Fatalf("session payload has type %T, want map[string]any", payload["session"])
	}
	if len(sessionFields) != 2 {
		t.Fatalf("expected 2 session fields, got %d: %v", len(sessionFields), sessionFields)
	}
	if sessionFields["id"] != "session-123" || sessionFields["name"] != "codex-smoke" {
		t.Fatalf("unexpected mixed field mask output: %v", sessionFields)
	}
}
