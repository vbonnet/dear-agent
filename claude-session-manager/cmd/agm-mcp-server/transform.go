package main

import (
	"time"

	"github.com/vbonnet/ai-tools/claude-session-manager/internal/manifest"
)

// transformSessionsToMCP converts manifest array to MCP format
func transformSessionsToMCP(manifests []*manifest.Manifest, limit int) *ListSessionsOutput {
	sessions := make([]MCPSessionMetadata, 0, len(manifests))
	totalCount := len(manifests)

	for i, m := range manifests {
		if i >= limit {
			break
		}
		sessions = append(sessions, manifestToMCPMetadata(m))
	}

	return &ListSessionsOutput{
		Sessions:      sessions,
		TotalCount:    totalCount,
		FilteredCount: len(sessions),
	}
}

// manifestToMCPMetadata converts a single manifest to MCP metadata format
func manifestToMCPMetadata(m *manifest.Manifest) MCPSessionMetadata {
	return MCPSessionMetadata{
		ID:          m.SessionID,
		SessionName: m.Name,
		CreatedAt:   m.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   m.UpdatedAt.Format(time.RFC3339),
		Status:      mapLifecycleToStatus(m.Lifecycle),
		AgentType:   "claude", // Hardcode for V1 (AGM is Claude-specific)
		TmuxSession: m.Tmux.SessionName,
		// WorkingDirectory: not in manifest (omit for V1)
		// ConversationCount: not tracked (omit for V1)
	}
}

// mapLifecycleToStatus converts manifest lifecycle to MCP status
func mapLifecycleToStatus(lifecycle string) string {
	if lifecycle == "archived" {
		return "archived"
	}
	return "active" // "" or any other value maps to active
}
