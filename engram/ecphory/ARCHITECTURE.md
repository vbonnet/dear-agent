# Ecphory - Architecture

## System Overview

The ecphory package implements a 3-tier memory retrieval pipeline for the Engram knowledge base. It combines fast frontmatter indexing, LLM-based semantic ranking, and token budget management to retrieve the most relevant engrams for a given query.

## Architectural Principles

1. **Three-Tier Pipeline**: Fast filter → Semantic rank → Budget load
2. **Provider Abstraction**: Multiple LLM backends (Anthropic, VertexAI)
3. **Failure-Aware Retrieval**: Debugging context detection and boosting
4. **Rate Limiting**: Token bucket algorithm prevents API abuse
5. **Thread-Safe**: Concurrent queries with RWMutex protection
6. **Graceful Degradation**: API failures fall back to unranked results

## Component Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                           Ecphory                               │
│         (Orchestrates 3-tier retrieval pipeline)                │
└──────┬──────────────────┬──────────────────┬────────────────────┘
       │                  │                  │
       ▼                  ▼                  ▼
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│    Index    │    │   Ranker    │    │ContextDetect│
│  (Tier 1)   │    │  (Tier 2)   │    │   (Boost)   │
│ Fast Filter │    │   Semantic  │    │   Failure   │
└──────┬──────┘    └──────┬──────┘    └──────┬──────┘
       │                  │                  │
       │                  ▼                  │
       │           ┌─────────────┐           │
       │           │  Provider   │           │
       │           │  Interface  │           │
       │           └──────┬──────┘           │
       │                  │                  │
       │          ┌───────┴───────┐          │
       │          ▼               ▼          │
       │   ┌────────────┐  ┌────────────┐   │
       │   │ Anthropic  │  │  VertexAI  │   │
       │   │  Provider  │  │  Provider  │   │
       │   └────────────┘  └────────────┘   │
       │                                     │
       └─────────────┬───────────────────────┘
                     │
                     ▼
              ┌─────────────┐
              │   Parser    │
              │  (engram)   │
              └─────────────┘
                     │
                     ▼
              ┌─────────────┐
              │  Engrams    │
              │  (.ai.md)   │
              └─────────────┘
