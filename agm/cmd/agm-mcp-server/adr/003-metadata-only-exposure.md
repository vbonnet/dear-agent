# ADR 003: Metadata-Only Exposure

## Status

Accepted

## Context

The AGM MCP server needs to expose AGM session information to external clients (Claude Code). AGM sessions contain:
1. **Metadata**: Session name, ID, timestamps, status (in `manifest.json`)
2. **Conversation Content**: User prompts, agent responses, tool calls (in `history.jsonl`)

Exposing conversation content would raise significant privacy and security concerns:
- Conversation history may contain sensitive information (API keys, credentials, personal data)
- Users may not want AI assistants reading past conversations
- Large conversation histories would slow down queries
- No clear use case for exposing full conversations via MCP

### Requirements

1. Enable Claude Code to discover and filter sessions
2. Protect user privacy (no conversation content exposure)
3. Minimize attack surface (limit data accessible via MCP)
4. Maintain performance (avoid reading large history files)

### Options Considered

#### Option 1: Expose Full Sessions (Metadata + Conversation)

**What's Exposed**:
- Session metadata (name, ID, timestamps)
- Full conversation history (turns, prompts, responses)
- Tool calls and results
- File contents from conversation

**Pros**:
- Maximum flexibility for AI assistants
- Could enable "search within conversations" feature
- AI could analyze conversation patterns

**Cons**:
- **Major Privacy Risk**: Conversations may contain sensitive data
- **Performance Hit**: Reading 1000s of history files is slow
- **Security Risk**: API keys, credentials exposed to MCP client
- **No Clear Use Case**: What would Claude Code do with past conversations?
- **Trust Model**: Users may not trust AI reading their conversations

#### Option 2: Expose Metadata + Sanitized Summaries

**What's Exposed**:
- Session metadata (name, ID, timestamps)
- Auto-generated summaries (stripped of sensitive data)
- Conversation statistics (turn count, token usage)

**Pros**:
- Richer search (search by conversation topic)
- Privacy-preserving (summaries sanitized)
- Moderate performance (summarize once, cache)

**Cons**:
- **Complex Implementation**: Automatic sanitization is hard
- **False Sense of Security**: Sanitization may miss sensitive data
- **Performance**: Requires reading/summarizing all conversations
- **Storage Overhead**: Summaries need to be stored/cached
- **No Clear ROI**: Effort doesn't match value for V1

#### Option 3: Expose Metadata Only

**What's Exposed**:
- Session ID (UUID)
- Session name
- Creation timestamp
- Last updated timestamp
- Status (active/archived)
- Agent type (claude/gemini/gpt)
- Tmux session name

**What's NOT Exposed**:
- Conversation turns
- User prompts
- Agent responses
- Tool calls
- API keys
- Credentials
- File contents

**Pros**:
- **Maximum Privacy**: Zero conversation content exposed
- **Simple Implementation**: Just read manifest.json
- **Best Performance**: Manifest files are small (<1KB)
- **Clear Security Boundary**: No risk of credential leakage
- **Sufficient for V1**: Covers all current use cases

**Cons**:
- Cannot search within conversation content
- Cannot analyze conversation patterns
- Limited context for AI assistant

#### Option 4: Expose Metadata + Opt-In Content

**What's Exposed**:
- Always: Session metadata
- Opt-in: Conversation content (user enables per session)

**Pros**:
- User control (opt-in privacy)
- Flexibility (enable for non-sensitive sessions)

**Cons**:
- **Complex UX**: Users must understand opt-in implications
- **Implementation Complexity**: Requires opt-in tracking
- **Default Unsafe?**: If default is "exposed", privacy suffers
- **V1 Scope Creep**: Adds significant complexity

## Decision

We will **expose metadata only** (Option 3).

## Rationale

### Privacy First

AGM sessions may contain:
- API keys for OpenAI, Anthropic, Google
- Credentials for databases, cloud services
- Sensitive business logic
- Personal information
- Proprietary code

**Exposing this via MCP is unacceptable risk for V1.**

### Clear Security Boundary

Metadata-only exposure creates a clear trust boundary:
- **Manifest files**: Safe to read (no sensitive data)
- **History files**: Never touched by MCP server

This makes security auditing trivial:
```bash
# Verify MCP server never reads history.jsonl
grep -r "history.jsonl" cmd/agm-mcp-server/
# No results = security boundary maintained
```

### Performance

Metadata-only access is 100x faster:
- Manifest file: ~500 bytes (parse in <1ms)
- History file: ~100KB - 10MB (parse in 50-500ms)

For 1000 sessions:
- Metadata only: ~1 second total
- Metadata + content: ~100+ seconds total

