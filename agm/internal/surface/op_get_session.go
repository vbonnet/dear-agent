package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// GetSession retrieves detailed metadata for a single session.
var GetSession = codegen.Op{
	Name:         "get_session",
	Description:  "Get detailed session metadata by ID or name",
	Category:     codegen.CategoryRead,
	RequestType:  "GetSessionRequest",
	ResponseType: "GetSessionResult",
	HandlerFunc:  "GetSession",
	ManualSkill:  true,
	CLI: &codegen.CLISurface{
		CommandPath:   "session get",
		Use:           "get <identifier>",
		OutputFormats: []string{"json", "table"},
	},
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_get_session",
		Description: "Get detailed metadata for an AGM session. Use when you need full session details by ID or name.",
	},
	Skill: &codegen.SkillSurface{
		SlashCommand: "agm-status",
		AllowedTools: "Bash(agm session get:*)",
		ActionVerb:   "get the session details",
		OutputTable:  []string{"Field", "Value"},
	},
}

// GetSessionRequest mirrors ops.GetSessionRequest with ef tags for generation.
type GetSessionRequest struct {
	Identifier string `json:"identifier" ef:"identifier,pos=0,required" desc:"Session ID, name, or UUID prefix"`
}