```

## C4 Component Diagram

### Internal Architecture Visualization (Level 3)

A detailed C4 Component diagram showing the internal structure of the Ecphory 3-tier retrieval system:

**Diagrams**:
- [C4 Component Diagram (SVG)](diagrams/c4-component-ecphory.svg)
- [C4 Component Diagram (PNG)](diagrams/c4-component-ecphory.png)
- [D2 Source](diagrams/c4-component-ecphory.d2)

The diagram illustrates:

**Tier 1: Fast Filter**
- Index component (in-memory frontmatter index with tag/agent/type filtering)
- Symlink Handler (cycle detection, max depth 5)

**Tier 2: Semantic Ranking**
- Ranker (semantic relevance ranking via LLM)
- Rate Limiter (token bucket: 100/hour, 20/session, 1s minimum interval)
- Provider Factory (multi-provider management with precedence ordering)

**Tier 3: Budget Management**
- Budget Manager (token budget enforcement, load in relevance order)
- Metadata Updater (async frontmatter updates)

**LLM Providers**
- Anthropic Provider (claude-3-5-haiku)
- Vertex AI Claude (claude-sonnet-4-5, us-east5)
- Vertex AI Gemini (gemini-2.0-flash, us-central1)
- Local Provider (tag-based fallback, no API calls)

**Failure Boosting System**
- Context Detector (regex pattern matching, 5 error categories)
- Failure Booster (+25.0 relevance boost for matching category)

**External Dependencies**
- Engram Parser, Reflection Types, Prompt Sanitizer, LLM Auth
- Anthropic API, Vertex AI API, EventBus, Engram Files (.ai.md)

---

## Component Details

### 1. Ecphory (Orchestrator)

**Responsibility**: Coordinate 3-tier retrieval pipeline and manage lifecycle

**Key Responsibilities**:
- Initialize index, ranker, and context detector
- Execute 3-tier pipeline (filter, rank, budget)
- Apply failure boosting for debugging queries
- Publish telemetry events
- Update frontmatter metadata
- Clean up resources

**State Management**:
```go
type Ecphory struct {
    index           *Index            // Tier 1: Frontmatter index
    ranker          *Ranker           // Tier 2: Semantic ranker
    parser          *engram.Parser    // Parser for loading content
    tokenBudget     int               // Tier 3: Maximum tokens
    eventBus        EventBus          // Optional telemetry
    basePath        string            // Base path for privacy
    contextDetector *ContextDetector  // Failure context detection
}
```

**Lifecycle**:
1. **Construction**: Build index, create ranker, initialize detector
2. **Query**: Filter → Rank → Boost → Budget → Load
3. **Cleanup**: Close ranker, clear index

**Design Decisions**:
- Ranker initialization failure triggers index cleanup (resource leak prevention)
- Context cancellation checked before expensive operations (API calls, loading)
- Telemetry and metadata updates are asynchronous (non-blocking)
- API failures fall back to unranked candidates (graceful degradation)

### 2. Index (Tier 1: Fast Filter)

**Responsibility**: In-memory frontmatter index for fast filtering

**Architecture**:
```go
type Index struct {
    mu sync.RWMutex  // Protects all maps and slices

    byTag           map[string][]string  // Tag -> paths
    byType          map[string][]string  // Type -> paths
    byAgent         map[string][]string  // Agent -> paths
    agentAgnostic   []string             // Pre-cached agent-agnostic
    all             []string             // All paths

    parser          *engram.Parser
    visitedSymlinks map[string]bool      // Cycle detection
    symlinkDepth    int                  // Recursion limit
}
```

**Indexing Pipeline**:
```
filepath.Walk → Filter .ai.md → Parse frontmatter → Index by tag/type/agent
```

**Filtering Strategy**:
- **Tag filtering**: Hierarchical prefix matching (e.g., "languages/python" matches "languages")
- **Agent filtering**: Pre-cached agent-agnostic engrams for O(1) lookup
- **Type filtering**: Direct map lookup
- **Intersection**: Fast set intersection for multi-criteria filters

**Symlink Handling**:
- Detect cycles using visitedSymlinks map
- Enforce maximum depth limit (5)
- Follow symlinks to get actual file info
- Skip broken symlinks gracefully

**Concurrency Model**:
- RWMutex allows concurrent reads, exclusive writes
- Build() takes write lock, FilterBy*() take read locks
- Thread-safe for concurrent queries

**Error Handling**:
- Parse errors logged and skipped (continue indexing)
- Symlink cycles logged and skipped
- Engram count limit (100,000) prevents DoS

### 3. Ranker (Tier 2: Semantic Ranking)

**Responsibility**: Rank candidates by semantic relevance using LLM providers

**Architecture**:
```go
type Ranker struct {
    provider    Provider     // LLM provider (Anthropic or VertexAI)
    rateLimiter *RateLimiter // Token bucket rate limiter
}

type Provider interface {
    Complete(ctx context.Context, prompt string) (string, error)
    Close() error
}
```

**Ranking Pipeline**:
```
Build prompt → Check rate limit → Call provider → Parse JSON → Validate results
```

**Provider Auto-Detection**:
```
1. Check GOOGLE_CLOUD_PROJECT → VertexAI
2. Fallback ANTHROPIC_API_KEY → Anthropic
3. None available → Error
```

**Prompt Structure** (Injection Defense):
```xml
<user>
User query: "error handling patterns"
</user>

<untrusted_data>
Engram candidates:
1. patterns/error-handling.ai.md
2. strategies/error-recovery.ai.md
...

