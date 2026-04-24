package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/state"
)

func TestNewServer(t *testing.T) {
	detector := state.NewDetector()
	s := NewServer(8080, detector)

	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.port != 8080 {
		t.Errorf("port = %d, want 8080", s.port)
	}
	if s.detector == nil {
		t.Error("detector should not be nil")
	}
	if s.sessions == nil {
		t.Error("sessions map should be initialized")
	}
}

func TestUpdateSessionState(t *testing.T) {
	detector := state.NewDetector()
	s := NewServer(0, detector)

	result := state.DetectionResult{
		State:      state.StateReady,
		Timestamp:  time.Now(),
		Evidence:   "prompt detected",
		Confidence: "high",
	}

	s.UpdateSessionState("test-session", result)

	s.mu.RLock()
	status, exists := s.sessions["test-session"]
	s.mu.RUnlock()

	if !exists {
		t.Fatal("session should exist after UpdateSessionState")
	}
	if status.SessionName != "test-session" {
		t.Errorf("SessionName = %q, want %q", status.SessionName, "test-session")
	}
	if status.State != state.StateReady {
		t.Errorf("State = %v, want %v", status.State, state.StateReady)
	}
	if status.Evidence != "prompt detected" {
		t.Errorf("Evidence = %q, want %q", status.Evidence, "prompt detected")
	}
}

func TestHandleHealth(t *testing.T) {
	detector := state.NewDetector()
	s := NewServer(0, detector)

	tests := []struct {
		name       string
		method     string
		wantCode   int
		wantStatus string
	}{
		{
			name:       "GET returns ok",
			method:     http.MethodGet,
			wantCode:   http.StatusOK,
			wantStatus: "ok",
		},
		{
			name:     "POST not allowed",
			method:   http.MethodPost,
			wantCode: http.StatusMethodNotAllowed,
		},
		{
			name:     "PUT not allowed",
			method:   http.MethodPut,
			wantCode: http.StatusMethodNotAllowed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/health", nil)
			w := httptest.NewRecorder()

			s.handleHealth(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantCode)
			}

			if tt.wantStatus != "" {
				var body map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatalf("failed to parse response: %v", err)
				}
				if body["status"] != tt.wantStatus {
					t.Errorf("status = %v, want %v", body["status"], tt.wantStatus)
				}
			}
		})
	}
}

func TestHandleHealth_SessionCount(t *testing.T) {
	detector := state.NewDetector()
	s := NewServer(0, detector)

	// Add sessions
	result := state.DetectionResult{
		State:      state.StateReady,
		Timestamp:  time.Now(),
		Confidence: "high",
	}
	s.UpdateSessionState("s1", result)
	s.UpdateSessionState("s2", result)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	s.handleHealth(w, req)

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	sessions, ok := body["sessions"].(float64)
	if !ok {
		t.Fatal("sessions field missing or wrong type")
	}
	if int(sessions) != 2 {
		t.Errorf("sessions = %d, want 2", int(sessions))
	}
}

func TestHandleStatus(t *testing.T) {
	detector := state.NewDetector()
	s := NewServer(0, detector)

	// POST should fail
	t.Run("POST not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/status", nil)
		w := httptest.NewRecorder()
		s.handleStatus(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusMethodNotAllowed)
		}
	})

	// Empty sessions
	t.Run("empty sessions", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/status", nil)
		w := httptest.NewRecorder()
		s.handleStatus(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}

		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		count, ok := body["count"].(float64)
		if !ok {
			t.Fatal("count field missing")
		}
		if int(count) != 0 {
			t.Errorf("count = %d, want 0", int(count))
		}
	})

	// With sessions
	t.Run("with sessions", func(t *testing.T) {
		result := state.DetectionResult{
			State:      state.StateThinking,
			Timestamp:  time.Now(),
			Evidence:   "spinner",
			Confidence: "high",
		}
		s.UpdateSessionState("session-a", result)
		s.UpdateSessionState("session-b", result)

		req := httptest.NewRequest(http.MethodGet, "/status", nil)
		w := httptest.NewRecorder()
		s.handleStatus(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("status code = %d, want %d", w.Code, http.StatusOK)
		}

		ct := w.Header().Get("Content-Type")
		if ct != "application/json" {
			t.Errorf("Content-Type = %q, want %q", ct, "application/json")
		}

		var body map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("failed to parse response: %v", err)
		}

		count := int(body["count"].(float64))
		if count != 2 {
			t.Errorf("count = %d, want 2", count)
		}
	})
}

func TestHandleSessionStatus(t *testing.T) {
	detector := state.NewDetector()
	s := NewServer(0, detector)

	result := state.DetectionResult{
		State:      state.StateBlockedAuth,
		Timestamp:  time.Now(),
		Evidence:   "y/N prompt",
		Confidence: "high",
	}
	s.UpdateSessionState("my-session", result)

	tests := []struct {
		name      string
		method    string
		path      string
		wantCode  int
		wantState state.State
	}{
		{
			name:      "existing session",
			method:    http.MethodGet,
			path:      "/status/my-session",
			wantCode:  http.StatusOK,
			wantState: state.StateBlockedAuth,
		},
		{
			name:      "nonexistent session",
			method:    http.MethodGet,
			path:      "/status/missing",
			wantCode:  http.StatusNotFound,
			wantState: state.StateUnknown,
		},
		{
			name:     "POST not allowed",
			method:   http.MethodPost,
			path:     "/status/my-session",
			wantCode: http.StatusMethodNotAllowed,
		},
		{
			name:     "bare status path",
			method:   http.MethodGet,
			path:     "/status/",
			wantCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			s.handleSessionStatus(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantCode)
			}

			if tt.wantState != "" {
				var status StatusResponse
				if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
					t.Fatalf("failed to parse response: %v", err)
				}
				if status.State != tt.wantState {
					t.Errorf("State = %v, want %v", status.State, tt.wantState)
				}
			}
		})
	}
}

func TestStop_NilServer(t *testing.T) {
	s := &Server{}
	err := s.Stop()
	if err != nil {
		t.Errorf("Stop() with nil server should return nil, got %v", err)
	}
}

func TestUpdateSessionState_Overwrite(t *testing.T) {
	detector := state.NewDetector()
	s := NewServer(0, detector)

	// Set initial state
	s.UpdateSessionState("sess", state.DetectionResult{
		State:      state.StateReady,
		Timestamp:  time.Now(),
		Confidence: "high",
	})

	// Overwrite
	s.UpdateSessionState("sess", state.DetectionResult{
		State:      state.StateStuck,
		Timestamp:  time.Now(),
		Evidence:   "no output for 120s",
		Confidence: "medium",
	})

	s.mu.RLock()
	status := s.sessions["sess"]
	s.mu.RUnlock()

	if status.State != state.StateStuck {
		t.Errorf("State = %v, want %v", status.State, state.StateStuck)
	}
}
