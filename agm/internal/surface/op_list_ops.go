package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// ListOps lists all available operations for agent discovery.
var ListOps = codegen.Op{
	Name:         "list_ops",
	Description:  "List all available operations for schema discovery",
	Category:     codegen.CategoryMeta,
	RequestType:  "ListOpsRequest",
	ResponseType: "ListOpsResult",
	HandlerFunc:  "ListOps",
	// MCP only: introspection endpoint for agent discovery.
	MCP: &codegen.MCPSurface{
		ToolName:    "agm_list_ops",
		Description: "List all available AGM operations. Use for schema discovery and to see what tools are available.",
	},
	// No CLI or Skill: introspection is only useful for MCP consumers.
}

// ListOpsRequest defines the input for list_ops (empty -- no parameters).
type ListOpsRequest struct{}