Task: Rank these engrams by relevance to the user's query.
Return JSON array: [{"path": "...", "relevance": 0.9, "reasoning": "..."}]
</untrusted_data>
```

**Validation Strategy**:
- Non-empty results
- Valid paths (must be in candidate list)
- Relevance in range [0.0, 1.0]
- Required fields present (path, relevance)

**Design Decisions**:
- Provider interface allows extensibility (OpenAI, Gemini, local models)
- Query sanitization prevents prompt injection
- XML hierarchy isolates user input from system instructions
- JSON parsing with validation prevents malformed responses

### 4. RateLimiter (API Protection)

**Responsibility**: Token bucket rate limiting for API calls

**Architecture**:
```go
type RateLimiter struct {
    mu sync.Mutex  // Protects state

    tokensPerHour    int           // 100 requests/hour
    tokensPerSession int           // 20 requests/session
    minInterval      time.Duration // 1 second cooldown

    hourlyTokens  int
    sessionTokens int
    lastRequest   time.Time
    hourStart     time.Time
}
```

**Algorithm**:
```
1. Reset hourly tokens if hour elapsed (monotonic clock)
2. Check hourly limit (100/hour)
3. Check session limit (20/session)
4. Check minimum interval (1 second)
5. Consume tokens if allowed
```

**Monotonic Clock**:
- Uses time.Now().Sub() for duration calculation
- Immune to clock adjustments (NTP, DST)
- Prevents time drift issues

**Error Messages**:
- "hourly rate limit exceeded (100/hour)"
- "session rate limit exceeded (20/session)"
- "rate limit: wait 500ms before next request"

**Design Decisions**:
- Token bucket instead of sliding window (simpler)
- Per-session limits prevent runaway queries
- Minimum interval prevents burst traffic
- Thread-safe with mutex

### 5. ContextDetector (Failure Boosting)

**Responsibility**: Detect debugging context and classify error category

**Architecture**:
```go
type ContextDetector struct {
    debuggingKeywords  *regexp.Regexp  // error, failed, bug, crash, etc.
    syntaxPatterns     *regexp.Regexp  // syntax, parse, compilation
    toolMisusePatterns *regexp.Regexp  // wrong tool, invalid call
    permissionPatterns *regexp.Regexp  // permission denied, 403, 401
    timeoutPatterns    *regexp.Regexp  // timeout, hung, deadlock
}
```

**Detection Pipeline**:
```
1. Check syntax patterns → syntax_error
2. Check permission patterns → permission_denied
3. Check timeout patterns → timeout
4. Check tool misuse patterns → tool_misuse
5. Check general debugging keywords → other
6. No match → (false, "")
```

**Error Categories** (from reflection package):
- `syntax_error`: Parse errors, missing brackets, indentation
- `permission_denied`: Access denied, 401, 403, credentials
- `timeout`: Timeout, hung, deadlock, unresponsive
- `tool_misuse`: Wrong tool, invalid call, API misuse
- `other`: General errors, failures, bugs

**Boosting Strategy**:
- Extract error_category from reflection frontmatter
- Apply +25.0 boost for matching category
- Cap at 100.0 to maintain score range
- Leave non-reflections unchanged

**Design Decisions**:
- Regex patterns pre-compiled at construction
- Priority order ensures specific categories match first
- Word boundary detection prevents false positives
- Case-insensitive matching for robustness

### 6. Provider Implementations

#### AnthropicProvider

**Model**: claude-3-5-haiku-20241022 (fast, cost-effective)

**API**:
```go
response, err := client.Messages.New(ctx, anthropic.MessageNewParams{
    Model:     "claude-3-5-haiku-20241022",
    MaxTokens: 4096,
    Messages:  []MessageParam{UserMessage(prompt)},
})
```

**Validation**:
- API key must start with "sk-ant-"
- Response must contain TextBlock
- Non-empty content

#### VertexAIProvider

**Model**: claude-3-5-sonnet-v2@20241022 (via Google VertexAI)

**API**:
```http
POST https://{location}-aiplatform.googleapis.com/v1/
     projects/{project}/locations/{location}/
     publishers/anthropic/models/{model}:streamRawPredict
```

**Authentication**:
- Uses Application Default Credentials
- Calls `gcloud auth application-default print-access-token`
- Requires GOOGLE_APPLICATION_CREDENTIALS or gcloud login

**Response Parsing**:
- Streaming format (multiple JSON objects)
- Extract text from content_block_delta events
- Concatenate all text chunks

**Design Decisions**:
- VertexAI preferred over Anthropic (enterprise use case)
- gcloud CLI for token management (simplicity)
- Streaming response parsing for efficiency

## Data Flow

### Retrieval Pipeline

```
User Query
    │
    ▼
