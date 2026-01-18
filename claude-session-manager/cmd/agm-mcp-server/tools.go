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
		Status    string `json:"status,omitempty"`     // "active", "archived", "all"
		AgentType string `json:"agent_type,omitempty"` // "claude", "all"
		Limit     int    `json:"limit,omitempty"`      // max 1000, default 100
	} `json:"filters"`
}

type ListSessionsOutput struct {
	Sessions      []MCPSessionMetadata `json:"sessions"`
	TotalCount    int                  `json:"total_count"`
	FilteredCount int                  `json:"filtered_count"`
}

type SearchSessionsInput struct {
	Query   string `json:"query"` // required
	Filters struct {
		Status string `json:"status,omitempty"` // "active", "archived", "all"
		Limit  int    `json:"limit,omitempty"`  // max 50, default 10
	} `json:"filters"`
}

type SearchSessionsOutput struct {
	Sessions     []MCPSessionMetadata `json:"sessions"`
	TotalMatches int                  `json:"total_matches"`
}

type GetSessionMetadataInput struct {
	SessionID string `json:"session_id"` // UUID required
}

type GetSessionMetadataOutput struct {
	Session MCPSessionMetadata `json:"session"`
}

type MCPSessionMetadata struct {
	ID           string `json:"id"`
	SessionName  string `json:"session_name"`
	CreatedAt    string `json:"created_at"`     // ISO8601
	UpdatedAt    string `json:"updated_at"`     // ISO8601
	Status       string `json:"status"`         // "active" or "archived"
	AgentType    string `json:"agent_type"`     // "claude" for V1
	TmuxSession  string `json:"tmux_session"`   // tmux session name
	RelevanceScore float64 `json:"relevance_score,omitempty"` // for search results
}

// Tool creation functions

func createListSessionsTool(cfg *Config) *mcp.ServerTool {
	return mcp.NewServerTool[ListSessionsInput, ListSessionsOutput](
		"agm_list_sessions",
		"List AGM sessions with optional filters (status, agent_type, limit)",
		func(ctx context.Context, ss *mcp.ServerSession, params *mcp.CallToolParamsFor[ListSessionsInput]) (*mcp.CallToolResultFor[ListSessionsOutput], error) {
			input := params.Arguments

			// Validate input
			if input.Filters.Limit == 0 {
				input.Filters.Limit = 100 // default
			}
			if input.Filters.Limit > 1000 {
				return errorResponse("limit must be ≤1000"), nil
			}

			// Get sessions (cached)
			sessions, err := listSessionsCached(cfg.SessionsDir)
			if err != nil {
				return errorResponse(fmt.Sprintf("failed to list sessions: %v", err)), nil
			}

			// Apply filters
			filtered := filterSessions(sessions, input.Filters.Status, input.Filters.AgentType)

			// Transform to MCP format
			output := transformSessionsToMCP(filtered, input.Filters.Limit)

			// Return JSON response
			jsonOutput, _ := json.Marshal(output)
			return &mcp.CallToolResultFor[ListSessionsOutput]{
				Content: []mcp.Content{&mcp.TextContent{Text: string(jsonOutput)}},
			}, nil
		},
	)
}

func createSearchSessionsTool(cfg *Config) *mcp.ServerTool {
	return mcp.NewServerTool[SearchSessionsInput, SearchSessionsOutput](
		"agm_search_sessions",
		"Search AGM sessions by name (case-insensitive, sorted by relevance)",
		func(ctx context.Context, ss *mcp.ServerSession, params *mcp.CallToolParamsFor[SearchSessionsInput]) (*mcp.CallToolResultFor[SearchSessionsOutput], error) {
			input := params.Arguments

			// Validate input
			if input.Query == "" {
				return errorResponse("query is required"), nil
			}
			if input.Filters.Limit == 0 {
				input.Filters.Limit = 10 // default
			}
			if input.Filters.Limit > 50 {
				return errorResponse("search limit must be ≤50"), nil
			}

			// Get sessions (cached)
			sessions, err := listSessionsCached(cfg.SessionsDir)
			if err != nil {
				return errorResponse(fmt.Sprintf("failed to search sessions: %v", err)), nil
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

			// Return JSON response
			jsonOutput, _ := json.Marshal(output)
			return &mcp.CallToolResultFor[SearchSessionsOutput]{
				Content: []mcp.Content{&mcp.TextContent{Text: string(jsonOutput)}},
			}, nil
		},
	)
}

func createGetSessionMetadataTool(cfg *Config) *mcp.ServerTool {
	return mcp.NewServerTool[GetSessionMetadataInput, GetSessionMetadataOutput](
		"agm_get_session_metadata",
		"Get detailed metadata for a specific AGM session by ID (UUID)",
		func(ctx context.Context, ss *mcp.ServerSession, params *mcp.CallToolParamsFor[GetSessionMetadataInput]) (*mcp.CallToolResultFor[GetSessionMetadataOutput], error) {
			input := params.Arguments

			// Validate input
			if input.SessionID == "" {
				return errorResponse("session_id is required"), nil
			}

			// TODO: Validate UUID format

			// Get session by ID
			// For V1, we scan all sessions (TODO: optimize with direct manifest read)
			sessions, err := listSessionsCached(cfg.SessionsDir)
			if err != nil {
				return errorResponse(fmt.Sprintf("failed to get session: %v", err)), nil
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
				return errorResponse(fmt.Sprintf("session not found: %s", input.SessionID)), nil
			}

			// Transform to MCP format
			mcpSession := manifestToMCPMetadata(found)

			output := GetSessionMetadataOutput{
				Session: mcpSession,
			}

			// Return JSON response
			jsonOutput, _ := json.Marshal(output)
			return &mcp.CallToolResultFor[GetSessionMetadataOutput]{
				Content: []mcp.Content{&mcp.TextContent{Text: string(jsonOutput)}},
			}, nil
		},
	)
}

// Helper functions

func errorResponse(message string) *mcp.CallToolResultFor[any] {
	return &mcp.CallToolResultFor[any]{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}
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

	// TODO: Sort by relevance (exact match > starts with > contains)
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
