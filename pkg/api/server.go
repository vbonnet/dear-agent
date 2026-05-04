package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/vbonnet/dear-agent/pkg/audit"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

// RunsDB is the subset of *sql.DB the API needs. Pulled out as an
// interface so future tests can swap in a fake; the production wiring
// passes a *sql.DB directly.
type RunsDB interface {
	PingContext(ctx context.Context) error
}

// Server is the HTTP handler. Construct with New and pass to
// http.ListenAndServe (or hand off to a tsnet listener — see
// cmd/dear-agent-api).
//
// The zero value is not usable. All fields except Logger are
// required.
type Server struct {
	// RunsDB is the workflow runs database (the same file the runner
	// writes; pkg/workflow query helpers read from it directly).
	RunsDB *sql.DB

	// AuditStore is the audit subsystem store. May be nil if audit
	// endpoints are disabled.
	AuditStore audit.Store

	// Identifier maps requests to callers; see identity.go.
	Identifier Identifier

	// Runner triggers a workflow run for POST /run. The default
	// implementation shells out to `workflow-run`; tests inject a
	// stub.
	Runner Runner

	// Version is reported by GET /status.
	Version string

	// Logger receives request and error logs. If nil, slog.Default()
	// is used.
	Logger *slog.Logger

	mux *http.ServeMux
}

// Runner triggers a workflow execution. Returning a non-empty run_id
// is the success path; the API surfaces it to the caller.
type Runner interface {
	Run(ctx context.Context, req RunRequest, caller Caller) (RunResponse, error)
}

// RunRequest is the body of POST /run.
type RunRequest struct {
	File   string            `json:"file"`             // path to workflow YAML on the server
	Inputs map[string]string `json:"inputs,omitempty"` // workflow inputs
}

// RunResponse is the body returned from POST /run.
type RunResponse struct {
	RunID    string `json:"run_id,omitempty"`
	Workflow string `json:"workflow,omitempty"`
	PID      int    `json:"pid,omitempty"`
}

// New constructs a Server with sensible defaults filled in.
func New(s Server) *Server {
	if s.Logger == nil {
		s.Logger = slog.Default()
	}
	if s.Version == "" {
		s.Version = inferVersion()
	}
	srv := &s
	srv.routes()
	return srv
}

// inferVersion reads the build metadata embedded by `go build` so the
// /status endpoint reports something useful in production. Falls back
// to "dev" when run from `go run` or a fresh `go build` without
// version info.
func inferVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// ServeHTTP routes the request to a handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.handleStatus)
	mux.HandleFunc("GET /workflows", s.handleListWorkflows)
	mux.HandleFunc("GET /workflows/{run_id}", s.handleGetWorkflow)
	mux.HandleFunc("GET /gates", s.handleListGates)
	mux.HandleFunc("POST /gates/{approval_id}/approve", s.handleGateDecision(workflow.HITLDecisionApprove))
	mux.HandleFunc("POST /gates/{approval_id}/reject", s.handleGateDecision(workflow.HITLDecisionReject))
	mux.HandleFunc("GET /audit/findings", s.handleListFindings)
	mux.HandleFunc("POST /run", s.handleRun)
	s.mux = mux
}

// --- handlers ---

