package main

import (
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

func reconcileListedTools(t *testing.T, clientTools, rawSchemaTools []*mcp.Tool) []*mcp.Tool {
	t.Helper()
	rawByName := make(map[string]*mcp.Tool, len(rawSchemaTools))
	for _, tool := range rawSchemaTools {
		if _, duplicate := rawByName[tool.Name]; duplicate {
			t.Fatalf("duplicate server-side tool snapshot %q", tool.Name)
		}
		rawByName[tool.Name] = tool
	}
	result := make([]*mcp.Tool, len(clientTools))
	for index, clientTool := range clientTools {
		rawTool, ok := rawByName[clientTool.Name]
		if !ok {
			t.Fatalf("client-discovered tool %q missing from server-side snapshot", clientTool.Name)
		}
		if clientTool.Description != rawTool.Description {
			t.Fatalf("tool %q description diverged across discovery: client=%q server=%q",
				clientTool.Name, clientTool.Description, rawTool.Description)
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
