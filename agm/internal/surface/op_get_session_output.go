package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// GetSessionOutput reads the tail of a session's terminal output — the live
// tmux pane while the session runs, or the durable final capture after it
// completes. This is the read path orchestrators use to collect worker
// results without attaching to panes.
var GetSessionOutput = codegen.Op{
	Name:         "get_session_output",
	Description:  "Read the tail of a session's terminal output",
	Category:     codegen.CategoryRead,
	RequestType:  "GetSessionOutputRequest",
	ResponseType: "GetSessionOutputResult",
	HandlerFunc:  "GetSessionOutput",
	// MCP only: the CLI already exposes pane reads via `agm capture`.
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_get_session_output",
		Description: "Read the tail of an AGM session's terminal output — live pane while running, durable final capture after completion. Use to collect a worker's result without attaching to its pane.",
	},
}

// GetSessionOutputRequest mirrors ops.GetSessionOutputRequest with ef tags.
type GetSessionOutputRequest struct {
	Identifier string `json:"identifier" ef:"identifier,pos=0,required" desc:"Session ID, name, or UUID prefix"`
	Lines      int    `json:"lines,omitempty" ef:"lines" desc:"Trailing pane lines to capture (default 100, max 2000)"`
}
