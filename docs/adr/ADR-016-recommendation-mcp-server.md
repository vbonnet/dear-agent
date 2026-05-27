# ADR-016: Recommendation MCP Server

**Status**: **Superseded by [ADR-015](ADR-015-signal-aggregator.md) Part B**
(2026-05-26).

The recommendation MCP server (`cmd/recommendation-mcp/`) is now
described in [ADR-015 Part B](ADR-015-signal-aggregator.md#part-b--recommendation-mcp-cmdrecommendation-mcp).
ADR-016 was originally split off because it consumed ADR-015's data
layer; the 2026-05-17 inventory prune determined that the split added
no decision content beyond what ADR-015 already commits to (same
SQLite store, same dispatch shape as `dear-agent-mcp`, same release
cadence as the aggregator). Merging removes the parallel doc without
losing any of the substantive choices.

This file is preserved because code references it by section:

| Old anchor (this file) | New anchor (ADR-015) |
|---|---|
| `ADR-016 §D1` (read-only tools) | [ADR-015 Part B](ADR-015-signal-aggregator.md#part-b--recommendation-mcp-cmdrecommendation-mcp) |
| `ADR-016 §D2` (`get_signals`) | [ADR-015 §D-MCP-1](ADR-015-signal-aggregator.md#part-b--recommendation-mcp-cmdrecommendation-mcp) |
| `ADR-016 §D3` (`get_recommendations`) | [ADR-015 §D-MCP-2](ADR-015-signal-aggregator.md#part-b--recommendation-mcp-cmdrecommendation-mcp) |
| `ADR-016 §D4` (`get_signal_trends` bucketing) | [ADR-015 §D-MCP-3](ADR-015-signal-aggregator.md#part-b--recommendation-mcp-cmdrecommendation-mcp) |

New code should reference ADR-015 directly. The existing
`cmd/recommendation-mcp/*.go` comments still cite `ADR-016` and will be
updated opportunistically; both pointers resolve to the same content
while the merge settles.
