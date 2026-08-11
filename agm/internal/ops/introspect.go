package ops

// OpInfo describes a single operation for schema discovery.
type OpInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"` // "read", "mutation", "meta"
	Surface     string `json:"surface"`  // "cli,mcp,skill" - which surfaces support it
}

// ListOpsResult is the output of ListOps.
type ListOpsResult struct {
	Operation  string   `json:"operation"`
	Operations []OpInfo `json:"operations"`
	Total      int      `json:"total"`
}

// ListOps returns all registered operations for agent discovery.
func ListOps() *ListOpsResult {
	ops := []OpInfo{
		{Name: "list_sessions", Description: "List sessions with filters. Use when you need to see all active sessions.", Category: "read", Surface: "cli,mcp,skill"},
		{Name: "get_session", Description: "Get detailed session metadata by ID or name. Use when you need full session details.", Category: "read", Surface: "cli,mcp,skill"},
		{Name: "get_session_output", Description: "Read the tail of a session's terminal output — live pane or durable final capture. Use to collect a worker's result.", Category: "read", Surface: "mcp"},
		{Name: "search_sessions", Description: "Search sessions by name with relevance scoring. Use when finding a specific session.", Category: "read", Surface: "cli,mcp,skill"},
		{Name: "get_status", Description: "Get live status of all sessions with summary counts. Use for dashboard views.", Category: "read", Surface: "cli,mcp,skill"},
		{Name: "list_workspaces", Description: "List configured workspaces. Use when checking workspace configuration.", Category: "read", Surface: "cli,mcp"},
		{Name: "list_wayfinder_sessions", Description: "List Wayfinder project sessions. Use when checking SDLC workflow status.", Category: "read", Surface: "mcp"},
		{Name: "get_wayfinder_session", Description: "Get detailed Wayfinder project session metadata. Use when inspecting phase status.", Category: "read", Surface: "mcp"},
		{Name: "create_session", Description: "Create a tmux-backed AGM session for a supported harness. Use when spawning work programmatically.", Category: "mutation", Surface: "cli,mcp"},
		{Name: "archive_session", Description: "Archive a session. Use when a session is no longer needed.", Category: "mutation", Surface: "cli,mcp,skill"},
		{Name: "kill_session", Description: "Terminate a running session. Use when a session needs to be stopped.", Category: "mutation", Surface: "cli,mcp,skill"},
		{Name: "send_message", Description: "Send a message to a session. Use for inter-session communication.", Category: "mutation", Surface: "cli,mcp,skill"},
		{Name: "list_ops", Description: "List all available operations. Use for schema discovery.", Category: "meta", Surface: "mcp"},
	}
	return &ListOpsResult{
		Operation:  "list_ops",
		Operations: ops,
		Total:      len(ops),
	}
}
