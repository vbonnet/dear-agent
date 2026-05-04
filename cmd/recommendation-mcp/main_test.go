package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/vbonnet/dear-agent/pkg/aggregator"
)

// newTestServer opens a fresh signals.db in a temp dir and returns a
// Server pointing at it. The store is closed via t.Cleanup.
func newTestServer(t *testing.T) (*Server, *aggregator.SQLiteStore) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "signals.db")
	store, err := aggregator.OpenSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("OpenSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return &Server{Store: store, DBPath: dbPath}, store
}

// callTool wraps the JSON-RPC envelope so tests can call tools by name.
func callTool(t *testing.T, srv *Server, name string, args map[string]any) rpcResponse {
	t.Helper()
	argBytes, _ := json.Marshal(args)
	params := toolCallParams{Name: name, Arguments: argBytes}
	pBytes, _ := json.Marshal(params)
	return srv.HandleRequest(context.Background(), rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "tools/call",
		Params: pBytes,
	})
}

// insertSignals is a thin convenience wrapper. It assigns IDs so tests
// don't have to keep writing uuid.NewString().
func insertSignals(t *testing.T, store aggregator.Store, sigs ...aggregator.Signal) {
	t.Helper()
	out := make([]aggregator.Signal, len(sigs))
	for i, s := range sigs {
		if s.ID == "" {
			s.ID = uuid.NewString()
		}
		out[i] = s
	}
	if err := store.Insert(context.Background(), out); err != nil {
		t.Fatalf("Insert: %v", err)
	}
}

func TestMCP_Initialize(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := srv.HandleRequest(context.Background(), rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "initialize",
	})
	if resp.Error != nil {
		t.Fatalf("initialize error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v", res["protocolVersion"])
	}
	info := res["serverInfo"].(map[string]any)
	if info["name"] != "recommendation-mcp" {
		t.Errorf("serverInfo.name = %v", info["name"])
	}
}

func TestMCP_ToolsList_HasThreeTools(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := srv.HandleRequest(context.Background(), rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "tools/list",
	})
	if resp.Error != nil {
		t.Fatalf("tools/list error: %+v", resp.Error)
	}
	tools := resp.Result.(map[string]any)["tools"].([]map[string]any)
	if len(tools) != 3 {
		t.Fatalf("got %d tools, want 3", len(tools))
	}
	want := map[string]bool{
		"get_signals":         false,
		"get_recommendations": false,
		"get_signal_trends":   false,
	}
	for _, tool := range tools {
		want[tool["name"].(string)] = true
	}
	for n, found := range want {
		if !found {
			t.Errorf("missing tool %q", n)
		}
	}
}

// Validation property #1: every tool's inputSchema is round-trippable
// JSON. Catches typos like a non-marshalable map or a stray any.
func TestMCP_ToolsList_SchemasRoundTrip(t *testing.T) {
	for _, tool := range toolDescriptors() {
		b, err := json.Marshal(tool["inputSchema"])
		if err != nil {
			t.Fatalf("marshal schema for %s: %v", tool["name"], err)
		}
		var back any
		if err := json.Unmarshal(b, &back); err != nil {
			t.Fatalf("unmarshal schema for %s: %v", tool["name"], err)
		}
	}
}

func TestMCP_UnknownMethod(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := srv.HandleRequest(context.Background(), rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "does/not/exist",
	})
	if resp.Error == nil {
		t.Fatal("expected error for unknown method")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("code=%d, want -32601", resp.Error.Code)
	}
}

func TestMCP_UnknownTool(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "no_such_tool", nil)
	if resp.Error == nil {
		t.Fatal("expected error for unknown tool")
	}
	if resp.Error.Code != -32601 {
		t.Errorf("code=%d, want -32601", resp.Error.Code)
	}
}

// Validation property #8: the server starts cleanly against a path that
// doesn't exist yet, and tools/list still works (the schema is applied
// on open). This is the "fresh deployment" UX.
func TestMCP_FreshDB_ToolsListSucceeds(t *testing.T) {
	dir := t.TempDir()
	store, err := aggregator.OpenSQLiteStore(filepath.Join(dir, "brand-new.db"))
	if err != nil {
		t.Fatalf("OpenSQLiteStore on missing path: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	srv := &Server{Store: store}

	resp := srv.HandleRequest(context.Background(), rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "tools/list",
	})
	if resp.Error != nil {
		t.Fatalf("tools/list on fresh DB: %+v", resp.Error)
	}

	resp = callTool(t, srv, "get_signals", map[string]any{"kind": "lint_trend"})
	if resp.Error != nil {
		t.Fatalf("get_signals on fresh DB: %+v", resp.Error)
	}
	sigs := resp.Result.(map[string]any)["signals"].([]map[string]any)
	if len(sigs) != 0 {
		t.Errorf("fresh DB returned %d signals", len(sigs))
	}
}

// helper: deterministic recent timestamp (5 minutes ago).
func recently() time.Time {
	return time.Now().Add(-5 * time.Minute).UTC()
}
