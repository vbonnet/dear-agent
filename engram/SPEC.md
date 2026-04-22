# Engram — Product Specification

## Overview

Engram is a persistent memory system for AI coding agents. It enables sessions
to store, consolidate, and retrieve memories across conversations using
biologically-inspired retrieval (ecphory) and consolidation (hippocampus).

## Functional Requirements

### Memory Storage
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
