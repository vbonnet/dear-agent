// Package main implements the dear-agent MCP server. It exposes the
// workflow engine's read/write surface as MCP tools so any MCP client
// (Claude Code, Cursor, custom agents) can drive runs without shelling
// out to the CLI.
//
// Phase 2 ships five workflow tools:
//
//	workflow_run     — start a new run from a YAML file
//	workflow_status  — fetch the status of a run by id
//	workflow_approve — approve a pending HITL request
//	workflow_reject  — reject a pending HITL request
//	workflow_cancel  — cancel an in-flight run
//
// The server speaks JSON-RPC 2.0 over stdio (the standard MCP transport).
// `--http :PORT` switches to a tiny HTTP shim used in tests; production
// deployments stick to stdio because that is what every MCP client speaks.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/vbonnet/dear-agent/pkg/source"
	sqlitesource "github.com/vbonnet/dear-agent/pkg/source/sqlite"
	"github.com/vbonnet/dear-agent/pkg/workflow"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		dbPath     = flag.String("db", "runs.db", "path to runs.db")
		sourcePath = flag.String("sources", "", "path to sources.db (default: same as --db)")
		httpAddr   = flag.String("http", "", "if set, serve JSON-RPC over HTTP at this address (default: stdio)")
	)
	flag.Parse()

	db, err := openDB(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer db.Close()

	// The sources adapter lives in the same SQLite file as runs by
	// default — this makes JOINs across `sources` and `runs` cheap and
	// keeps deployments to one file. Override with --sources to host
	// the knowledge corpus elsewhere.
	var srcAdapter source.Adapter
	if *sourcePath == "" || *sourcePath == *dbPath {
		a, err := sqlitesource.New(db)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		srcAdapter = a
	} else {
		a, err := sqlitesource.Open(*sourcePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		defer a.Close()
		srcAdapter = a
	}

	srv := &Server{DB: db, DBPath: *dbPath, Source: srcAdapter}

	if *httpAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/jsonrpc", srv.ServeHTTP)
		fmt.Fprintf(os.Stderr, "dear-agent-mcp listening on %s\n", *httpAddr)
		if err := http.ListenAndServe(*httpAddr, mux); err != nil { //nolint:gosec // dev tool
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	return srv.ServeStdio()
}

// Server holds the state shared across requests. DB is the SQLite
// connection used by every tool that doesn't open its own; DBPath is
// retained so workflow_run can pass it back to the runner via state.
// Source is the knowledge-store adapter behind FetchSource / AddSource;
// nil disables those tools.
type Server struct {
	DB     *sql.DB
	DBPath string
	Source source.Adapter
}

// rpcRequest is the JSON-RPC 2.0 envelope. Method is the dotted MCP
// method name (initialize / tools.list / tools.call); Params is
// the raw payload we re-decode per method.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// HandleRequest dispatches one JSON-RPC request. Errors are returned as a
// well-formed rpcResponse rather than as Go errors, matching the JSON-RPC
// spec.
func (s *Server) HandleRequest(ctx context.Context, req rpcRequest) rpcResponse {
	switch req.Method {
	case "initialize":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "dear-agent-mcp",
				"version": "0.2.0",
			},
		}}
	case "tools/list":
		tools := toolDescriptors()
		if s.Source != nil {
			tools = append(tools, sourceToolDescriptors()...)
		}
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": tools}}
	case "tools/call":
		return s.handleToolCall(ctx, req)
	default:
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{Code: -32601, Message: "method not found"}}
	}
}

