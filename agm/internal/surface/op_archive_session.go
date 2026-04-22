package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// ArchiveSession archives a session by marking it as archived.
var ArchiveSession = codegen.Op{
	Name:         "archive_session",
	Description:  "Archive a session",
	Category:     codegen.CategoryMutation,
	RequestType:  "ArchiveSessionRequest",
	ResponseType: "ArchiveSessionResult",
	HandlerFunc:  "ArchiveSession",
	ManualSkill:  true,
	CLI: &codegen.CLISurface{
		CommandPath:   "session archive",
		Use:           "archive <identifier>",
		OutputFormats: []string{"json", "table"},
	},
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_archive_session",
		Description: "Archive an AGM session by marking it as archived. Use when a session is no longer needed and should be hidden from the active list.",
	},
	Skill: &codegen.SkillSurface{
		SlashCommand: "agm-exit",
		AllowedTools: "Bash(agm session archive:*)",
		ActionVerb:   "archive the session",
		OutputTable:  []string{"Name", "PreviousStatus", "Result"},
	},
}

// ArchiveSessionRequest mirrors ops.ArchiveSessionRequest with ef tags for generation.
type ArchiveSessionRequest struct {
	Identifier string `json:"identifier" ef:"identifier,pos=0,required" desc:"Session ID, name, or UUID prefix"`
}