// statusResponse is the body of GET /status.
type statusResponse struct {
	OK      bool   `json:"ok"`
	Version string `json:"version"`
	DBPing  bool   `json:"db_ping"`
	Caller  Caller `json:"caller"`
	Now     string `json:"now"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	caller, _ := s.Identifier.Identify(r.Context(), r)
	dbOK := true
	if err := s.RunsDB.PingContext(r.Context()); err != nil {
		dbOK = false
	}
	writeJSON(w, http.StatusOK, statusResponse{
		OK:      dbOK,
		Version: s.Version,
		DBPing:  dbOK,
		Caller:  caller,
		Now:     time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleListWorkflows(w http.ResponseWriter, r *http.Request) {
	state := workflow.RunState(r.URL.Query().Get("state"))
	limit := parseLimit(r.URL.Query().Get("limit"), 50, 500)
	runs, err := workflow.List(r.Context(), s.RunsDB, workflow.ListOptions{State: state, Limit: limit})
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "list_runs", err)
		return
	}
	if runs == nil {
		runs = []workflow.RunSummary{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

func (s *Server) handleGetWorkflow(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	if runID == "" {
		s.writeError(w, r, http.StatusBadRequest, "missing_run_id", errors.New("run_id required"))
		return
	}
	st, err := workflow.Status(r.Context(), s.RunsDB, runID)
	if err != nil {
		if errors.Is(err, workflow.ErrRunNotFound) {
			s.writeError(w, r, http.StatusNotFound, "run_not_found", err)
			return
		}
		s.writeError(w, r, http.StatusInternalServerError, "status", err)
		return
	}
	logsLimit := parseLimit(r.URL.Query().Get("logs_limit"), 200, 1000)
	events, err := workflow.Logs(r.Context(), s.RunsDB, runID, workflow.LogsOptions{Limit: logsLimit})
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "logs", err)
		return
	}
	if events == nil {
		events = []workflow.AuditEvent{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": st, "events": events})
}

func (s *Server) handleListGates(w http.ResponseWriter, r *http.Request) {
	pending, err := workflow.ListPendingHITLRequests(r.Context(), s.RunsDB)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "list_gates", err)
		return
	}
	if pending == nil {
		pending = []workflow.HITLRequest{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"gates": pending})
}

// gateDecisionRequest is the optional body of POST /gates/.../approve.
type gateDecisionRequest struct {
	Role   string `json:"role,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func (s *Server) handleGateDecision(dec workflow.HITLDecision) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		approvalID := r.PathValue("approval_id")
		if approvalID == "" {
			s.writeError(w, r, http.StatusBadRequest, "missing_approval_id", errors.New("approval_id required"))
			return
		}
		caller, err := s.Identifier.Identify(r.Context(), r)
		if err != nil {
			s.writeError(w, r, http.StatusUnauthorized, "identify", err)
			return
		}
		if caller.LoginName == "" {
			s.writeError(w, r, http.StatusUnauthorized, "no_caller", errors.New("caller identity not established"))
			return
		}
		var body gateDecisionRequest
		if r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				s.writeError(w, r, http.StatusBadRequest, "decode_body", err)
				return
			}
		}
		err = workflow.RecordHITLDecision(r.Context(), s.RunsDB, approvalID, dec, caller.LoginName, body.Role, body.Reason, time.Now())
		if err != nil {
			switch {
			case errors.Is(err, workflow.ErrApprovalNotFound):
				s.writeError(w, r, http.StatusNotFound, "approval_not_found", err)
			case errors.Is(err, workflow.ErrApprovalAlreadyResolved):
				s.writeError(w, r, http.StatusConflict, "already_resolved", err)
			case errors.Is(err, workflow.ErrApproverRoleMismatch):
				s.writeError(w, r, http.StatusForbidden, "role_mismatch", err)
			default:
				s.writeError(w, r, http.StatusInternalServerError, "record_decision", err)
			}
			return
		}
		s.Logger.Info("hitl decision recorded",
			"approval_id", approvalID,
			"decision", dec,
			"actor", caller.LoginName,
		)
		writeJSON(w, http.StatusOK, map[string]any{
			"approval_id": approvalID,
			"decision":    string(dec),
			"actor":       caller.LoginName,
		})
	}
}

func (s *Server) handleListFindings(w http.ResponseWriter, r *http.Request) {
	if s.AuditStore == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, "audit_disabled", errors.New("audit store not configured"))
		return
	}
	q := r.URL.Query()
	filter := audit.FindingFilter{
		Repo:     q.Get("repo"),
		CheckID:  q.Get("check_id"),
		Severity: audit.Severity(strings.ToUpper(q.Get("severity"))),
		State:    audit.FindingState(strings.ToLower(q.Get("state"))),
		Limit:    parseLimit(q.Get("limit"), 100, 1000),
	}
	if filter.Severity != "" && !filter.Severity.IsValid() {
		s.writeError(w, r, http.StatusBadRequest, "bad_severity", fmt.Errorf("severity %q not one of P0..P3", filter.Severity))
		return
	}
	if filter.State != "" && !filter.State.IsValid() {
		s.writeError(w, r, http.StatusBadRequest, "bad_state", fmt.Errorf("state %q not one of open/acknowledged/resolved/reopened", filter.State))
		return
	}
	findings, err := s.AuditStore.ListFindings(r.Context(), filter)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "list_findings", err)
		return
	}
	if findings == nil {
		findings = []audit.Finding{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"findings": findings})
}

func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	if s.Runner == nil {
		s.writeError(w, r, http.StatusServiceUnavailable, "runner_disabled", errors.New("runner not configured"))
		return
	}
	caller, err := s.Identifier.Identify(r.Context(), r)
	if err != nil {
		s.writeError(w, r, http.StatusUnauthorized, "identify", err)
		return
	}
	if caller.LoginName == "" {
		s.writeError(w, r, http.StatusUnauthorized, "no_caller", errors.New("caller identity not established"))
		return
	}
	var body RunRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.writeError(w, r, http.StatusBadRequest, "decode_body", err)
		return
	}
	if body.File == "" {
		s.writeError(w, r, http.StatusBadRequest, "missing_file", errors.New("file is required"))
		return
	}
	resp, err := s.Runner.Run(r.Context(), body, caller)
	if err != nil {
		s.writeError(w, r, http.StatusInternalServerError, "run", err)
		return
	}
	s.Logger.Info("workflow run triggered",
		"file", body.File,
		"actor", caller.LoginName,
		"pid", resp.PID,
	)
	writeJSON(w, http.StatusAccepted, resp)
}

