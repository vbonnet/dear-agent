package mcp

import "testing"

func TestNewServer_NotNil(t *testing.T) {
	s := NewServer(t.TempDir(), false)
	if s == nil {
		t.Fatal("NewServer() returned nil")
	}
}

func TestHandleRequest_UnknownMethod_ReturnsError(t *testing.T) {
	s := NewServer(t.TempDir(), false)
	resp := s.HandleRequest(map[string]any{
		"id":     1,
		"method": "nonexistent/method",
	})
	if resp == nil {
		t.Fatal("HandleRequest returned nil for unknown method")
	}
	if _, hasError := resp["error"]; !hasError {
		t.Error("HandleRequest should return error for unknown method")
	}
}
