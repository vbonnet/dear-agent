// Package main implements the recommendation MCP server. It exposes the
// aggregator (ADR-015) over JSON-RPC so any MCP client can ask
// "what should we work on next?" without knowing the SQLite schema.
//
// Three tools ship in v1 (ADR-016):
//
//	get_signals         — filtered query against the signals store
//	get_recommendations — ranked priority list (top-N weighted scores)
//	get_signal_trends   — time-bucketed aggregation for a single kind
//
// The server speaks JSON-RPC 2.0 over stdio (the standard MCP transport).
// `--http :PORT` switches to a tiny HTTP shim used in tests; production
// deployments stick to stdio because that is what every MCP client speaks.
//
// The dispatch scaffolding mirrors cmd/dear-agent-mcp so a future
// MCP-wide change (capability negotiation, batching) lands in both
// servers via the same edit.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

func main() {
	os.Exit(run())
}

func run() int {
	var (
		dbPath   = flag.String("db", "signals.db", "path to signals.db")
		httpAddr = flag.String("http", "", "if set, serve JSON-RPC over HTTP at this address (default: stdio)")
	)
	flag.Parse()

	store, err := aggregator.OpenSQLiteStore(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() { _ = store.Close() }()

	srv := &Server{Store: store, DBPath: *dbPath}

	if *httpAddr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/jsonrpc", srv.ServeHTTP)
		fmt.Fprintf(os.Stderr, "recommendation-mcp listening on %s\n", *httpAddr)
		if err := http.ListenAndServe(*httpAddr, mux); err != nil { //nolint:gosec // dev tool
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}

	return srv.ServeStdio()
}

// Server holds the state shared across requests. Store is the
// read-only-by-construction handle to signals.db; DBPath is retained
// for diagnostics (returned on initialize) but is never re-opened.
type Server struct {
	Store  aggregator.Store
	DBPath string
}

// rpcRequest is the JSON-RPC 2.0 envelope. Method is the MCP method
// name (initialize / tools/list / tools/call); Params is re-decoded
// per method.
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

// HandleRequest dispatches one JSON-RPC request. Errors are returned as
// well-formed rpcResponses rather than Go errors, matching JSON-RPC.
func (s *Server) HandleRequest(ctx context.Context, req rpcRequest) rpcResponse {
	switch req.Method {
	case "initialize":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "recommendation-mcp",
				"version": "0.1.0",
			},
		}}
	case "tools/list":
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{
			"tools": toolDescriptors(),
		}}
	case "tools/call":
		return s.handleToolCall(ctx, req)
	default:
		return rpcResponse{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{
			Code: -32601, Message: "method not found",
		}}
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
	case "get_signals":
		return s.toolGetSignals(ctx, req.ID, p.Arguments)
	case "get_recommendations":
		return s.toolGetRecommendations(ctx, req.ID, p.Arguments)
	case "get_signal_trends":
		return s.toolGetSignalTrends(ctx, req.ID, p.Arguments)
	}
	return errResponse(req.ID, -32601, "unknown tool", p.Name)
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
	defer func() { _ = r.Body.Close() }()
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
	return rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{
		Code: code, Message: msg, Data: data,
	}}
}

func trimNewline(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == '\r') {
		b = b[:len(b)-1]
	}
	return b
}
