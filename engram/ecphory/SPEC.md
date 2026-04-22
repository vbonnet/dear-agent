# Ecphory - Specification

## Overview

The ecphory package implements a 3-tier memory retrieval system for the Engram knowledge base. Ecphory (from Greek: ἐκφορά "retrieval") is the process of reconstructing a memory from a cue. This package provides fast, semantically-aware retrieval of relevant engrams while respecting token budget constraints.

## Purpose

**Primary Goal**: Retrieve the most relevant engrams from a knowledge base for a given query, ranked by semantic relevance, within a specified token budget.

**Key Capabilities**:
- Fast frontmatter-based filtering by tags and agent
- Semantic relevance ranking using LLM providers (Anthropic, VertexAI)
- Token-aware budget management
- Failure context detection and boosting for debugging queries
- Rate limiting to prevent API abuse
- Telemetry integration for usage tracking

## Functional Requirements

### FR-1: Ecphory Lifecycle

The system SHALL provide complete lifecycle management for memory retrieval:

- **FR-1.1**: Initialize ecphory with engram directory path and token budget
- **FR-1.2**: Build frontmatter index from .ai.md files
- **FR-1.3**: Execute 3-tier retrieval pipeline (filter, rank, budget)
- **FR-1.4**: Close and cleanup resources (API clients, index)

### FR-2: Tier 1 - Fast Filtering

The system SHALL perform fast frontmatter-based filtering:

- **FR-2.1**: Build in-memory index of all engram frontmatter metadata
- **FR-2.2**: Filter by tags with hierarchical matching (e.g., "languages/python" matches "languages")
- **FR-2.3**: Filter by agent (specific agent or agent-agnostic)
- **FR-2.4**: Filter by type (reflection, pattern, strategy, etc.)
- **FR-2.5**: Return all matching engram paths for ranking

### FR-3: Tier 2 - Semantic Ranking

The system SHALL rank filtered candidates by semantic relevance:

- **FR-3.1**: Support multiple LLM providers (Anthropic API, Google VertexAI)
- **FR-3.2**: Auto-detect provider based on environment variables
- **FR-3.3**: Build ranking prompt with query and candidate paths
- **FR-3.4**: Parse JSON response with relevance scores (0.0-1.0)
- **FR-3.5**: Validate ranking results (paths, scores, structure)
- **FR-3.6**: Apply failure boosting for debugging queries
- **FR-3.7**: Sort results by relevance (descending)

### FR-4: Tier 3 - Token Budget Management

The system SHALL load engrams within token budget constraints:

- **FR-4.1**: Estimate tokens using character count heuristic (char/4)
- **FR-4.2**: Load engrams in relevance order (highest first)
- **FR-4.3**: Stop loading when token budget exhausted
- **FR-4.4**: Return loaded engrams sorted by relevance

### FR-5: Failure Context Detection

The system SHALL detect debugging context and boost relevant failures:

- **FR-5.1**: Detect debugging keywords (error, failed, broken, bug, crash, etc.)
- **FR-5.2**: Classify error category (syntax, permission, timeout, tool_misuse, other)
- **FR-5.3**: Boost relevance scores (+25.0) for matching error categories
- **FR-5.4**: Cap boosted scores at 100.0
- **FR-5.5**: Leave non-debugging queries unaffected

### FR-6: Frontmatter Index

The system SHALL build and maintain a frontmatter index:

- **FR-6.1**: Walk directory tree to find .ai.md files
- **FR-6.2**: Parse frontmatter YAML for each engram
- **FR-6.3**: Index by tags, type, and agent
- **FR-6.4**: Detect and handle symlink cycles
- **FR-6.5**: Enforce maximum engram limit (100,000) for DoS protection
- **FR-6.6**: Thread-safe concurrent access with RWMutex

### FR-7: Rate Limiting

The system SHALL enforce rate limits for API calls:

- **FR-7.1**: Token bucket algorithm with hourly and session limits
- **FR-7.2**: Hourly limit: 100 requests/hour
- **FR-7.3**: Session limit: 20 requests/session
- **FR-7.4**: Minimum interval: 1 second between requests
- **FR-7.5**: Use monotonic clock to prevent time drift issues
- **FR-7.6**: Return error with wait time when limit exceeded

### FR-8: Provider Support

The system SHALL support multiple LLM providers:

- **FR-8.1**: Anthropic API provider
  - Model: claude-3-5-haiku-20241022
  - API key validation (must start with "sk-ant-")
  - Max tokens: 4096
  - Environment variable: ANTHROPIC_API_KEY

- **FR-8.2**: Google VertexAI provider
  - Model: claude-3-5-sonnet-v2@20241022
  - Access token via gcloud CLI
  - Streaming response parsing
  - Environment variables: GOOGLE_CLOUD_PROJECT, VERTEX_LOCATION, VERTEX_MODEL

### FR-9: Prompt Injection Defense

The system SHALL defend against prompt injection attacks:

- **FR-9.1**: Sanitize user queries (reject XML tags, injection patterns)
- **FR-9.2**: Wrap user query in `<user>` tags
- **FR-9.3**: Wrap candidate paths in `<untrusted_data>` tags
- **FR-9.4**: Use XML hierarchy to isolate external data
- **FR-9.5**: Validate sanitized query before API call

### FR-10: Telemetry Integration

The system SHALL publish telemetry events:

- **FR-10.1**: Publish ecphory.query events to EventBus
- **FR-10.2**: Include query, session ID, transcript, tags, agent
- **FR-10.3**: Include result count, paths, tokens used, duration
- **FR-10.4**: Use relative paths for privacy
- **FR-10.5**: Asynchronous publishing (non-blocking)
- **FR-10.6**: Optional (nil EventBus disables telemetry)

### FR-11: Frontmatter Metadata Updates

The system SHALL update engram metadata on retrieval:

- **FR-11.1**: Increment retrieval_count field
- **FR-11.2**: Update last_accessed timestamp
- **FR-11.3**: Asynchronous updates (non-blocking)
- **FR-11.4**: Log errors without failing queries

## Non-Functional Requirements

### NFR-1: Performance

- **NFR-1.1**: Tier 1 filtering SHALL complete in < 100ms for 10,000 engrams
- **NFR-1.2**: Index building SHALL use O(n) time where n = number of engrams
- **NFR-1.3**: Tag filtering SHALL use hierarchical prefix matching
- **NFR-1.4**: Agent filtering SHALL use pre-cached agent-agnostic engrams (O(1))

### NFR-2: Reliability

- **NFR-2.1**: API failures SHALL fall back to unranked candidates
- **NFR-2.2**: Parse errors SHALL be logged and skipped
- **NFR-2.3**: Context cancellation SHALL be checked before expensive operations
- **NFR-2.4**: Symlink cycles SHALL be detected and skipped
- **NFR-2.5**: Ranker initialization failures SHALL clean up index resources

### NFR-3: Security

- **NFR-3.1**: API keys SHALL be validated before use
- **NFR-3.2**: API keys SHALL NOT be logged
- **NFR-3.3**: User queries SHALL be sanitized for prompt injection
- **NFR-3.4**: Rate limits SHALL prevent API abuse
- **NFR-3.5**: Telemetry SHALL use relative paths (privacy)

### NFR-4: Scalability

- **NFR-4.1**: Support up to 100,000 engrams per index
- **NFR-4.2**: Thread-safe concurrent queries (RWMutex)
- **NFR-4.3**: Rate limiter SHALL be thread-safe
- **NFR-4.4**: Symlink depth limit (5) SHALL prevent infinite recursion

### NFR-5: Maintainability

- **NFR-5.1**: Provider interface SHALL be extensible
- **NFR-5.2**: Token budget SHALL be configurable at construction
- **NFR-5.3**: Rate limits SHALL be configurable via RateLimiter struct
- **NFR-5.4**: Failure boosting SHALL be toggleable (context detector)

## API Specification

### Ecphory API

```go
// Constructor
func NewEcphory(engramPath string, tokenBudget int) (*Ecphory, error)

// Configuration
func WithEventBus(bus EventBus) func(*Ecphory)
func (e *Ecphory) ApplyOptions(opts ...func(*Ecphory))

// Query
func (e *Ecphory) Query(
    ctx context.Context,
    query string,
    sessionID string,    // Session identifier for telemetry (can be empty)
    transcript string,   // Conversation context (can be empty)
    tags []string,
    agent string,
) ([]*engram.Engram, error)

// Cleanup
func (e *Ecphory) Close() error
```

### Index API

```go
// Constructor
func NewIndex() *Index

// Build
func (idx *Index) Build(engramPath string) error

// Filtering
func (idx *Index) FilterByTags(tags []string) []string
func (idx *Index) FilterByType(typ string) []string
func (idx *Index) FilterByAgent(agent string) []string
func (idx *Index) All() []string

// Cleanup
func (idx *Index) Clear()
```

### Ranker API

```go
// Constructor
func NewRanker() (*Ranker, error)

// Ranking
func (r *Ranker) Rank(
    ctx context.Context,
    query string,
    candidates []string,
) ([]RankingResult, error)

// Cleanup
func (r *Ranker) Close() error
```

### ContextDetector API

```go
// Constructor
func NewContextDetector() *ContextDetector

// Detection
func (d *ContextDetector) DetectContext(query string) (bool, reflection.ErrorCategory)
func (d *ContextDetector) IsDebuggingContext(query string) bool
```

### Data Types

```go
type Ecphory struct {
    index           *Index
    ranker          *Ranker
    parser          *engram.Parser
    tokenBudget     int
    eventBus        EventBus
    basePath        string
    contextDetector *ContextDetector
}

type Index struct {
    byTag           map[string][]string
    byType          map[string][]string
    byAgent         map[string][]string
    agentAgnostic   []string
    all             []string
    parser          *engram.Parser
    visitedSymlinks map[string]bool
    symlinkDepth    int
}

type Ranker struct {
    provider    Provider
    rateLimiter *RateLimiter
}

type RankingResult struct {
    Path      string
    Relevance float64
    Reasoning string
}

type Provider interface {
    Complete(ctx context.Context, prompt string) (string, error)
    Close() error
}

type RateLimiter struct {
    tokensPerHour    int
    tokensPerSession int
    minInterval      time.Duration
    hourlyTokens     int
    sessionTokens    int
    lastRequest      time.Time
    hourStart        time.Time
}
```