// toolDescriptors returns the JSON shape MCP clients expect from
// tools/list. Schema fields are kept conservative so the same tool string
// works across MCP versions.
func toolDescriptors() []map[string]any {
	return []map[string]any{
		{
			"name":        "workflow_run",
			"description": "Start a new workflow run from a YAML file path. Returns the assigned run_id.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file":   map[string]any{"type": "string", "description": "Path to the workflow YAML"},
					"inputs": map[string]any{"type": "object", "description": "Inputs as a map of name → string"},
				},
				"required": []string{"file"},
			},
		},
		{
			"name":        "workflow_status",
			"description": "Fetch the current state of a workflow run.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run_id": map[string]any{"type": "string"},
				},
				"required": []string{"run_id"},
			},
		},
		{
			"name":        "workflow_approve",
			"description": "Approve a pending HITL approval. Approver and role are recorded for audit.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"approval_id": map[string]any{"type": "string"},
					"approver":    map[string]any{"type": "string"},
					"role":        map[string]any{"type": "string"},
					"reason":      map[string]any{"type": "string"},
				},
				"required": []string{"approval_id"},
			},
		},
		{
			"name":        "workflow_reject",
			"description": "Reject a pending HITL approval; the gated node fails.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"approval_id": map[string]any{"type": "string"},
					"approver":    map[string]any{"type": "string"},
					"role":        map[string]any{"type": "string"},
					"reason":      map[string]any{"type": "string"},
				},
				"required": []string{"approval_id"},
			},
		},
		{
			"name":        "workflow_cancel",
			"description": "Cancel an in-flight workflow run.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"run_id": map[string]any{"type": "string"},
					"reason": map[string]any{"type": "string"},
					"actor":  map[string]any{"type": "string"},
				},
				"required": []string{"run_id"},
			},
		},
	}
}

// toolCallParams is the canonical shape MCP servers receive on tools/call.
type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func (s *Server) handleToolCall(ctx context.Context, req rpcRequest) rpcResponse {
	var p toolCallParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return errResponse(req.ID, -32602, "invalid params", err.Error())
	}
	switch p.Name {
	case "workflow_run":
		return s.toolRun(ctx, req.ID, p.Arguments)
	case "workflow_status":
		return s.toolStatus(ctx, req.ID, p.Arguments)
	case "workflow_approve":
		return s.toolDecide(ctx, req.ID, p.Arguments, workflow.HITLDecisionApprove)
	case "workflow_reject":
		return s.toolDecide(ctx, req.ID, p.Arguments, workflow.HITLDecisionReject)
	case "workflow_cancel":
		return s.toolCancel(ctx, req.ID, p.Arguments)
	case "FetchSource":
		return s.toolFetchSource(ctx, req.ID, p.Arguments)
	case "AddSource":
		return s.toolAddSource(ctx, req.ID, p.Arguments)
	}
	return errResponse(req.ID, -32601, "unknown tool", p.Name)
}

// toolRun returns a deferred-execution stub: an MCP client cannot block
// for the entire run, so we return immediately with the run_id and let
// the client poll via workflow_status. The actual run is dispatched via
// the workflow-run command (kept out of this binary so MCP doesn't
// depend on the LLM provider).
//
// The handler enqueues a record into runs (state=pending) and returns
// the run_id. A separate worker (or a manual `workflow-run` invocation)
// completes the work. This matches the substrate principle: the
// workflow engine owns durable state; ad-hoc transports never block.
func (s *Server) toolRun(ctx context.Context, id any, args json.RawMessage) rpcResponse {
	var a struct {
		File   string            `json:"file"`
		Inputs map[string]string `json:"inputs"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errResponse(id, -32602, "invalid arguments", err.Error())
	}
	if a.File == "" {
		return errResponse(id, -32602, "file is required", nil)
	}
	if _, err := os.Stat(a.File); err != nil {
		return errResponse(id, -32602, "workflow file not found", err.Error())
	}
	// Phase 2 surface: returns a placeholder run id so clients can correlate
	// against workflow_status once a separate worker promotes the run.
	// Phase 4 will wire this directly to the in-process runner.
	runID := fmt.Sprintf("queued-%d", time.Now().UnixNano())
	if err := insertQueuedRun(ctx, s.DB, runID, a.File, a.Inputs); err != nil {
		return errResponse(id, -32000, "queue run", err.Error())
	}
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"run_id":   runID,
		"file":     a.File,
		"queued":   true,
		"poll_for": "workflow_status",
	}}
}

func (s *Server) toolStatus(ctx context.Context, id any, args json.RawMessage) rpcResponse {
	var a struct{ RunID string `json:"run_id"` }
	if err := json.Unmarshal(args, &a); err != nil {
		return errResponse(id, -32602, "invalid arguments", err.Error())
	}
	st, err := workflow.Status(ctx, s.DB, a.RunID)
	if err != nil {
		if errors.Is(err, workflow.ErrRunNotFound) {
			return errResponse(id, -32001, "run not found", a.RunID)
		}
		return errResponse(id, -32000, "status", err.Error())
	}
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: st}
}

func (s *Server) toolDecide(ctx context.Context, id any, args json.RawMessage, dec workflow.HITLDecision) rpcResponse {
	var a struct {
		ApprovalID string `json:"approval_id"`
		Approver   string `json:"approver"`
		Role       string `json:"role"`
		Reason     string `json:"reason"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errResponse(id, -32602, "invalid arguments", err.Error())
	}
	if a.Approver == "" {
		a.Approver = "mcp"
	}
	if err := workflow.RecordHITLDecision(ctx, s.DB, a.ApprovalID, dec, a.Approver, a.Role, a.Reason, time.Now()); err != nil {
		switch {
		case errors.Is(err, workflow.ErrApprovalNotFound):
			return errResponse(id, -32001, "approval not found", a.ApprovalID)
		case errors.Is(err, workflow.ErrApprovalAlreadyResolved):
			return errResponse(id, -32002, "approval already resolved", a.ApprovalID)
		case errors.Is(err, workflow.ErrApproverRoleMismatch):
			return errResponse(id, -32003, "approver role mismatch", err.Error())
		}
		return errResponse(id, -32000, "decide", err.Error())
	}
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"approval_id": a.ApprovalID,
		"decision":    string(dec),
	}}
}