Ecphory.Query(ctx, query, sessionID, transcript, tags, agent)
    │
    ├─→ Tier 1: Index.FilterByTags(tags) + FilterByAgent(agent)
    │   └─→ Returns: []string (candidate paths)
    │
    ├─→ Tier 2: Ranker.Rank(ctx, query, candidates)
    │   ├─→ RateLimiter.Allow()
    │   ├─→ buildRankingPrompt(query, candidates)
    │   ├─→ Provider.Complete(ctx, prompt)
    │   └─→ Returns: []RankingResult (path, relevance, reasoning)
    │
    ├─→ Failure Boosting: applyFailureBoosting(query, ranked)
    │   ├─→ ContextDetector.DetectContext(query)
    │   ├─→ Extract error_category from frontmatter
    │   └─→ Boost matching reflections (+25.0)
    │
    ├─→ Sort by relevance (descending)
    │
    └─→ Tier 3: loadWithinBudget(ranked)
        ├─→ Parse engrams in relevance order
        ├─→ Estimate tokens (char/4 heuristic)
        ├─→ Stop when budget exhausted
        └─→ Returns: []*engram.Engram
```

### Telemetry Flow

```
Query completed
    │
    └─→ publishEcphoryEvent(ctx, query, sessionID, transcript, ...)
        ├─→ Build event with results, tokens, duration
        ├─→ Convert paths to relative (privacy)
        └─→ EventBus.Publish(event)  [async]
```

### Metadata Update Flow

```
Engrams loaded
    │
    └─→ updateFrontmatterMetadata(engrams)  [async]
        └─→ For each engram:
            ├─→ Re-parse frontmatter
            ├─→ Increment retrieval_count
            ├─→ Update last_accessed timestamp
            └─→ writeFrontmatter(path, frontmatter, content)
                └─→ [TODO: Atomic writes in Phase 2]
```

## Concurrency Model

### Goroutines

1. **Main query goroutine**: Executes 3-tier pipeline synchronously
2. **Telemetry goroutine**: Publishes events asynchronously (non-blocking)
3. **Metadata goroutine**: Updates frontmatter asynchronously (non-blocking)

### Synchronization

- Index: RWMutex for concurrent reads, exclusive writes
- RateLimiter: Mutex for state updates
- Ecphory: No shared mutable state (safe for concurrent queries)

### Thread Safety

- **Ecphory**: Thread-safe (index and ranker are thread-safe)
- **Index**: Thread-safe (RWMutex)
- **Ranker**: Thread-safe (RateLimiter has mutex)
- **ContextDetector**: Thread-safe (read-only regex patterns)
- **Provider**: Thread-safe (HTTP client, Anthropic SDK)

## Configuration Management

### Environment Variables

```bash
# Anthropic API (primary)
ANTHROPIC_API_KEY=sk-ant-...

