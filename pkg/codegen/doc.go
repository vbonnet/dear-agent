// Package codegen builds the intermediate representation (OpIR) that
// agm/cmd/agm-mcp-server's compiled-contract comparator checks operation
// definitions against; see ADR-002 in agm/cmd/agm-mcp-server/adr. It no
// longer generates source files — the CLI, MCP, skill, and parity emitters
// were retired with the generator that was their only caller.
package codegen
