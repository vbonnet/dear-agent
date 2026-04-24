package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// ListSessions lists AGM sessions with filters.
var ListSessions = codegen.Op{
	Name:         "list_sessions",
	Description:  "List AGM sessions with filters",
	Category:     codegen.CategoryRead,
	RequestType:  "ListSessionsRequest",
	ResponseType: "ListSessionsResult",
	HandlerFunc:  "ListSessions",
	CLI: &codegen.CLISurface{
		CommandPath:   "session list",
		Use:           "list [flags]",
		Aliases:       []string{"ls"},
		OutputFormats: []string{"json", "table"},
	},
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_list_sessions",
		Description: "List AGM sessions. Use when you need to see all active sessions or find sessions by status/type.",
	},
	Skill: &codegen.SkillSurface{
		SlashCommand: "agm-list",
		AllowedTools: "Bash(agm session list:*)",
		ActionVerb:   "list all AGM sessions",
		OutputTable:  []string{"Name", "Status", "Harness", "Project", "Updated"},
	},
}

// ListSessionsRequest mirrors ops.ListSessionsRequest with ef tags for generation.
type ListSessionsRequest struct {
	Status  string `json:"status,omitempty"  ef:"status,short=s,default=active,enum=active|archived|all" desc:"Filter by session status"`
	Harness string `json:"harness,omitempty" ef:"harness,short=H,enum=claude-code|gemini-cli|codex|opencode|all" desc:"Filter by agent type"`
	Limit   int    `json:"limit,omitempty"   ef:"limit,short=n,default=100" desc:"Maximum sessions to return (1-1000)"`
	Offset  int    `json:"offset,omitempty"  ef:"offset,default=0" desc:"Pagination offset"`
}
