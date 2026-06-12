package mcp

import (
	"testing"
)

func TestNewServer_NotNil(t *testing.T) {
	t.Parallel()
	s := NewServer("test-workspace", false)
	if s == nil {
		t.Fatal("NewServer() returned nil")
	}
}

func TestHandleRequest_UnknownMethod(t *testing.T) {
	t.Parallel()
	s := NewServer("ws", false)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "unknown/method",
	}
	resp := s.HandleRequest(req)
	if _, ok := resp["error"]; !ok {
		t.Errorf("HandleRequest(unknown method): expected error field, got %v", resp)
	}
}

func TestHandleRequest_MissingMethod(t *testing.T) {
	t.Parallel()
	s := NewServer("ws", false)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		// no "method" key
	}
	resp := s.HandleRequest(req)
	if _, ok := resp["error"]; !ok {
		t.Errorf("HandleRequest(no method): expected error field, got %v", resp)
	}
}

func TestHandleRequest_Initialize(t *testing.T) {
	t.Parallel()
	s := NewServer("ws", false)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "initialize",
	}
	resp := s.HandleRequest(req)
	if _, ok := resp["error"]; ok {
		t.Errorf("HandleRequest(initialize): unexpected error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("HandleRequest(initialize): result is not a map: %T", resp["result"])
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v, want 2024-11-05", result["protocolVersion"])
	}
}

func TestErrorResponse_Structure(t *testing.T) {
	t.Parallel()
	s := NewServer("ws", false)
	resp := s.errorResponse("req-1", -32600, "Invalid Request", nil)

	if resp["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", resp["jsonrpc"])
	}
	if resp["id"] != "req-1" {
		t.Errorf("id = %v, want req-1", resp["id"])
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field is not a map: %T", resp["error"])
	}
	if errObj["code"] != -32600 {
		t.Errorf("error.code = %v, want -32600", errObj["code"])
	}
}

func TestSuccessResponse_Structure(t *testing.T) {
	t.Parallel()
	s := NewServer("ws", false)
	resp := s.successResponse(42, "ok message", "schema://test", map[string]string{"k": "v"})

	if resp["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v, want 2.0", resp["jsonrpc"])
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("result field is not a map: %T", resp["result"])
	}
	if _, ok := result["content"]; !ok {
		t.Error("result.content missing")
	}
}
