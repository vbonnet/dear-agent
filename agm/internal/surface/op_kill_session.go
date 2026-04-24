package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// KillSession terminates a running session's tmux process.
var KillSession = codegen.Op{
	Name:         "kill_session",
	Description:  "Terminate a running session",
	Category:     codegen.CategoryMutation,
	RequestType:  "KillSessionRequest",
	ResponseType: "KillSessionResult",
	HandlerFunc:  "KillSession",
	CLI: &codegen.CLISurface{
		CommandPath:   "session kill",
		Use:           "kill <identifier>",
		OutputFormats: []string{"json", "table"},
	},
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_kill_session",
		Description: "Kill the tmux session for an AGM session. Use when a session is stuck or unresponsive and needs to be force-stopped.",
	},
	// No skill: too destructive for auto-generation.
}

// KillSessionRequest mirrors ops.KillSessionRequest with ef tags for generation.
type KillSessionRequest struct {
	Identifier string `json:"identifier" ef:"identifier,pos=0,required" desc:"Session ID, name, or UUID prefix"`
}
