package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/a2aproject/a2a-go/a2a"
)

func TestA2AHandler_MethodNotAllowed(t *testing.T) {
	h := newA2AHandler(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	req := httptest.NewRequest(http.MethodPost, "/.well-known/agent.json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestA2AHandler_NotFound(t *testing.T) {
	h := newA2AHandler(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestA2AHandler_SessionCardNotFound(t *testing.T) {
	// Without a running Dolt, this will return 500 (storage error),
	// but we verify routing works.
	h := newA2AHandler(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	req := httptest.NewRequest(http.MethodGet, "/.well-known/agents/nonexistent.json", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Expect either 404 (not found) or 500 (no dolt) — not a panic
	if w.Code != http.StatusNotFound && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 404 or 500, got %d", w.Code)
	}
}

func TestA2AHandler_WriteJSON(t *testing.T) {
	h := newA2AHandler(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	w := httptest.NewRecorder()

	card := a2a.AgentCard{
		Name:            "test",
		Description:     "test agent",
		ProtocolVersion: string(a2a.Version),
	}
	h.writeJSON(w, card)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var result a2a.AgentCard
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result.Name != "test" {
		t.Errorf("expected name 'test', got %q", result.Name)
	}
}