### Sufficiency for Current Use Cases

All identified V1 use cases work with metadata only:

1. **List sessions**: ✅ (name, ID, status)
2. **Search by name**: ✅ (session name)
3. **Filter by status**: ✅ (active/archived)
4. **Get session details**: ✅ (metadata fields)

No current use case requires conversation content.

### Extensibility

If future use cases require content access, we can add it in V2 with proper safeguards:
- Opt-in per session
- Sanitization pipeline
- Audit logging
- User consent flow

Starting with metadata-only doesn't prevent future extensions.

## Implementation

### Data Model

```go
type MCPSessionMetadata struct {
    ID             string  `json:"id"`              // Session UUID
    SessionName    string  `json:"session_name"`    // User-assigned name
    CreatedAt      string  `json:"created_at"`      // RFC3339 timestamp
    UpdatedAt      string  `json:"updated_at"`      // RFC3339 timestamp
    Status         string  `json:"status"`          // active|archived
    AgentType      string  `json:"agent_type"`      // claude|gemini|gpt
    TmuxSession    string  `json:"tmux_session"`    // Tmux session name
    RelevanceScore float64 `json:"relevance_score"` // Optional (search only)
}
```

### Source of Truth

All data comes from `manifest.json`:
```json
{
  "session_id": "uuid",
  "name": "session name",
  "created_at": "2025-01-15T10:30:00Z",
  "updated_at": "2025-01-15T14:20:00Z",
  "lifecycle": "active",
  "tmux": {
    "session_name": "agm-session-1"
  }
}
```

### Code Enforcement

Never import `internal/claude/history` or read `history.jsonl`:
```go
// GOOD: Only imports manifest
import "github.com/vbonnet/ai-tools/agm/internal/manifest"

// BAD: Don't import history
// import "github.com/vbonnet/ai-tools/agm/internal/claude/history"
```

## Consequences

### Positive

- **Privacy Protected**: Zero risk of exposing sensitive conversation data
- **Simple Implementation**: Only parse manifest files
- **Fast Performance**: Manifest files are <1KB each
- **Clear Security Audit**: Easy to verify no history access
- **User Trust**: Users can trust MCP server with their sessions

### Negative

- **Limited Search**: Cannot search within conversation content
- **No Conversation Analysis**: Cannot analyze conversation patterns
- **Less Context**: AI assistant has less context about sessions

### Neutral

- **Extensible**: Can add content access in V2 if needed
- **Consistent**: Matches privacy principles of other AGM features

## Privacy Documentation

### README Section

Add to README.md:
```markdown
## Privacy & Security

**Exposed Metadata** (safe):
- Session ID, name, created/updated timestamps
- Status (active/archived)
- Agent type, tmux session name

**NOT Exposed** (privacy protected):
- Conversation turns, user prompts, agent responses
- API keys, credentials
- Full conversation history
```

### User-Facing Documentation

Document in user guide:
> The AGM MCP server exposes only session metadata (name, ID, timestamps, status).
> Your conversation history is never accessed or shared with MCP clients. This ensures
> your API keys, credentials, and sensitive data remain private.

## Future Enhancements

### V2: Opt-In Content Access

If conversation search becomes a requirement:

1. **Add Opt-In Flag**:
   ```yaml
   # manifest.json
   {
     "mcp_exposure": "metadata_only" | "full_content"
   }
   ```

2. **Sanitization Pipeline**:
   - Regex-based credential detection
   - API key pattern matching
   - Personal data redaction (emails, phone numbers)

3. **Audit Logging**:
   - Log all content access attempts
   - User can review access history

4. **User Consent**:
   - Explicit consent flow in AGM CLI
   - Default to "metadata_only"

### V3: Conversation Summaries

Instead of full content, expose AI-generated summaries:
- Summarize conversation on archive
- Store summary in manifest.json
- Expose summary via MCP (not full content)

Benefits:
- Richer search without exposing sensitive data
- Performance (read summary, not full history)
- Privacy (summary sanitized during generation)

## Security Audit Checklist

- [ ] No imports of `internal/claude/history`
- [ ] No file reads of `history.jsonl`
- [ ] All data sourced from `manifest.json` only
- [ ] No user-controlled file paths
- [ ] No conversation content in MCP responses
- [ ] README documents privacy guarantees

## References

- OWASP Top 10: https://owasp.org/www-project-top-ten/
- Privacy by Design: https://en.wikipedia.org/wiki/Privacy_by_design
- Principle of Least Privilege: https://en.wikipedia.org/wiki/Principle_of_least_privilege

## Decision Date

2025-01-15

## Reviewers

- author
