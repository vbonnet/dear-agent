// source.go adds the FetchSource / AddSource MCP tools (Phase 3.3).
// They expose pkg/source.Adapter through the same JSON-RPC surface as
// the workflow_* tools so a vanilla MCP client can both index and
// search durable knowledge without shelling out.
//
// The adapter lifecycle is owned by Server: a single Adapter is
// constructed once and shared by every request. The default backend
// is sqlite (pkg/source/sqlite); future backends register via the
// same shape.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/vbonnet/dear-agent/pkg/source"
)

// sourceToolDescriptors returns the tools/list entries for FetchSource
// and AddSource. Kept separate from workflow.go's toolDescriptors so
// each tool family can evolve independently.
func sourceToolDescriptors() []map[string]any {
	return []map[string]any{
		{
			"name":        "FetchSource",
			"description": "Search the knowledge store for sources matching a query. Returns up to k results.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":     map[string]any{"type": "string"},
					"k":         map[string]any{"type": "integer", "description": "max results (default 10)"},
					"cues":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"work_item": map[string]any{"type": "string"},
					"after":     map[string]any{"type": "string", "description": "RFC3339 lower bound on indexed_at"},
					"before":    map[string]any{"type": "string", "description": "RFC3339 upper bound on indexed_at"},
					"backend":   map[string]any{"type": "string", "description": "expected adapter name; mismatch is an error"},
				},
			},
		},
		{
			"name":        "AddSource",
			"description": "Index a source into the knowledge store. Idempotent on uri.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"uri":        map[string]any{"type": "string"},
					"title":      map[string]any{"type": "string"},
					"snippet":    map[string]any{"type": "string"},
					"content":    map[string]any{"type": "string"},
					"cues":       map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"work_item":  map[string]any{"type": "string"},
					"role":       map[string]any{"type": "string"},
					"confidence": map[string]any{"type": "number"},
					"src":        map[string]any{"type": "string", "description": "Metadata.Source"},
					"backend":    map[string]any{"type": "string", "description": "expected adapter name; mismatch is an error"},
				},
				"required": []string{"uri"},
			},
		},
	}
}

// fetchSourceArgs / addSourceArgs are the JSON shapes received from
// the client. Times are accepted as RFC3339 strings.
type fetchSourceArgs struct {
	Query    string   `json:"query"`
	K        int      `json:"k"`
	Cues     []string `json:"cues"`
	WorkItem string   `json:"work_item"`
	After    string   `json:"after"`
	Before   string   `json:"before"`
	Backend  string   `json:"backend"`
}

type addSourceArgs struct {
	URI        string   `json:"uri"`
	Title      string   `json:"title"`
	Snippet    string   `json:"snippet"`
	Content    string   `json:"content"`
	Cues       []string `json:"cues"`
	WorkItem   string   `json:"work_item"`
	Role       string   `json:"role"`
	Confidence float64  `json:"confidence"`
	Source     string   `json:"src"`
	Backend    string   `json:"backend"`
}

// toolFetchSource serves FetchSource. Backend mismatch fails fast with
// -32004 so a misconfigured client (e.g. talking to sqlite while
// expecting obsidian) gets a clear signal rather than silent results.
func (s *Server) toolFetchSource(ctx context.Context, id any, args json.RawMessage) rpcResponse {
	if s.Source == nil {
		return errResponse(id, -32000, "source backend not configured", nil)
	}
	var a fetchSourceArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return errResponse(id, -32602, "invalid arguments", err.Error())
	}
	if a.Backend != "" && a.Backend != s.Source.Name() {
		return errResponse(id, -32004, "backend mismatch", map[string]any{
			"expected": a.Backend, "actual": s.Source.Name(),
		})
	}
	q := source.FetchQuery{
		Query: a.Query,
		K:     a.K,
		Filters: source.Filters{
			Cues:     a.Cues,
			WorkItem: a.WorkItem,
		},
	}
	if a.After != "" {
		t, err := time.Parse(time.RFC3339, a.After)
		if err != nil {
			return errResponse(id, -32602, "invalid 'after' timestamp", err.Error())
		}
		q.Filters.After = &t
	}
	if a.Before != "" {
		t, err := time.Parse(time.RFC3339, a.Before)
		if err != nil {
			return errResponse(id, -32602, "invalid 'before' timestamp", err.Error())
		}
		q.Filters.Before = &t
	}
	got, err := s.Source.Fetch(ctx, q)
	if err != nil {
		return errResponse(id, -32000, "fetch", err.Error())
	}
	out := make([]map[string]any, 0, len(got))
	for _, r := range got {
		out = append(out, sourceToWire(r))
	}
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"backend": s.Source.Name(),
		"sources": out,
	}}
}

// toolAddSource serves AddSource. Backend mismatch is the same
// fail-fast as FetchSource.
func (s *Server) toolAddSource(ctx context.Context, id any, args json.RawMessage) rpcResponse {
	if s.Source == nil {
		return errResponse(id, -32000, "source backend not configured", nil)
	}
	var a addSourceArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return errResponse(id, -32602, "invalid arguments", err.Error())
	}
	if a.URI == "" {
		return errResponse(id, -32602, "uri is required", nil)
	}
	if a.Backend != "" && a.Backend != s.Source.Name() {
		return errResponse(id, -32004, "backend mismatch", map[string]any{
			"expected": a.Backend, "actual": s.Source.Name(),
		})
	}
	src := source.Source{
		URI:     a.URI,
		Title:   a.Title,
		Snippet: a.Snippet,
		Content: []byte(a.Content),
		Metadata: source.Metadata{
			Cues:       a.Cues,
			WorkItem:   a.WorkItem,
			Role:       a.Role,
			Confidence: a.Confidence,
			Source:     a.Source,
		},
	}
	ref, err := s.Source.Add(ctx, src)
	if err != nil {
		if errors.Is(err, source.ErrNotFound) {
			return errResponse(id, -32001, "not found", err.Error())
		}
		return errResponse(id, -32000, "add", err.Error())
	}
	return rpcResponse{JSONRPC: "2.0", ID: id, Result: map[string]any{
		"uri":        ref.URI,
		"backend":    ref.Backend,
		"indexed_at": ref.IndexedAt.UTC().Format(time.RFC3339Nano),
	}}
}

// sourceToWire flattens a source.Source into the JSON-RPC result shape.
// Content is returned as a string so MCP clients (which generally don't
// handle raw bytes well) can render it directly.
func sourceToWire(s source.Source) map[string]any {
	return map[string]any{
		"uri":        s.URI,
		"title":      s.Title,
		"snippet":    s.Snippet,
		"content":    string(s.Content),
		"score":      s.Score,
		"indexed_at": s.IndexedAt.UTC().Format(time.RFC3339Nano),
		"metadata": map[string]any{
			"cues":       s.Metadata.Cues,
			"work_item":  s.Metadata.WorkItem,
			"role":       s.Metadata.Role,
			"confidence": s.Metadata.Confidence,
			"src":        s.Metadata.Source,
			"custom":     s.Metadata.Custom,
		},
	}
}
