package restbackend

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Server provides a REST API for managing sessions via the RestBackend.
type Server struct {
	backend *RestBackend
	mux     *http.ServeMux
}

// NewServer creates a new REST API server backed by the given RestBackend.
func NewServer(b *RestBackend) *Server {
	s := &Server{backend: b}
	s.mux = http.NewServeMux()
	s.registerRoutes()
	return s
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the HTTP server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s)
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("POST /sessions", s.handleCreateSession)
	s.mux.HandleFunc("GET /sessions", s.handleListSessions)
	s.mux.HandleFunc("GET /sessions/", s.handleSessionRoutes)
	s.mux.HandleFunc("POST /sessions/", s.handleSessionRoutes)
	s.mux.HandleFunc("DELETE /sessions/", s.handleSessionRoutes)
}

// --- Request/Response types ---

type createSessionRequest struct {
	Name    string `json:"name"`
	Workdir string `json:"workdir,omitempty"`
}

type sessionResponse struct {
	Name   string       `json:"name"`
	State  ProcessState `json:"state"`
	Alive  bool         `json:"alive"`
}

type sendMessageRequest struct {
	Message string `json:"message"`
}

type outputResponse struct {
	Output string `json:"output"`
	Lines  int    `json:"lines"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// --- Handlers ---

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	var req createSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name is required"})
		return
	}

	if err := s.backend.CreateSession(req.Name, req.Workdir); err != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
		return
	}

	state, _ := s.backend.GetProcessState(req.Name)
	writeJSON(w, http.StatusCreated, sessionResponse{
		Name:  req.Name,
		State: state,
		Alive: true,
	})
}

func (s *Server) handleListSessions(w http.ResponseWriter, r *http.Request) {
	infos, err := s.backend.ListSessionsWithInfo()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}

	sessions := make([]sessionResponse, len(infos))
	for i, info := range infos {
		state, _ := s.backend.GetProcessState(info.Name)
		sessions[i] = sessionResponse{
			Name:  info.Name,
			State: state,
			Alive: true,
		}
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (s *Server) handleSessionRoutes(w http.ResponseWriter, r *http.Request) {
	// Parse: /sessions/{name} or /sessions/{name}/message or /sessions/{name}/output
	path := strings.TrimPrefix(r.URL.Path, "/sessions/")
	parts := strings.SplitN(path, "/", 2)
	sessionName := parts[0]

	if sessionName == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "session name is required"})
		return
	}

	subpath := ""
	if len(parts) > 1 {
		subpath = parts[1]
	}

	switch {
	case r.Method == http.MethodGet && subpath == "":
		s.handleGetSession(w, r, sessionName)
	case r.Method == http.MethodGet && subpath == "output":
		s.handleGetOutput(w, r, sessionName)
	case r.Method == http.MethodPost && subpath == "message":
		s.handleSendMessage(w, r, sessionName)
	case r.Method == http.MethodDelete && subpath == "":
		s.handleDeleteSession(w, r, sessionName)
	default:
		writeJSON(w, http.StatusNotFound, errorResponse{
			Error: fmt.Sprintf("unknown route: %s /sessions/%s", r.Method, path),
		})
	}
}

func (s *Server) handleGetSession(w http.ResponseWriter, _ *http.Request, name string) {
	exists, err := s.backend.HasSession(name)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	if !exists {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: fmt.Sprintf("session %q not found", name)})
		return
	}

	state, _ := s.backend.GetProcessState(name)
	writeJSON(w, http.StatusOK, sessionResponse{
		Name:  name,
		State: state,
		Alive: state == ProcessStateRunning || state == ProcessStateStarting,
	})
}

func (s *Server) handleGetOutput(w http.ResponseWriter, r *http.Request, name string) {
	lines := 50
	if l := r.URL.Query().Get("lines"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			lines = n
		}
	}

	output, err := s.backend.ReadOutput(name, lines)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, outputResponse{
		Output: output,
		Lines:  lines,
	})
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request, name string) {
	var req sendMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if req.Message == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "message is required"})
		return
	}

	if err := s.backend.SendKeys(name, req.Message); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"delivered": true})
}

func (s *Server) handleDeleteSession(w http.ResponseWriter, _ *http.Request, name string) {
	if err := s.backend.TerminateSession(name); err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "not found") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, errorResponse{Error: err.Error()})
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
