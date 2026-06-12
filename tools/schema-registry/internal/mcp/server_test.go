package mcp

import (
	"testing"
)

func newTestServer() *Server {
	return NewServer("", false)
}

// --- HandleRequest dispatch ---

func TestHandleRequest_MissingMethod(t *testing.T) {
	s := newTestServer()
	resp := s.HandleRequest(map[string]any{"id": 1})
	if code, ok := resp["error"].(map[string]any)["code"]; !ok || code != -32600 {
		t.Errorf("expected Invalid Request error (-32600), got: %v", resp)
	}
}

func TestHandleRequest_UnknownMethod(t *testing.T) {
	s := newTestServer()
	resp := s.HandleRequest(map[string]any{"id": 1, "method": "bogus/method"})
	err, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error in response, got: %v", resp)
	}
	if code := err["code"]; code != -32601 {
		t.Errorf("expected Method not found error (-32601), got code: %v", code)
	}
}

// --- handleInitialize ---

func TestHandleRequest_Initialize(t *testing.T) {
	s := newTestServer()
	resp := s.HandleRequest(map[string]any{"id": 1, "method": "initialize"})

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result in initialize response, got: %v", resp)
	}
	if pv, _ := result["protocolVersion"].(string); pv == "" {
		t.Error("protocolVersion should be set in initialize result")
	}
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok {
		t.Fatal("serverInfo should be present")
	}
	if serverInfo["name"] == "" {
		t.Error("serverInfo.name should be non-empty")
	}
	if resp["id"] != 1 {
		t.Errorf("response id = %v, want 1", resp["id"])
	}
}

// --- handleToolsList ---

func TestHandleRequest_ToolsList(t *testing.T) {
	s := newTestServer()
	resp := s.HandleRequest(map[string]any{"id": 2, "method": "tools/list"})

	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result in tools/list response, got: %v", resp)
	}
	tools, ok := result["tools"].([]map[string]any)
	if !ok {
		t.Fatalf("expected tools slice in result, got: %T", result["tools"])
	}
	if len(tools) == 0 {
		t.Error("tools list should not be empty")
	}
	// Each tool must have a name and inputSchema.
	for _, tool := range tools {
		if tool["name"] == "" {
			t.Errorf("tool has empty name: %v", tool)
		}
		if tool["inputSchema"] == nil {
			t.Errorf("tool %q has no inputSchema", tool["name"])
		}
	}
}
