package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// GetStatus returns live status of all sessions with summary counts.
var GetStatus = codegen.Op{
	Name:         "get_status",
	Description:  "Get live status of all sessions with summary counts",
	Category:     codegen.CategoryRead,
	RequestType:  "GetStatusRequest",
	ResponseType: "GetStatusResult",
	HandlerFunc:  "GetStatus",
	CLI: &codegen.CLISurface{
		CommandPath:   "session status",
		Use:           "status [flags]",
		OutputFormats: []string{"json", "table"},
	},
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_get_status",
		Description: "Get live status of all AGM sessions with summary counts. Use for dashboard views.",
	},
	Skill: &codegen.SkillSurface{
		SlashCommand: "agm-status",
		AllowedTools: "Bash(agm session status:*)",
		ActionVerb:   "get the current session status",
		OutputTable:  []string{"Name", "Status", "State", "Harness"},
	},
}

// GetStatusRequest mirrors ops.GetStatusRequest with ef tags for generation.
type GetStatusRequest struct {
	IncludeArchived bool `json:"include_archived,omitempty" ef:"include-archived,short=a" desc:"Include archived sessions in the status report"`
}
