// Package surface defines AGM logical operation intent, consumed only as
// input to the compiled-contract comparator (see ADR-002 in
// agm/cmd/agm-mcp-server/adr). The provider-visible MCP server is
// hand-registered and contract-tested; installed plugin commands are owned by
// the live Cobra tree. This package does not generate CLI, MCP, or parity
// code.
package surface

import "github.com/vbonnet/dear-agent/pkg/codegen"

// Registry is the list of all AGM operations, compared against the compiled
// MCP contract by the surface_contract tests.
var Registry = []codegen.Op{
	// Read operations
	ListSessions,
	GetSession,
	GetSessionOutput,
	SearchSessions,
	GetStatus,
	GetCompletionRelayTarget,
	GetQuotaStatus,

	// Mutation operations
	ArchiveSession,
	KillSession,
	SetCompletionRelayTarget,

	// Meta operations
	ListOps,
}
