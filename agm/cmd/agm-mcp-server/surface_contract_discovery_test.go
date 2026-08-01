package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listToolsSnapshot struct {
	result *mcp.ListToolsResult
	err    error
}

type listToolsCaptureTransport struct {
	inner     mcp.Transport
	snapshots chan<- listToolsSnapshot
}

func captureListedTools(inner mcp.Transport) (mcp.Transport, <-chan listToolsSnapshot) {
	snapshots := make(chan listToolsSnapshot, 1)
	return &listToolsCaptureTransport{inner: inner, snapshots: snapshots}, snapshots
}

func (t *listToolsCaptureTransport) Connect(ctx context.Context) (mcp.Connection, error) {
	connection, err := t.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &listToolsCaptureConnection{
		Connection: connection,
		snapshots:  t.snapshots,
		pending:    make(map[string]struct{}),
	}, nil
}

type listToolsCaptureConnection struct {
	mcp.Connection
	snapshots chan<- listToolsSnapshot
	mu        sync.Mutex
	pending   map[string]struct{}
}

func (c *listToolsCaptureConnection) Write(ctx context.Context, message jsonrpc.Message) error {
	request, capture := message.(*jsonrpc.Request)
	capture = capture && request.Method == "tools/list" && request.ID.IsValid()
	var key string
	if capture {
		key = jsonRPCIDKey(request.ID)
		c.mu.Lock()
		c.pending[key] = struct{}{}
		c.mu.Unlock()
	}
	if err := c.Connection.Write(ctx, message); err != nil {
		if capture {
			c.mu.Lock()
			delete(c.pending, key)
			c.mu.Unlock()
		}
		return err
	}
	return nil
}

func (c *listToolsCaptureConnection) Read(ctx context.Context) (jsonrpc.Message, error) {
	message, err := c.Connection.Read(ctx)
	if err != nil {
		return nil, err
	}
	response, ok := message.(*jsonrpc.Response)
	if !ok || !c.takePending(response.ID) {
		return message, nil
	}
	snapshot := decodeListToolsSnapshot(response)
	select {
	case c.snapshots <- snapshot:
		return message, nil
	default:
		return nil, fmt.Errorf("tools/list wire snapshot buffer is full")
	}
}

func (c *listToolsCaptureConnection) takePending(id jsonrpc.ID) bool {
	key := jsonRPCIDKey(id)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.pending[key]; !ok {
		return false
	}
	delete(c.pending, key)
	return true
}

func jsonRPCIDKey(id jsonrpc.ID) string {
	raw := id.Raw()
	return fmt.Sprintf("%T:%v", raw, raw)
}

func decodeListToolsSnapshot(response *jsonrpc.Response) listToolsSnapshot {
	if response.Error != nil {
		return listToolsSnapshot{err: fmt.Errorf("tools/list wire response: %w", response.Error)}
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Result))
	decoder.UseNumber()
	var result mcp.ListToolsResult
	if err := decoder.Decode(&result); err != nil {
		return listToolsSnapshot{err: fmt.Errorf("decode tools/list wire result: %w", err)}
	}
	tools, err := cloneToolsWithRawSchemas(result.Tools)
	if err != nil {
		return listToolsSnapshot{err: err}
	}
	result.Tools = tools
	return listToolsSnapshot{result: &result}
}

func cloneToolsWithRawSchemas(tools []*mcp.Tool) ([]*mcp.Tool, error) {
	result := make([]*mcp.Tool, len(tools))
	for index, tool := range tools {
		if tool == nil {
			return nil, fmt.Errorf("nil tool at index %d", index)
		}
		schema, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("marshal schema for %s: %w", tool.Name, err)
		}
		clone := *tool
		clone.InputSchema = json.RawMessage(schema)
		result[index] = &clone
	}
	return result, nil
}

func takeListedToolsSnapshot(t *testing.T, snapshots <-chan listToolsSnapshot) *mcp.ListToolsResult {
	t.Helper()
	select {
	case snapshot := <-snapshots:
		if snapshot.err != nil {
			t.Fatalf("capture tools/list wire result: %v", snapshot.err)
		}
		if snapshot.result == nil {
			t.Fatal("tools/list wire snapshot has a nil result")
		}
		return snapshot.result
	default:
		t.Fatal("tools/list completed without a wire response snapshot")
		return nil
	}
}

func schemaAfterGenericDecode(schema any) ([]byte, error) {
	encoded, err := json.Marshal(schema)
	if err != nil {
		return nil, fmt.Errorf("marshal schema: %w", err)
	}
	var generic any
	if err := json.Unmarshal(encoded, &generic); err != nil {
		return nil, fmt.Errorf("decode generic schema: %w", err)
	}
	normalized, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("marshal generic schema: %w", err)
	}
	return normalized, nil
}

