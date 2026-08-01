// Package surface defines AGM logical operation intent for CLI and
// build-ignored MCP reference generation. The provider-visible MCP server is
// hand-registered and contract-tested; installed plugin commands are owned by
// the live Cobra tree.
package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// Registry is the list of all AGM operations for codegen generation.
var Registry = []codegen.Op{
	// Read operations
	ListSessions,
	GetSession,
	SearchSessions,
	GetStatus,

	// Mutation operations
	ArchiveSession,
	KillSession,

	// Meta operations
	ListOps,
}
