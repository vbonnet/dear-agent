# Engram — Product Specification

<!-- Last audited at: NEEDS-AUDIT -->

## Overview

Engram is a persistent memory system for AI coding agents. It enables sessions
to store, consolidate, and retrieve memories across conversations using
biologically-inspired retrieval (ecphory) and consolidation (hippocampus).

## Knowledge Model

Engram separates knowledge into two distinct layers (see ADR-009):

- **Documents** — stateless, immutable, *versioned* knowledge blobs (specs,
  architecture docs, research findings, reference material). Trusted as-is;
  editing appends a new version. No decay, merge, or importance scoring.
- **Memories** — mutable, extracted facts distilled from session history.
  Learned, updated in place, merged, decayed, and pruned by consolidation.

Keeping the layers separate sharpens recall precision and prevents stale
extracted facts from contaminating canonical reference knowledge.

## Functional Requirements

### Document Storage (stateless knowledge layer)
- Store versioned documents by stable logical ID; each edit appends an
  immutable new version (prior versions retained)
- Categorize documents by kind (spec, architecture, research, reference, adr)
- Retrieve the latest version, a specific version, or list versions
- List/filter documents within a namespace by kind and title
- Content-hash (SHA-256) every version for integrity and dedup

### Memory Storage (extracted-fact layer)
- Store typed memories (user, feedback, project, reference) with frontmatter
- Index memories for fast retrieval
- Support memory updates and deletion

### Ecphory (Cue-Based Retrieval)
- Accept contextual cues (current task, user prompt, tool results)
- Rank and return relevant memories based on semantic similarity
- Inject retrieved memories into session context via hooks

### Error Memory
- Capture error patterns from failed tool calls
- Recall similar past errors and their resolutions
- Reduce repeated mistakes across sessions

### Hippocampus (Consolidation)
- Periodically consolidate short-term memories into long-term storage
- Merge duplicate or overlapping memories
- Prune stale memories based on access patterns

### Hooks Integration
- `UserPromptSubmit`: Trigger ecphory on user input
- `PostTool`: Capture error patterns after tool failures
- `SessionStart`: Load session-relevant memories

### MCP Server
- Expose memory operations via Model Context Protocol
- Support store, recall, and search operations

## Non-Functional Requirements

- Retrieval latency: < 500ms for ecphory queries
- Storage: File-based (no external database required)
- Compatibility: Works with Claude Code hook system
- Security: No secrets or credentials stored in memories

## Out of Scope

- Cloud-hosted memory synchronization
- Multi-user memory sharing
- Real-time memory streaming