func reconcileListedTools(t *testing.T, clientTools, rawSchemaTools []*mcp.Tool) []*mcp.Tool {
	t.Helper()
	rawByName := make(map[string]*mcp.Tool, len(rawSchemaTools))
	for index, tool := range rawSchemaTools {
		if tool == nil {
			t.Fatalf("nil wire tool snapshot at index %d", index)
		}
		if _, duplicate := rawByName[tool.Name]; duplicate {
			t.Fatalf("duplicate wire tool snapshot %q", tool.Name)
		}
		rawByName[tool.Name] = tool
	}
	result := make([]*mcp.Tool, len(clientTools))
	for index, clientTool := range clientTools {
		if clientTool == nil {
			t.Fatalf("nil client-discovered tool at index %d", index)
		}
		rawTool, ok := rawByName[clientTool.Name]
		if !ok {
			t.Fatalf("client-discovered tool %q missing from wire snapshot", clientTool.Name)
		}
		if clientTool.Description != rawTool.Description {
			t.Fatalf("tool %q description diverged across discovery: client=%q wire=%q",
				clientTool.Name, clientTool.Description, rawTool.Description)
		}
		// The SDK client exposes InputSchema as map[string]any, so JSON numbers may
		// already be float64 values. This comparison detects nonnumeric client
		// decoding drift. The exact contract below remains the schema captured from
		// the wire response, whose decoder uses json.Number.
		clientSchema, err := schemaAfterGenericDecode(clientTool.InputSchema)
		if err != nil {
			t.Fatalf("normalize client schema for %q: %v", clientTool.Name, err)
		}
		rawSchema, err := schemaAfterGenericDecode(rawTool.InputSchema)
		if err != nil {
			t.Fatalf("normalize server schema for %q: %v", clientTool.Name, err)
		}
		if !bytes.Equal(clientSchema, rawSchema) {
			t.Fatalf("DAH-002/discovery-schema-drift: tool %q client=%s wire=%s",
				clientTool.Name, clientSchema, rawSchema)
		}
		clone := *clientTool
		clone.InputSchema = rawTool.InputSchema
		result[index] = &clone
		delete(rawByName, clientTool.Name)
	}
	if len(rawByName) != 0 {
		t.Fatalf("wire tools missing from client discovery: %v", sortedKeys(rawByName))
	}
	return result
}

func reconcileNextCursor(client, wire string) (string, error) {
	if client != wire {
		return "", fmt.Errorf("DAH-002/discovery-cursor-drift: client=%q wire=%q", client, wire)
	}
	return wire, nil
}

func TestSchemaAfterGenericDecodePreservesDiscoveryComparison(t *testing.T) {
	raw := json.RawMessage(
		`{"type":"object","properties":{"value":{"const":9007199254740993,"description":"same"}}}`,
	)
	var client any
	if err := json.Unmarshal(raw, &client); err != nil {
		t.Fatalf("decode client fixture: %v", err)
	}
	rawSchema, err := schemaAfterGenericDecode(raw)
	if err != nil {
		t.Fatalf("normalize raw fixture: %v", err)
	}
	clientSchema, err := schemaAfterGenericDecode(client)
	if err != nil {
		t.Fatalf("normalize client fixture: %v", err)
	}
	if !bytes.Equal(clientSchema, rawSchema) {
		t.Fatalf("generic number normalization diverged: client=%s server=%s", clientSchema, rawSchema)
	}

	var changed any
	if err := json.Unmarshal([]byte(
		`{"type":"object","properties":{"value":{"const":9007199254740993,"description":"changed"}}}`,
	), &changed); err != nil {
		t.Fatalf("decode changed fixture: %v", err)
	}
	changedSchema, err := schemaAfterGenericDecode(changed)
	if err != nil {
		t.Fatalf("normalize changed fixture: %v", err)
	}
	if bytes.Equal(changedSchema, rawSchema) {
		t.Fatal("nonnumeric discovery drift was hidden by generic number normalization")
	}
}

func TestReconcileNextCursorRejectsDiscoveryDrift(t *testing.T) {
	if cursor, err := reconcileNextCursor("next", "next"); err != nil || cursor != "next" {
		t.Fatalf("reconcileNextCursor(equal) = %q, %v", cursor, err)
	}
	if _, err := reconcileNextCursor("client", "wire"); err == nil ||
		!strings.Contains(err.Error(), "DAH-002/discovery-cursor-drift") {
		t.Fatalf("reconcileNextCursor(drift) error = %v, want stable drift key", err)
	}
}

func TestRegisteredMCPToolsPreservesWirePagination(t *testing.T) {
	want := map[string]bool{"one": true, "two": true, "three": true}
	tools, pages := registeredMCPToolsWithOptions(t, &mcp.ServerOptions{PageSize: 1},
		func(server *mcp.Server, _ *Config) {
			for name := range want {
				server.AddTool(&mcp.Tool{
					Name:        name,
					InputSchema: json.RawMessage(`{"type":"object"}`),
				}, nil)
			}
		})
	if pages != len(want) {
		t.Fatalf("discovery pages = %d, want %d", pages, len(want))
	}
	seen := make(map[string]bool, len(tools))
	for _, tool := range tools {
		if tool == nil {
			t.Fatal("discovery returned a nil tool")
		}
		if seen[tool.Name] {
			t.Fatalf("discovery returned duplicate tool %q", tool.Name)
		}
		seen[tool.Name] = true
	}
	if len(seen) != len(want) {
		t.Fatalf("discovered tools = %v, want %v", sortedKeys(seen), sortedKeys(want))
	}
	for name := range want {
		if !seen[name] {
			t.Fatalf("discovered tools = %v, want %v", sortedKeys(seen), sortedKeys(want))
		}
	}
}
