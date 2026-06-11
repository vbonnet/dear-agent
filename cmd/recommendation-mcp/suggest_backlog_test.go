package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// callToolRaw sends a tools/call with raw bytes as arguments — used to test
// malformed input without going through json.Marshal.
func callToolRaw(t *testing.T, srv *Server, name string, rawArgs json.RawMessage) rpcResponse {
	t.Helper()
	params := toolCallParams{Name: name, Arguments: rawArgs}
	pBytes, _ := json.Marshal(params)
	return srv.HandleRequest(context.Background(), rpcRequest{
		JSONRPC: "2.0", ID: 1, Method: "tools/call",
		Params: pBytes,
	})
}

// suggestResult type-asserts the result of a suggest_backlog call.
// Tests call HandleRequest directly without JSON round-trip, so numeric
// types are Go-native (int, float64) not always float64.
func suggestResult(t *testing.T, resp rpcResponse) (suggested, blocked []map[string]any, total int) {
	t.Helper()
	res := resp.Result.(map[string]any)
	if v, ok := res["suggested"]; ok && v != nil {
		suggested = v.([]map[string]any)
	}
	if v, ok := res["blocked"]; ok && v != nil {
		blocked = v.([]map[string]any)
	}
	total, _ = res["total"].(int)
	return
}

func TestSuggestBacklog_MissingFile_EmptyResult(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "suggest_backlog", map[string]any{
		"paths": []string{filepath.Join(t.TempDir(), "no-such.md")},
	})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	suggested, _, total := suggestResult(t, resp)
	if total != 0 {
		t.Errorf("total=%d, want 0", total)
	}
	if len(suggested) != 0 {
		t.Errorf("suggested non-empty for missing file")
	}
}

func TestSuggestBacklog_EmptyArgs_UsesDefaults(t *testing.T) {
	srv, _ := newTestServer(t)
	// No BACKLOG.md in test dir — should succeed with empty result.
	resp := callTool(t, srv, "suggest_backlog", map[string]any{})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if _, ok := res["suggested"]; !ok {
		t.Error("result missing 'suggested' key")
	}
	if _, ok := res["blocked"]; !ok {
		t.Error("result missing 'blocked' key")
	}
	if _, ok := res["total"]; !ok {
		t.Error("result missing 'total' key")
	}
}



func TestSuggestBacklog_WithMarkdown_ReturnsSuggestions(t *testing.T) {
	backlogMD := `## Phase 1

| # | Title | Status | Size | Deps |
|---|-------|--------|------|------|
| 1.1 | First task | pending | S | |
| 1.2 | Second task | pending | M | |
| 1.3 | Third task | in-flight | S | |
| 1.4 | Fourth task | done | S | |
`
	dir := t.TempDir()
	path := filepath.Join(dir, "BACKLOG.md")
	if err := os.WriteFile(path, []byte(backlogMD), 0644); err != nil {
		t.Fatal(err)
	}

	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "suggest_backlog", map[string]any{
		"paths":    []string{path},
		"capacity": 2,
	})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	suggested, _, total := suggestResult(t, resp)
	// Only pending items with no unmet deps are eligible.
	// 1.1 and 1.2 are pending; 1.3 is in-flight; 1.4 is done.
	if len(suggested) != 2 {
		t.Errorf("suggested len=%d, want 2", len(suggested))
	}
	if total != 4 {
		t.Errorf("total=%d, want 4", total)
	}
	// Verify wire shape for first suggestion.
	if len(suggested) > 0 {
		s := suggested[0]
		if _, ok := s["id"]; !ok {
			t.Error("suggestion missing 'id'")
		}
		if _, ok := s["title"]; !ok {
			t.Error("suggestion missing 'title'")
		}
		if _, ok := s["score"]; !ok {
			t.Error("suggestion missing 'score'")
		}
	}
}

func TestSuggestBacklog_CapacityOne_ReturnsOneItem(t *testing.T) {
	backlogMD := `## Phase 1

| # | Title | Status |
|---|-------|--------|
| 1.1 | Task A | pending |
| 1.2 | Task B | pending |
`
	dir := t.TempDir()
	path := filepath.Join(dir, "BACKLOG.md")
	if err := os.WriteFile(path, []byte(backlogMD), 0644); err != nil {
		t.Fatal(err)
	}

	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "suggest_backlog", map[string]any{
		"paths":    []string{path},
		"capacity": 1,
	})
	if resp.Error != nil {
		t.Fatalf("error: %+v", resp.Error)
	}
	suggested, _, _ := suggestResult(t, resp)
	if len(suggested) != 1 {
		t.Errorf("suggested len=%d, want 1 (capacity enforced)", len(suggested))
	}
}

func TestSuggestBacklog_InvalidArgs_Returns32602(t *testing.T) {
	srv, _ := newTestServer(t)
	// Pass something that can't be unmarshalled into suggestBacklogArgs.
	resp := callToolRaw(t, srv, "suggest_backlog", []byte(`not-json`))
	if resp.Error == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}
