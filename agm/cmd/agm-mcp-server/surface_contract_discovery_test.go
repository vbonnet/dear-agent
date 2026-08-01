package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type listToolsSnapshot struct {
	tools []*mcp.Tool
	err   error
}

func captureListedTools() (mcp.Middleware, <-chan listToolsSnapshot) {
	snapshots := make(chan listToolsSnapshot, 1)
	middleware := func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
			result, err := next(ctx, method, request)
			if method != "tools/list" {
				return result, err
			}
			snapshot := listToolsSnapshot{err: err}
			if err == nil {
				listed, ok := result.(*mcp.ListToolsResult)
				if !ok {
					snapshot.err = fmt.Errorf("tools/list result = %T, want *mcp.ListToolsResult", result)
				} else if listed == nil {
					snapshot.err = fmt.Errorf("tools/list result is a nil *mcp.ListToolsResult")
				} else {
					snapshot.tools, snapshot.err = cloneToolsWithRawSchemas(listed.Tools)
				}
			}
			snapshots <- snapshot
			return result, err
		}
	}
	return middleware, snapshots
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

func takeListedToolsSnapshot(t *testing.T, snapshots <-chan listToolsSnapshot) []*mcp.Tool {
	t.Helper()
	select {
	case snapshot := <-snapshots:
		if snapshot.err != nil {
			t.Fatalf("capture raw tools/list schemas: %v", snapshot.err)
		}
		return snapshot.tools
	default:
		t.Fatal("tools/list completed without a server-side schema snapshot")
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
			t.Fatalf("nil server-side tool snapshot at index %d", index)
		}
		if _, duplicate := rawByName[tool.Name]; duplicate {
			t.Fatalf("duplicate server-side tool snapshot %q", tool.Name)
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
			t.Fatalf("client-discovered tool %q missing from server-side snapshot", clientTool.Name)
		}
		if clientTool.Description != rawTool.Description {
			t.Fatalf("tool %q description diverged across discovery: client=%q server=%q",
				clientTool.Name, clientTool.Description, rawTool.Description)
		}
		clientSchema, err := schemaAfterGenericDecode(clientTool.InputSchema)
		if err != nil {
			t.Fatalf("normalize client schema for %q: %v", clientTool.Name, err)
		}
		rawSchema, err := schemaAfterGenericDecode(rawTool.InputSchema)
		if err != nil {
			t.Fatalf("normalize server schema for %q: %v", clientTool.Name, err)
		}
		if !bytes.Equal(clientSchema, rawSchema) {
			t.Fatalf("DAH-002/discovery-schema-drift: tool %q client=%s server=%s",
				clientTool.Name, clientSchema, rawSchema)
		}
		clone := *clientTool
		clone.InputSchema = rawTool.InputSchema
		result[index] = &clone
		delete(rawByName, clientTool.Name)
	}
	if len(rawByName) != 0 {
		t.Fatalf("server-side tools missing from client discovery: %v", sortedKeys(rawByName))
	}
	return result
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