## Usage Patterns

### Pattern 1: Basic Retrieval

```go
ecphory, err := NewEcphory("/path/to/engrams", 10000)
if err != nil {
    log.Fatal(err)
}
defer ecphory.Close()

results, err := ecphory.Query(
    context.Background(),
    "error handling patterns",
    "session-123",                      // sessionID for telemetry (empty string accepted)
    "User asked about error handling",  // transcript for context (empty string accepted)
    []string{"go"},
    "claude-code",
)
if err != nil {
    log.Fatal(err)
}

for _, eg := range results {
    fmt.Printf("Engram: %s\n", eg.Path)
    fmt.Printf("Content: %s\n", eg.Content)
}
```

### Pattern 2: With Telemetry

```go
bus := eventbus.New()
ecphory, _ := NewEcphory("/path/to/engrams", 10000)
ecphory.ApplyOptions(WithEventBus(bus))
defer ecphory.Close()

results, _ := ecphory.Query(ctx, query, sessionID, transcript, tags, agent)
// Telemetry event published asynchronously
```

### Pattern 3: Debugging Context

```go
// User asks: "why did my API call fail with permission denied?"
results, _ := ecphory.Query(
    ctx,
    "permission denied API call error",
    sessionID,
    transcript,
    []string{"api"},
    "claude-code",
)
// Past permission_denied failures are boosted to top 5
```

### Pattern 4: Context Cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

results, err := ecphory.Query(ctx, query, sessionID, transcript, tags, agent)
if err != nil {
    if ctx.Err() != nil {
        log.Println("Query cancelled or timed out")
    }
}
```

## Constraints and Assumptions

### Constraints

- **C-1**: Requires LLM provider (Anthropic API or Google VertexAI)
- **C-2**: Token estimation uses char/4 heuristic (not exact tokenizer)
- **C-3**: Maximum 100,000 engrams per index
- **C-4**: Frontmatter must be valid YAML
- **C-5**: Engram files must have .ai.md extension

### Assumptions

- **A-1**: Engrams are stored in .ai.md files with YAML frontmatter
- **A-2**: Frontmatter includes tags, type, agents, retrieval_count, last_accessed
- **A-3**: LLM provider returns JSON array with path, relevance, reasoning
- **A-4**: API rate limits are enforced per-session
- **A-5**: Token budget is specified in approximate tokens (char/4)

## Error Handling

### Error Categories

1. **Initialization Errors**: Invalid engram path, index build failure, ranker initialization failure
2. **Runtime Errors**: API failures, rate limit exceeded, context cancellation
3. **Validation Errors**: Invalid ranking results, parse errors, prompt injection detected

### Error Strategies

- **API fallback**: If ranking fails, return unranked candidates
- **Graceful degradation**: Parse errors skip individual engrams, continue indexing
- **Fail fast**: Invalid API key format fails at initialization
- **Resource cleanup**: Ranker initialization failure cleans up index

## Testing Requirements

### Unit Tests

- **T-1**: Index building with tags, types, agents
- **T-2**: Tag filtering with hierarchical matching
- **T-3**: Agent filtering with agent-agnostic caching
- **T-4**: Rate limiter token bucket algorithm
- **T-5**: Context detector pattern matching
- **T-6**: Failure boosting score adjustment
- **T-7**: Symlink cycle detection
- **T-8**: Provider API validation

### Integration Tests

- **T-9**: End-to-end retrieval with real engrams
- **T-10**: Debugging query with failure boosting
- **T-11**: Rate limit enforcement across multiple queries
- **T-12**: Context cancellation during API calls
- **T-13**: Telemetry event publishing

### Validation Tests

- **T-14**: Failure boosting top results (all 5 categories)
- **T-15**: Ranking order correctness
- **T-16**: Simulated debugging scenarios
- **T-17**: Normal query non-interference
- **T-18**: Boost effectiveness measurement (≥80%)

## Dependencies

- `github.com/vbonnet/engram/core/pkg/engram` - Engram parser and frontmatter
- `github.com/vbonnet/engram/core/internal/reflection` - Error categories
- `github.com/vbonnet/engram/core/internal/prompt` - Prompt injection defense
- `github.com/anthropics/anthropic-sdk-go` - Anthropic API client
- `gopkg.in/yaml.v3` - YAML frontmatter parsing

## Future Considerations

- **F-1**: Exact tokenizer (tiktoken) instead of char/4 heuristic
- **F-2**: Caching of ranking results to reduce API calls
- **F-3**: Streaming query results as they load
- **F-4**: Vector similarity search (embedding-based retrieval)
- **F-5**: Learned relevance scoring (fine-tuned ranking models)
- **F-6**: Atomic frontmatter updates for retrieval_count
- **F-7**: Additional providers (OpenAI, Gemini, local models)
