package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vbonnet/dear-agent/agm/internal/dolt"
	"github.com/vbonnet/dear-agent/agm/internal/ops"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// injectTraceContext injects W3C trace context into an MCP _meta map.
func injectTraceContext(ctx context.Context) map[string]interface{} {
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	meta := map[string]interface{}{}
	if tp := carrier.Get("traceparent"); tp != "" {
		meta["traceparent"] = tp
	}
	return meta
}

// --- Input/Output structures ---

type ListSessionsInput struct {
	Filters struct {
		Status    string `json:"status,omitempty" jsonschema:"Filter by status: active (default), archived, or all"`
		AgentType string `json:"agent_type,omitempty" jsonschema:"Filter by agent type: claude-code, gemini-cli, or all"`
		Limit     int    `json:"limit,omitempty" jsonschema:"Maximum sessions to return (1-1000, default 100)"`
	} `json:"filters"`
	Fields []string `json:"fields,omitempty" jsonschema:"Field mask: only return these fields (e.g. [id, name, status]). Omit for all fields."`
}

type SearchSessionsInput struct {
	Query   string `json:"query" jsonschema:"Search query for session names (case-insensitive)"`
	Filters struct {
		Status string `json:"status,omitempty" jsonschema:"Filter by status: active (default), archived, or all"`
		Limit  int    `json:"limit,omitempty" jsonschema:"Maximum results (1-50, default 10)"`
	} `json:"filters"`
}

type GetSessionInput struct {
	Identifier string `json:"identifier" jsonschema:"Session ID, name, or UUID prefix"`
}

// --- Tool registration ---

func addListSessionsTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_list_sessions",
		Description: "List AGM sessions. Use when you need to see all active sessions or find sessions by status/type.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input ListSessionsInput) (*mcp.CallToolResult, any, error) {
		opCtx, cleanup, err := newMCPOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.Fields = input.Fields

		result, opErr := ops.ListSessions(opCtx, &ops.ListSessionsRequest{
			Status:  input.Filters.Status,
			Harness: input.Filters.AgentType,
			Limit:   input.Filters.Limit,
		})
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

func addSearchSessionsTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_search_sessions",
		Description: "Search AGM sessions by name. Use when you need to find a specific session by partial name match.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input SearchSessionsInput) (*mcp.CallToolResult, any, error) {
		if input.Query == "" {
			return mcpError(ops.ErrInvalidInput("query", "Search query is required.")), nil, nil
		}

		opCtx, cleanup, err := newMCPOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()

		result, opErr := ops.SearchSessions(opCtx, &ops.SearchSessionsRequest{
			Query:  input.Query,
			Status: input.Filters.Status,
			Limit:  input.Filters.Limit,
		})
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

func addGetSessionMetadataTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_get_session",
		Description: "Get detailed metadata for an AGM session. Use when you need full session details by ID or name.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input GetSessionInput) (*mcp.CallToolResult, any, error) {
		if input.Identifier == "" {
			return mcpError(ops.ErrInvalidInput("identifier", "Session identifier is required.")), nil, nil
		}

		opCtx, cleanup, err := newMCPOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()

		result, opErr := ops.GetSession(opCtx, &ops.GetSessionRequest{
			Identifier: input.Identifier,
		})
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

// --- MCP helpers ---

// newMCPOpContext creates an OpContext with Dolt storage for MCP tool handlers.
// Returns a cleanup function that must be deferred.
func newMCPOpContext() (*ops.OpContext, func(), error) {
	doltCfg, err := dolt.DefaultConfig()
	if err != nil {
		return nil, func() {}, fmt.Errorf("dolt config: %w", err)
	}

	adapter, err := dolt.New(doltCfg)
	if err != nil {
		return nil, func() {}, fmt.Errorf("dolt connect: %w", err)
	}

	cleanup := func() { adapter.Close() }

	return &ops.OpContext{
		Storage:    adapter,
		OutputMode: "json",
	}, cleanup, nil
}

// mcpSuccess formats an ops result as an MCP success response.
func mcpSuccess(result any) *mcp.CallToolResult {
	data, _ := json.Marshal(result)
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: string(data)}},
	}
}

// mcpError formats an error as an MCP error response.
// If the error is an OpError, includes the full RFC 7807 JSON.
func mcpError(err error) *mcp.CallToolResult {
	var text string
	opErr := &ops.OpError{}
	if errors.As(err, &opErr) {
		text = string(opErr.JSON())
	} else {
		text = err.Error()
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
		IsError: true,
	}
}


// --- Mutation tools ---

type ArchiveSessionInput struct {
	Identifier string `json:"identifier" jsonschema:"Session ID, name, or tmux session name to archive"`
	DryRun     bool   `json:"dry_run,omitempty" jsonschema:"Preview the archive without executing. Returns what would happen."`
}

type KillSessionInput struct {
	Identifier string `json:"identifier" jsonschema:"Session ID, name, or tmux session name to kill"`
	DryRun     bool   `json:"dry_run,omitempty" jsonschema:"Preview the kill without executing. Returns what would happen."`
}

func addArchiveSessionTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_archive_session",
		Description: "Archive an AGM session by marking it as archived. Use when a session is no longer needed and should be hidden from the active list.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input ArchiveSessionInput) (*mcp.CallToolResult, any, error) {
		if input.Identifier == "" {
			return mcpError(ops.ErrInvalidInput("identifier", "Session identifier is required.")), nil, nil
		}

		opCtx, cleanup, err := newMCPOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.DryRun = input.DryRun

		result, opErr := ops.ArchiveSession(opCtx, &ops.ArchiveSessionRequest{
			Identifier: input.Identifier,
		})
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

func addKillSessionTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_kill_session",
		Description: "Kill the tmux session for an AGM session. Use when a session is stuck or unresponsive and needs to be force-stopped.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, input KillSessionInput) (*mcp.CallToolResult, any, error) {
		if input.Identifier == "" {
			return mcpError(ops.ErrInvalidInput("identifier", "Session identifier is required.")), nil, nil
		}

		opCtx, cleanup, err := newMCPOpContext()
		if err != nil {
			return mcpError(err), nil, nil
		}
		defer cleanup()
		opCtx.DryRun = input.DryRun

		result, opErr := ops.KillSession(opCtx, &ops.KillSessionRequest{
			Identifier: input.Identifier,
		})
		if opErr != nil {
			return mcpError(opErr), nil, nil
		}

		return mcpSuccess(result), result, nil
	})
}

// --- Schema introspection ---

type ListOpsInput struct{}

func addListOpsTool(server *mcp.Server, _ *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_list_ops",
		Description: "List all available AGM operations. Use for schema discovery and to see what tools are available.",
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ ListOpsInput) (*mcp.CallToolResult, any, error) {
		result := ops.ListOps()
		return mcpSuccess(result), result, nil
	})
}

// --- Wayfinder MCP tool forwarding (Phase 7.1) ---

type ListWayfinderSessionsInput struct {
	StatusFilter string `json:"status_filter,omitempty" jsonschema:"Filter by status: active, completed, failed, abandoned"`
	Limit        int    `json:"limit,omitempty" jsonschema:"Maximum sessions to return (max 1000, default 100)"`
}

type GetWayfinderSessionInput struct {
	SessionID string `json:"session_id" jsonschema:"Session UUID"`
}

func addListWayfinderSessionsTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "engram_list_wayfinder_sessions",
		Description: "List Wayfinder sessions from Engram. Use when checking status of SDLC projects.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ListWayfinderSessionsInput) (*mcp.CallToolResult, interface{}, error) {
		result, err := forwardToEngramMCP(ctx, "engram_list_wayfinder_sessions", map[string]interface{}{
			"status_filter": input.StatusFilter,
			"limit":         input.Limit,
		}, cfg)
		if err != nil {
			return mcpError(err), nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: result}},
		}, nil, nil
	})
}

func addGetWayfinderSessionTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "engram_get_wayfinder_session",
		Description: "Get detailed Wayfinder session info by ID. Use when you need phase status for a specific project.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input GetWayfinderSessionInput) (*mcp.CallToolResult, interface{}, error) {
		if input.SessionID == "" {
			return mcpError(ops.ErrInvalidInput("session_id", "Session ID is required.")), nil, nil
		}
		result, err := forwardToEngramMCP(ctx, "engram_get_wayfinder_session", map[string]interface{}{
			"session_id": input.SessionID,
		}, cfg)
		if err != nil {
			return mcpError(err), nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: result}},
		}, nil, nil
	})
}

// forwardToEngramMCP forwards MCP tool call to Engram MCP server via HTTP
func forwardToEngramMCP(ctx context.Context, toolName string, arguments map[string]interface{}, cfg *Config) (string, error) {
	engramURL := cfg.EngramMCPURL
	if engramURL == "" {
		engramURL = "http://localhost:8081"
	}

	// Inject W3C trace context into _meta for downstream propagation
	meta := injectTraceContext(ctx)

	mcpParams := map[string]interface{}{
		"name":      toolName,
		"arguments": arguments,
	}
	if len(meta) > 0 {
		mcpParams["_meta"] = meta
	}

	mcpRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params":  mcpParams,
	}

	requestBody, err := json.Marshal(mcpRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(httpCtx, http.MethodPost, engramURL, bytes.NewReader(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w (is Engram MCP server running at %s?)", err, engramURL)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(responseBody))
	}

	var mcpResponse struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(responseBody, &mcpResponse); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if mcpResponse.Error.Code != 0 {
		return "", fmt.Errorf("Engram MCP error %d: %s", mcpResponse.Error.Code, mcpResponse.Error.Message)
	}

	if len(mcpResponse.Result.Content) == 0 {
		return "", fmt.Errorf("empty response from Engram MCP server")
	}

	return mcpResponse.Result.Content[0].Text, nil
}