# VertexAI (alternative)
GOOGLE_CLOUD_PROJECT=my-project
VERTEX_LOCATION=us-central1           # Optional, default: us-central1
VERTEX_MODEL=claude-3-5-sonnet-v2@... # Optional, default: claude-3-5-sonnet-v2@20241022
```

### Constructor Parameters

```go
ecphory, err := NewEcphory(
    "/path/to/engrams",  // Engram directory
    10000,               // Token budget (char/4 ≈ 2500 actual tokens)
)
```

### Functional Options

```go
ecphory.ApplyOptions(
    WithEventBus(bus),  // Enable telemetry
)
```

## Error Handling Strategy

### Error Categories

1. **Initialization Errors**: Fatal (return error from NewEcphory)
2. **API Errors**: Non-fatal (fall back to unranked candidates)
3. **Parse Errors**: Non-fatal (log and skip individual engrams)
4. **Validation Errors**: Fatal (invalid ranking results)

### Error Recovery

```
Index build failure → Fatal (cannot proceed)
Ranker init failure → Clean up index, fatal
API call failure → Fall back to unranked candidates
Rate limit exceeded → Error with wait time
Parse error → Log and skip engram, continue
Context cancelled → Return error immediately
```

### Design Philosophy

**Graceful Degradation**: API failures should not block retrieval. Return unranked candidates rather than no results.

## Performance Characteristics

### Time Complexity

- Index build: O(n) where n = number of engrams
- Tag filtering: O(t × m) where t = tags, m = engrams per tag
- Agent filtering: O(1) (pre-cached agent-agnostic)
- API ranking: O(1) per query (single API call for all candidates)
- Token budget loading: O(k) where k = candidates ranked

### Space Complexity

- Index: O(n) where n = number of engrams
- Ranker: O(1) (stateless except rate limiter)
- Rate limiter: O(1) (fixed state size)

### Bottlenecks

- Index building (large engram directories)
- API ranking (network latency, LLM inference)
- Token budget loading (file I/O for parsing)

### Optimizations

- Pre-cached agent-agnostic engrams (O(1) lookup)
- Single API call for all candidates (batch ranking)
- Context cancellation checks before expensive operations
- RWMutex for concurrent reads during filtering

## Security Considerations

### Attack Vectors

1. **Prompt Injection**: User query could contain instructions to ignore system prompt
   - Mitigation: Query sanitization, XML hierarchy

2. **API Key Leakage**: API key could be logged or exposed
   - Mitigation: Never log API keys, validate format only

3. **Rate Limit Bypass**: Malicious user could exhaust API quota
   - Mitigation: Token bucket rate limiting (100/hour, 20/session)

4. **DoS via Large Index**: Millions of engrams could exhaust memory
   - Mitigation: Maximum engram limit (100,000)

5. **Symlink Attacks**: Symlink cycles or depth bombs
   - Mitigation: Cycle detection, depth limit (5)

### Security Best Practices

- API keys validated but never logged
- User queries sanitized before API calls
- Rate limits enforced per session
- Telemetry uses relative paths (privacy)
- Context cancellation prevents runaway queries

## Testing Strategy

### Unit Tests

- Index building and filtering (all criteria)
- Rate limiter token bucket algorithm
- Context detector pattern matching
- Failure boosting score adjustment
- Symlink cycle detection
- Provider API validation
- Prompt injection sanitization

### Integration Tests

- End-to-end retrieval with real engrams
- API call with real providers
- Rate limit enforcement across queries
- Context cancellation during API calls
- Telemetry event publishing

### Validation Tests

- Failure boosting effectiveness (≥80%)
- Top results contain matching failures
- Ranking order correctness
- No false positives for normal queries
- All 5 error categories validated

### Test Coverage

- **Total tests**: 70 (64 existing + 6 validation)
- **Context detection**: 100% (50 test cases)
- **Failure boosting**: 100% (11 test cases)
- **Validation**: 100% (15 scenarios)

## Deployment Considerations

### System Requirements

- Go 1.19+ (monotonic clock support)
- LLM provider (Anthropic API or Google VertexAI)
- gcloud CLI (for VertexAI authentication)
- Engram directory with .ai.md files

### Resource Requirements

- Memory: ~10MB per 10,000 engrams
- Disk: None (read-only access to engrams)
- CPU: Negligible (index is in-memory)
- Network: API calls (1-2 per query)

### Scaling Limits

- Maximum engrams: 100,000
- Maximum API calls: 100/hour, 20/session
- Maximum symlink depth: 5
- Maximum token budget: Unlimited (user-specified)

## Future Architecture Enhancements

### Planned Improvements

1. **Exact Tokenizer**: Replace char/4 heuristic with tiktoken
2. **Ranking Cache**: Cache ranking results to reduce API calls
3. **Streaming Results**: Stream engrams as they load
4. **Vector Search**: Embedding-based similarity search
5. **Learned Ranking**: Fine-tuned ranking models
6. **Atomic Metadata**: Atomic frontmatter updates for retrieval_count
7. **Additional Providers**: OpenAI, Gemini, local models

### Extensibility Points

- Provider interface for new LLM backends
- EventBus interface for custom telemetry
- ContextDetector patterns for new error categories
- Index filtering strategies

## Dependencies

### Direct Dependencies

- `github.com/vbonnet/engram/core/pkg/engram` - Parser and frontmatter
- `github.com/vbonnet/engram/core/internal/reflection` - Error categories
- `github.com/vbonnet/engram/core/internal/prompt` - Injection defense
- `github.com/anthropics/anthropic-sdk-go` - Anthropic API client
- `gopkg.in/yaml.v3` - YAML frontmatter parsing

### System Dependencies

- gcloud CLI (for VertexAI authentication)
- Anthropic API or Google VertexAI access

## Architectural Decision Records

See ADR.md for detailed architectural decisions and rationale.
