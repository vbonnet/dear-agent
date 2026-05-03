package main

import (
	"testing"
	"time"
)

func TestMCP_AddSource_RoundTrip(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "AddSource", map[string]any{
		"uri":       "https://example.com/doc-1",
		"title":     "MCP test source",
		"snippet":   "snippet",
		"content":   "the quick brown fox jumps",
		"cues":      []string{"animals", "test"},
		"work_item": "run-1/n1",
	})
	if resp.Error != nil {
		t.Fatalf("AddSource error: %+v", resp.Error)
	}
	res := resp.Result.(map[string]any)
	if res["uri"] != "https://example.com/doc-1" {
		t.Errorf("uri = %v", res["uri"])
	}
	if res["backend"] != "sqlite" {
		t.Errorf("backend = %v, want sqlite", res["backend"])
	}

	resp = callTool(t, srv, "FetchSource", map[string]any{
		"query": "fox",
		"k":     5,
	})
	if resp.Error != nil {
		t.Fatalf("FetchSource error: %+v", resp.Error)
	}
	res = resp.Result.(map[string]any)
	sources := res["sources"].([]map[string]any)
	if len(sources) != 1 {
		t.Fatalf("got %d sources, want 1", len(sources))
	}
	if sources[0]["uri"] != "https://example.com/doc-1" {
		t.Errorf("Fetch uri = %v", sources[0]["uri"])
	}
}

func TestMCP_AddSource_RequiresURI(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "AddSource", map[string]any{"title": "no uri"})
	if resp.Error == nil {
		t.Fatal("expected error when uri is missing")
	}
	if resp.Error.Code != -32602 {
		t.Errorf("code=%d, want -32602", resp.Error.Code)
	}
}

func TestMCP_AddSource_BackendMismatch(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "AddSource", map[string]any{
		"uri": "u1", "backend": "obsidian",
	})
	if resp.Error == nil {
		t.Fatal("expected backend-mismatch error")
	}
	if resp.Error.Code != -32004 {
		t.Errorf("code=%d, want -32004", resp.Error.Code)
	}
}

func TestMCP_FetchSource_FiltersByCue(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, args := range []map[string]any{
		{"uri": "u1", "title": "alpha", "content": "alpha doc", "cues": []string{"alpha"}},
		{"uri": "u2", "title": "beta", "content": "beta doc", "cues": []string{"beta"}},
	} {
		resp := callTool(t, srv, "AddSource", args)
		if resp.Error != nil {
			t.Fatalf("AddSource %v: %+v", args["uri"], resp.Error)
		}
	}
	resp := callTool(t, srv, "FetchSource", map[string]any{
		"cues": []string{"alpha"},
	})
	if resp.Error != nil {
		t.Fatalf("FetchSource: %+v", resp.Error)
	}
	sources := resp.Result.(map[string]any)["sources"].([]map[string]any)
	if len(sources) != 1 || sources[0]["uri"] != "u1" {
		t.Fatalf("filter cue=alpha got %v, want u1", sources)
	}
}

func TestMCP_FetchSource_BackendMismatch(t *testing.T) {
	srv, _ := newTestServer(t)
	resp := callTool(t, srv, "FetchSource", map[string]any{"backend": "obsidian"})
	if resp.Error == nil {
		t.Fatal("expected backend-mismatch error")
	}
	if resp.Error.Code != -32004 {
		t.Errorf("code=%d, want -32004", resp.Error.Code)
	}
}

func TestMCP_FetchSource_AfterFilter(t *testing.T) {
	srv, _ := newTestServer(t)
	// Seed via AddSource (indexed_at is set by adapter at insert time);
	// then run with after = now-1h to confirm the filter pipes through.
	resp := callTool(t, srv, "AddSource", map[string]any{
		"uri": "u1", "title": "x", "content": "y",
	})
	if resp.Error != nil {
		t.Fatalf("AddSource: %+v", resp.Error)
	}
	cutoff := time.Now().Add(-time.Hour).Format(time.RFC3339)
	resp = callTool(t, srv, "FetchSource", map[string]any{
		"after": cutoff,
	})
	if resp.Error != nil {
		t.Fatalf("FetchSource after: %+v", resp.Error)
	}
	sources := resp.Result.(map[string]any)["sources"].([]map[string]any)
	if len(sources) != 1 {
		t.Fatalf("after-filter returned %d, want 1", len(sources))
	}
}
