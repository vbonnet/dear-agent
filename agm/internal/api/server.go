// Package api provides api functionality.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/vbonnet/dear-agent/agm/internal/state"
)

// StatusResponse represents API response for session status
type StatusResponse struct {
	SessionName string      `json:"session_name"`
	State       state.State `json:"state"`
	Timestamp   time.Time   `json:"timestamp"`
	Evidence    string      `json:"evidence,omitempty"`
	Confidence  string      `json:"confidence"`
	LastUpdated time.Time   `json:"last_updated"`
	Error       string      `json:"error,omitempty"`
}

// Server provides HTTP API for state queries
type Server struct {
	port     int
	detector *state.Detector
	sessions map[string]*StatusResponse
	mu       sync.RWMutex
	server   *http.Server
}

// NewServer creates a new API server
func NewServer(port int, detector *state.Detector) *Server {
	return &Server{
		port:     port,
		detector: detector,
		sessions: make(map[string]*StatusResponse),
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Register endpoints
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/status/", s.handleSessionStatus)
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server
func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}
	return s.server.Close()
}

// UpdateSessionState updates cached state for a session
func (s *Server) UpdateSessionState(sessionName string, result state.DetectionResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessions[sessionName] = &StatusResponse{
		SessionName: sessionName,
		State:       result.State,
		Timestamp:   result.Timestamp,
		Evidence:    result.Evidence,
		Confidence:  result.Confidence,
		LastUpdated: time.Now(),
	}
}

// handleStatus returns status for all sessions
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Convert map to slice for JSON response
	statuses := make([]StatusResponse, 0, len(s.sessions))
	for _, status := range s.sessions {
		statuses = append(statuses, *status)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"sessions":  statuses,
		"count":     len(statuses),
		"timestamp": time.Now(),
	})
}

// handleSessionStatus returns status for specific session
func (s *Server) handleSessionStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract session name from path: /status/{session-name}
	sessionName := filepath.Base(r.URL.Path)
	if sessionName == "" || sessionName == "status" {
		http.Error(w, "Session name required", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	status, exists := s.sessions[sessionName]
	s.mu.RUnlock()

	if !exists {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(StatusResponse{
			SessionName: sessionName,
			State:       state.StateUnknown,
			Timestamp:   time.Now(),
			Error:       "Session not found or not being monitored",
			Confidence:  "low",
			LastUpdated: time.Now(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleHealth returns server health status
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now(),
		"sessions":  len(s.sessions),
	})
}