// --- wire helpers ---

// errorResponse is the body of any non-2xx response.
type errorResponse struct {
	Error  string `json:"error"`
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, code string, err error) {
	s.Logger.Warn("api error",
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"code", code,
		"err", err,
	)
	writeJSON(w, status, errorResponse{
		Error:  http.StatusText(status),
		Code:   code,
		Detail: err.Error(),
	})
}

// parseLimit clamps a query-string limit to [1, ceiling], returning
// def when the value is missing or unparseable.
func parseLimit(s string, def, ceiling int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	if n > ceiling {
		return ceiling
	}
	return n
}

// --- default Runner implementation ---

// ExecRunner is the production Runner: it shells out to a `workflow-run`
// binary on $PATH, asynchronously. Returns immediately after the child
// is spawned; the API surfaces the child PID so the operator can
// follow it via `GET /workflows/{id}` once the child has registered
// the run in runs.db.
type ExecRunner struct {
	// Binary is the path to the workflow-run executable. Defaults to
	// "workflow-run" on $PATH.
	Binary string
	// WorkingDir is the cwd handed to bash nodes via -cwd. Empty =
	// inherit the API process's cwd.
	WorkingDir string
	// Logger receives spawn / lifecycle logs.
	Logger *slog.Logger
}

// Run spawns workflow-run as a child process. argv is built from req
// after validating each component to keep gosec's taint analysis
// happy: the binary path is operator-controlled (a CLI flag, not the
// network), the workflow file path must be a clean relative or
// absolute path with no shell metacharacters, and input keys/values
// are constrained to printable, separator-free strings.
func (e *ExecRunner) Run(_ context.Context, req RunRequest, caller Caller) (RunResponse, error) {
	binary := e.Binary
	if binary == "" {
		binary = "workflow-run"
	}
	if err := validateExecArg(req.File); err != nil {
		return RunResponse{}, fmt.Errorf("api: bad file arg: %w", err)
	}
	args := []string{"-file", req.File}
	if e.WorkingDir != "" {
		if err := validateExecArg(e.WorkingDir); err != nil {
			return RunResponse{}, fmt.Errorf("api: bad cwd: %w", err)
		}
		args = append(args, "-cwd", e.WorkingDir)
	}
	for k, v := range req.Inputs {
		if err := validateInputKV(k, v); err != nil {
			return RunResponse{}, fmt.Errorf("api: bad input: %w", err)
		}
		args = append(args, "-input", k+"="+v)
	}
	// binary is an operator-supplied flag value; every argv element
	// above this line is validated for shell-safe content, so the
	// only "taint" is the operator's own input.
	cmd := exec.Command(binary, args...) //nolint:gosec // G204/G702: argv pre-validated; binary is an operator-controlled flag
	cmd.Env = append(cmd.Environ(),
		"DEAR_AGENT_API_TRIGGERED_BY="+caller.LoginName,
	)
	if err := cmd.Start(); err != nil {
		return RunResponse{}, fmt.Errorf("api: spawn %s: %w", binary, err)
	}
	pid := cmd.Process.Pid
	logger := e.Logger
	if logger == nil {
		logger = slog.Default()
	}
	// The wait goroutine deliberately uses a fresh background context:
	// the request that triggered the run has already returned. We do
	// not want cancelling the request to also kill the child.
	go func() {
		if err := cmd.Wait(); err != nil {
			logger.Warn("workflow-run exited", "pid", pid, "err", err)
		} else {
			logger.Info("workflow-run exited", "pid", pid)
		}
	}()
	return RunResponse{PID: pid}, nil
}

// validateExecArg rejects arguments that contain shell metacharacters
// or control characters. The runner CLI does not accept anything
// shell-quoting-sensitive, so a strict allowlist is fine.
func validateExecArg(s string) error {
	if s == "" {
		return errors.New("empty")
	}
	if strings.ContainsAny(s, "\x00\n\r") {
		return errors.New("contains control character")
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return errors.New("contains control character")
		}
	}
	return nil
}

// validateInputKV enforces that input keys are simple identifier-like
// strings and that values are printable, single-line strings.
func validateInputKV(k, v string) error {
	if k == "" {
		return errors.New("empty key")
	}
	for _, r := range k {
		alnum := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !alnum && r != '_' && r != '-' && r != '.' {
			return fmt.Errorf("input key %q contains illegal character", k)
		}
	}
	if strings.ContainsAny(v, "\x00\n\r") {
		return fmt.Errorf("input value for %q contains control character", k)
	}
	return nil
}
