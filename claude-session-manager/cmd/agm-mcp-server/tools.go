package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// Input/Output structures for MCP tools

type ListSessionsInput struct {
	Filters struct {
		Status    string `json:"status,omitempty" jsonschema:"description=Filter by status: active, archived, or all"`
		AgentType string `json:"agent_type,omitempty" jsonschema:"description=Filter by agent type: claude or all"`
		Limit     int    `json:"limit,omitempty" jsonschema:"description=Maximum number of sessions to return (max 1000, default 100)"`
	} `json:"filters"`
}

type ListSessionsOutput struct {
	Sessions      []MCPSessionMetadata `json:"sessions"`
	TotalCount    int                  `json:"total_count"`
	FilteredCount int                  `json:"filtered_count"`
}

type SearchSessionsInput struct {
	Query   string `json:"query" jsonschema:"description=Search query for session names,required"`
	Filters struct {
		Status string `json:"status,omitempty" jsonschema:"description=Filter by status: active, archived, or all"`
		Limit  int    `json:"limit,omitempty" jsonschema:"description=Maximum results (max 50, default 10)"`
	} `json:"filters"`
}

type SearchSessionsOutput struct {
	Sessions     []MCPSessionMetadata `json:"sessions"`
	TotalMatches int                  `json:"total_matches"`
}

type GetSessionMetadataInput struct {
	SessionID string `json:"session_id" jsonschema:"description=Session UUID,required"`
}

type GetSessionMetadataOutput struct {
	Session MCPSessionMetadata `json:"session"`
}

type MCPSessionMetadata struct {
	ID             string  `json:"id"`
	SessionName    string  `json:"session_name"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	Status         string  `json:"status"`
	AgentType      string  `json:"agent_type"`
	TmuxSession    string  `json:"tmux_session"`
	RelevanceScore float64 `json:"relevance_score,omitempty"`
}

// Tool registration functions (v1.2.0 API)

func addListSessionsTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_list_sessions",
		Description: "List AGM sessions with optional filters (status, agent_type, limit)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input ListSessionsInput) (*mcp.CallToolResult, ListSessionsOutput, error) {
		// Validate input
		if input.Filters.Limit == 0 {
			input.Filters.Limit = 100 // default
		}
		if input.Filters.Limit > 1000 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "limit must be ≤1000"}},
				IsError: true,
			}, ListSessionsOutput{}, nil
		}

		// Get sessions (cached)
		sessions, err := listSessionsCached(cfg.SessionsDir)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to list sessions: %v", err)}},
				IsError: true,
			}, ListSessionsOutput{}, nil
		}

		// Apply filters
		filtered := filterSessions(sessions, input.Filters.Status, input.Filters.AgentType)

		// Transform to MCP format
		outputPtr := transformSessionsToMCP(filtered, input.Filters.Limit)
		output := *outputPtr

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}},
		}, output, nil
	})
}

func addSearchSessionsTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_search_sessions",
		Description: "Search AGM sessions by name (case-insensitive, sorted by relevance)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input SearchSessionsInput) (*mcp.CallToolResult, SearchSessionsOutput, error) {
		// Validate input
		if input.Query == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "query is required"}},
				IsError: true,
			}, SearchSessionsOutput{}, nil
		}
		if input.Filters.Limit == 0 {
			input.Filters.Limit = 10 // default
		}
		if input.Filters.Limit > 50 {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "search limit must be ≤50"}},
				IsError: true,
			}, SearchSessionsOutput{}, nil
		}

		// Get sessions (cached)
		sessions, err := listSessionsCached(cfg.SessionsDir)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to search sessions: %v", err)}},
				IsError: true,
			}, SearchSessionsOutput{}, nil
		}

		// Search by name (case-insensitive)
		matches := searchSessionsByName(sessions, input.Query, input.Filters.Status)

		// Limit results
		if len(matches) > input.Filters.Limit {
			matches = matches[:input.Filters.Limit]
		}

		// Transform to MCP format with relevance scores
		mcpSessions := make([]MCPSessionMetadata, 0, len(matches))
		for _, m := range matches {
			mcpSession := manifestToMCPMetadata(m)
			mcpSession.RelevanceScore = calculateRelevance(m.Name, input.Query)
			mcpSessions = append(mcpSessions, mcpSession)
		}

		output := SearchSessionsOutput{
			Sessions:     mcpSessions,
			TotalMatches: len(mcpSessions),
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}},
		}, output, nil
	})
}

func addGetSessionMetadataTool(server *mcp.Server, cfg *Config) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "agm_get_session_metadata",
		Description: "Get detailed metadata for a specific AGM session by ID (UUID)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, input GetSessionMetadataInput) (*mcp.CallToolResult, GetSessionMetadataOutput, error) {
		// Validate input
		if input.SessionID == "" {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "session_id is required"}},
				IsError: true,
			}, GetSessionMetadataOutput{}, nil
		}

		// Get session by ID
		sessions, err := listSessionsCached(cfg.SessionsDir)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("failed to get session: %v", err)}},
				IsError: true,
			}, GetSessionMetadataOutput{}, nil
		}

		// Find session by ID
		var found *manifest.Manifest
		for _, s := range sessions {
			if s.SessionID == input.SessionID {
				found = s
				break
			}
		}

		if found == nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("session not found: %s", input.SessionID)}},
				IsError: true,
			}, GetSessionMetadataOutput{}, nil
		}

		// Transform to MCP format
		mcpSession := manifestToMCPMetadata(found)

		output := GetSessionMetadataOutput{
			Session: mcpSession,
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: formatJSON(output)}},
		}, output, nil
	})
}

// Helper functions

func formatJSON(v interface{}) string {
	jsonBytes, _ := json.Marshal(v)
	return string(jsonBytes)
}

func filterSessions(sessions []*manifest.Manifest, status string, agentType string) []*manifest.Manifest {
	filtered := make([]*manifest.Manifest, 0, len(sessions))
	for _, s := range sessions {
		// Filter by status
		if status != "" && status != "all" {
			sessionStatus := "active"
			if s.Lifecycle == "archived" {
				sessionStatus = "archived"
			}
			if sessionStatus != status {
				continue
			}
		}

		// Filter by agent type (for V1, all are "claude")
		if agentType != "" && agentType != "all" && agentType != "claude" {
			continue
		}

		filtered = append(filtered, s)
	}
	return filtered
}

func searchSessionsByName(sessions []*manifest.Manifest, query string, status string) []*manifest.Manifest {
	queryLower := strings.ToLower(query)
	matches := make([]*manifest.Manifest, 0)

	for _, s := range sessions {
		// Filter by status
		if status != "" && status != "all" {
			sessionStatus := "active"
			if s.Lifecycle == "archived" {
				sessionStatus = "archived"
			}
			if sessionStatus != status {
				continue
			}
		}

		// Search in session name (case-insensitive)
		if strings.Contains(strings.ToLower(s.Name), queryLower) {
			matches = append(matches, s)
		}
	}

	return matches
}

func calculateRelevance(sessionName string, query string) float64 {
	nameLower := strings.ToLower(sessionName)
	queryLower := strings.ToLower(query)

	// Exact match
	if nameLower == queryLower {
		return 1.0
	}

	// Starts with
	if strings.HasPrefix(nameLower, queryLower) {
		return 0.8
	}

	// Contains
	if strings.Contains(nameLower, queryLower) {
		return 0.5
	}

	return 0.0
}