func (s *Server) toolCancel(ctx context.Context, id any, args json.RawMessage) rpcResponse {
	var a struct {
		RunID  string `json:"run_id"`
		Reason string `json:"reason"`
		Actor  string `json:"actor"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return errResponse(id, -32602, "invalid arguments", err.Error())
	}
	if a.Reason == "" {
		a.Reason = "cancelled-via-mcp"
	}
	if a.Actor == "" {
		a.Actor = "mcp"
	}
	if err := workflow.Cancel(ctx, s.DB, a.RunID, a.Reason, a.Actor); err != nil {
		if errors.Is(err, workflow.ErrRunNotFound) {
			return errResponse(id, -32001, "run not found", a.RunID)
		}
		return errResponse(id, -32000, "cancel", err.Error())
	}
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{"cancelled": a.RunID}}
}

// insertQueuedRun records a placeholder run row so workflow_status can
// reflect the request. Real execution is the worker's responsibility.
func insertQueuedRun(ctx context.Context, db *sql.DB, runID, file string, inputs map[string]string) error {
	now := time.Now()
	wfID := "queued:" + file
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO workflows (workflow_id, name, version, yaml_canonical, registered_at)
		VALUES (?, ?, '', '', ?)
		ON CONFLICT (workflow_id) DO NOTHING
	`, wfID, file, now); err != nil {
		return err
	}
	inputsJSON, _ := json.Marshal(inputs)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO runs (run_id, workflow_id, state, inputs_json, started_at, trigger)
		VALUES (?, ?, 'pending', ?, ?, 'mcp')
	`, runID, wfID, string(inputsJSON), now); err != nil {
		return err
	}
	return tx.Commit()
}

// ServeStdio reads JSON-RPC requests one per line from stdin and writes
// responses to stdout. This is the canonical MCP transport.
func (s *Server) ServeStdio() int {
	in := bufio.NewReader(os.Stdin)
	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	for {
		line, err := in.ReadBytes('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0
			}
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		line = trimNewline(line)
		if len(line) == 0 {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(out, errResponse(nil, -32700, "parse error", err.Error()))
			continue
		}
		resp := s.HandleRequest(context.Background(), req)
		writeResponse(out, resp)
	}
}

// ServeHTTP is the test/debug surface. Each request is one JSON-RPC
// envelope; the response is exactly one envelope.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		_ = json.NewEncoder(w).Encode(errResponse(nil, -32700, "parse error", err.Error()))
		return
	}
	resp := s.HandleRequest(r.Context(), req)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func writeResponse(w *bufio.Writer, r rpcResponse) {
	b, err := json.Marshal(r)
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal response:", err)
		return
	}
	_, _ = w.Write(b)
	_, _ = w.WriteString("\n")
	_ = w.Flush()
}

func errResponse(id any, code int, msg string, data any) rpcResponse {
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: msg, Data: data}}
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}

func openDB(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(on)")
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, nil
}

// strings imported to keep the package import list visible after future edits.
var _ = strings.TrimSpace
