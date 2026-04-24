// Package surface defines AGM codegen operation definitions that drive
// CLI, MCP, and Skill surface generation from a single source of truth.
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
