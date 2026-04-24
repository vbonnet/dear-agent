package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// SearchSessions searches sessions by name with relevance scoring.
var SearchSessions = codegen.Op{
	Name:         "search_sessions",
	Description:  "Search sessions by name with relevance scoring",
	Category:     codegen.CategoryRead,
	RequestType:  "SearchSessionsRequest",
	ResponseType: "SearchSessionsResult",
	HandlerFunc:  "SearchSessions",
	CLI: &codegen.CLISurface{
		CommandPath:   "session search",
		Use:           "search <query>",
		OutputFormats: []string{"json", "table"},
	},
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_search_sessions",
		Description: "Search AGM sessions by name. Use when you need to find a specific session by partial name match.",
	},
	Skill: &codegen.SkillSurface{
		SlashCommand: "agm-search",
		AllowedTools: "Bash(agm session search:*)",
		ActionVerb:   "search for matching sessions",
		OutputTable:  []string{"Name", "Status", "Harness", "Score"},
	},
}

// SearchSessionsRequest mirrors ops.SearchSessionsRequest with ef tags for generation.
type SearchSessionsRequest struct {
	Query  string `json:"query"            ef:"query,pos=0,required" desc:"Search query for session names (case-insensitive)"`
	Status string `json:"status,omitempty" ef:"status,short=s,default=active,enum=active|archived|all" desc:"Filter by session status"`
	Limit  int    `json:"limit,omitempty"  ef:"limit,short=n,default=10" desc:"Maximum results to return (1-50)"`
}
