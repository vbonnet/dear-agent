# Ranking - Architecture

## System Overview

The ranking package provides a multi-provider abstraction layer for semantic relevance ranking in the Engram knowledge base. It implements a factory pattern to manage multiple LLM backends (Anthropic, Vertex AI Gemini, Vertex AI Claude) with automatic provider detection and graceful fallback to local Jaccard similarity ranking.

## Architectural Principles

1. **Provider Abstraction**: Unified interface for multiple LLM backends
2. **Zero Vendor Lock-in**: Can run without any specific API provider
3. **Automatic Provider Selection**: Environment-based auto-detection with precedence
4. **Graceful Degradation**: Always falls back to local provider
5. **Extensibility**: Easy to add new providers (< 200 LOC)
6. **Cost Optimization**: Choose cheapest provider based on deployment context

## Component Architecture

```
┌────────────────────────────────────────────────────────────┐
│                    Ecphory (Client)                        │
│                Tier 2: Semantic Ranking                    │
└──────────────────────┬─────────────────────────────────────┘
                       │
                       ▼
         ┌─────────────────────────┐
         │    Factory               │
         │  (Provider Management)   │
         │  - Register()            │
         │  - AutoDetect()          │
         │  - GetProvider()         │
         │  - ListProviders()       │
         │  - Detect()              │
         └──────────┬───────────────┘
                    │
        ┌───────────┼────────────────────┐
        │           │                    │
        ▼           ▼                    ▼
  ┌─────────┐ ┌──────────┐      ┌────────────┐
  │Provider │ │ Provider │      │  Provider  │
  │Interface│ │Interface │  ... │ Interface  │
  └────┬────┘ └─────┬────┘      └─────┬──────┘
       │            │                  │
       │            │                  │
  ┌────┴─────┬──────┴────┬─────────────┴────────┐
  │          │           │                      │
  ▼          ▼           ▼                      ▼
┌────────┐ ┌────────┐ ┌────────┐ ┌──────────────────┐
│Anthropic Vertex   │ │Vertex  │ │      Local       │
│Provider│ │Gemini  │ │Claude  │ │    Provider      │
│        │ │Provider│ │Provider│ │ (Jaccard Sim.)   │
└────────┘ └────────┘ └────────┘ └──────────────────┘
    │          │          │              │
    │          │          │              │
    ▼          ▼          ▼              ▼
┌────────┐ ┌────────┐ ┌────────┐ ┌──────────────────┐
│Anthropic Google  │ │ Google │ │ Tag/Keyword      │
│  API   │ │Vertex │ │Vertex  │ │  Matching        │
│        │ │AI API │ │AI API  │ │  (No API)        │
└────────┘ └────────┘ └────────┘ └──────────────────┘
```

## Component Details

### 1. Provider Interface

**Responsibility**: Define common ranking contract for all implementations

**Interface Definition**:
```go
type Provider interface {
    Name() string                    // Provider identifier
    Model() string                   // Model identifier
    Rank(ctx context.Context, query string, candidates []Candidate) ([]RankedResult, error)
    Capabilities() Capabilities      // Provider feature set
}
```

**Design Rationale**:
- **Simplicity**: Minimal interface (4 methods) easy to implement
- **Extensibility**: New providers only need to implement interface
- **Testability**: Easy to mock for testing
- **Introspection**: Capabilities() allows runtime feature detection

### 2. Factory

**Responsibility**: Manage provider lifecycle and selection

**State Management**:
```go
type Factory struct {
    providers map[string]Provider  // Registered providers
    config    *Config              // Configuration
}
```

**Key Methods**:
- **NewFactory(config)**: Initialize factory with providers
- **AutoDetect()**: Select best available provider
- **GetProvider(name)**: Get specific provider by name
- **Register(provider)**: Add provider at runtime
- **ListProviders()**: List all registered providers
- **Detect()**: Introspection for available providers

**Auto-Detection Logic**:
```
1. Check ANTHROPIC_API_KEY → Register Anthropic
2. Check GOOGLE_CLOUD_PROJECT:
   a. If location=us-east5 → Register Vertex Claude
   b. If USE_VERTEX_GEMINI=true → Register Vertex Gemini
3. Always register Local provider

Precedence: Anthropic > Vertex Claude > Vertex Gemini > Local
```

**Design Decisions**:
- **Map-based registry**: O(1) lookup by name
- **Lazy provider initialization**: Only create requested providers
- **Always register local**: Ensures fallback availability
- **Environment-based detection**: No configuration files required

### 3. Anthropic Provider

**Model**: claude-3-5-haiku-20241022

**Implementation**:
```go
type AnthropicProvider struct {
    client *anthropic.Client
    model  string
    config *Config
}
```

**Ranking Flow**:
1. Build ranking prompt with query and candidates
2. Call Anthropic Messages API with structured output
3. Parse JSON response with scores and reasoning
4. Validate scores in [0.0, 1.0] range
5. Sort by relevance (descending)

**Features**:
- **Prompt Caching**: Reduces cost for repeated queries
- **Structured Output**: JSON mode for reliable parsing
- **Error Handling**: Retry with exponential backoff
- **Cost Tracking**: Integration with CostSink

**Configuration**:
```yaml
anthropic:
  api_key_env: ANTHROPIC_API_KEY
  model: claude-3-5-haiku-20241022
```

### 4. Vertex AI Gemini Provider

**Model**: gemini-2.0-flash-exp

**Implementation**:
```go
type VertexAIGeminiProvider struct {
    projectID string
    location  string
    model     string
    config    *Config
}
```

**Ranking Flow**:
1. Build ranking prompt with JSON schema
2. Authenticate via Google Cloud default credentials
3. Call Vertex AI Gemini API with JSON mode
4. Parse response and extract scores
5. Sort by relevance

**Features**:
- **Large Context Window**: 1M tokens
- **JSON Mode**: Structured output support
- **Google Cloud Integration**: Uses default credentials
- **High Throughput**: 5 concurrent requests

**Configuration**:
```yaml
vertexai:
  project_id_env: GOOGLE_CLOUD_PROJECT
  location: us-central1
  model: gemini-2.0-flash-exp
```

### 5. Vertex AI Claude Provider

**Model**: claude-sonnet-4-5@20250929

**Implementation**:
```go
type VertexAIClaudeProvider struct {
    projectID string
    location  string  // Must be us-east5
    model     string
    config    *Config
}
```

**Ranking Flow**:
1. Build ranking prompt (Claude-specific format)
2. Authenticate via Google Cloud
3. Call Vertex AI Claude API endpoint
4. Parse response and extract rankings
5. Sort by relevance

**Features**:
- **Prompt Caching**: Same as direct Anthropic
- **Structured Output**: JSON mode support
- **Google Cloud Billing**: Unified billing with Vertex AI
- **Region Restriction**: Only available in us-east5

**Configuration**:
```yaml
vertexai-claude:
  project_id_env: GOOGLE_CLOUD_PROJECT
  location: us-east5
  model: claude-sonnet-4-5@20250929
```

### 6. Local Provider

**Algorithm**: Jaccard Similarity on tags and keywords

**Implementation**:
```go
type LocalProvider struct{}

func (p *LocalProvider) Rank(ctx context.Context, query string, candidates []Candidate) ([]RankedResult, error) {
    queryTokens := tokenize(query)

    for _, candidate := range candidates {
        candidateTokens := extractTokens(candidate)
        score := jaccardSimilarity(queryTokens, candidateTokens)
        results = append(results, RankedResult{
            Candidate: candidate,
            Score:     score,
            Reasoning: "", // No reasoning for local
        })
    }

    sort.Slice(results, func(i, j int) bool {
        return results[i].Score > results[j].Score
    })

    return results, nil
}
```

**Features**:
- **Zero Dependencies**: No API calls
- **Always Available**: Fallback when APIs fail
- **Fast**: < 10ms for typical queries
- **Deterministic**: Same input → same output
- **Free**: No cost

**Jaccard Similarity**:
```
Jaccard(A, B) = |A ∩ B| / |A ∪ B|

Example:
Query: "error handling patterns"
Tokens: {error, handling, patterns}

Candidate Tags: ["errors", "go", "patterns"]
Tokens: {errors, go, patterns}

Intersection: {patterns} (1 element)
Union: {error, errors, handling, go, patterns} (5 elements)
Score: 1/5 = 0.20
```

### 7. Configuration

**Configuration Sources** (precedence order):
1. Environment variables (highest priority)
2. ~/.engram/config.yaml
3. Default configuration values

**Default Configuration**:
```go
func DefaultConfig() *Config {
    return &Config{
        Ecphory: EcphoryConfig{
            Ranking: RankingConfig{
                Provider: "auto",
                Fallback: "local",
            },
            Providers: ProvidersConfig{
                Anthropic: AnthropicConfig{
                    APIKeyEnv: "ANTHROPIC_API_KEY",
                    Model:     "claude-3-5-haiku-20241022",
                },
                VertexAI: VertexAIConfig{
                    ProjectIDEnv: "GOOGLE_CLOUD_PROJECT",
                    Location:     "us-central1",
                    Model:        "gemini-2.0-flash-exp",
                },
                VertexAIClaude: VertexAIClaudeConfig{
                    ProjectIDEnv: "GOOGLE_CLOUD_PROJECT",
                    Location:     "us-east5",
                    Model:        "claude-sonnet-4-5@20250929",
                },
            },
        },
    }
}
```

### 8. Capabilities

**Capabilities Struct**:
```go
type Capabilities struct {
    SupportsCaching          bool  // Prompt caching support
    SupportsStructuredOutput bool  // JSON mode support
    MaxTokensPerRequest      int   // Context window size
    MaxConcurrentRequests    int   // Rate limit
}
```

**Provider Capabilities**:
| Provider | Caching | Structured | Max Tokens | Max Concurrent |
|----------|---------|------------|------------|----------------|
| Anthropic | ✅ | ✅ | 200,000 | 5 |
| Vertex Gemini | ❌ | ✅ | 1,000,000 | 5 |
| Vertex Claude | ✅ | ✅ | 200,000 | 5 |
| Local | ❌ | ❌ | 0 (unlimited) | 0 (unlimited) |

## Data Flow

### Ranking Request Flow

```
1. Client calls factory.AutoDetect()
   ↓
2. Factory checks environment variables
   ↓
3. Factory selects provider based on precedence
   ↓
4. Client calls provider.Rank(ctx, query, candidates)
   ↓
5. Provider builds ranking prompt
   ↓
6. Provider calls LLM API (or local algorithm)
   ↓
7. Provider parses response and extracts scores
   ↓
8. Provider validates and sorts results
   ↓
9. Provider returns ranked results to client
```

### Auto-Detection Flow

```
START
  ↓
Check ANTHROPIC_API_KEY?
  ├─ YES → Register Anthropic provider
  └─ NO  → Skip
  ↓
Check GOOGLE_CLOUD_PROJECT?
  ├─ YES → Check location and flags
  │        ├─ us-east5? → Register Vertex Claude
  │        └─ USE_VERTEX_GEMINI? → Register Vertex Gemini
  └─ NO  → Skip
  ↓
Always register Local provider
  ↓
Select provider by precedence:
  1. Anthropic
  2. Vertex Claude
  3. Vertex Gemini
  4. Local
  ↓
RETURN selected provider
```

## Error Handling

### Error Types

1. **Configuration Errors**: Missing API keys, invalid models
2. **API Errors**: Network failures, rate limits, authentication
3. **Validation Errors**: Invalid scores, malformed responses
4. **Timeout Errors**: Slow API responses

### Error Handling Strategy

```go
// Example: Ranking with fallback
results, err := provider.Rank(ctx, query, candidates)
if err != nil {
    // Log error
    log.Errorf("Provider %s failed: %v", provider.Name(), err)

    // Fall back to local provider
    localProvider := factory.GetProvider("local")
    results, err = localProvider.Rank(ctx, query, candidates)
    if err != nil {
        return nil, fmt.Errorf("all providers failed: %w", err)
    }
}
```

## Performance Characteristics

| Provider | Latency | Throughput | Cost |
|----------|---------|------------|------|
| Anthropic | 500-2000ms | 5 req/s | ~$0.001/req |
| Vertex Gemini | 300-1500ms | 5 req/s | ~$0.0005/req |
| Vertex Claude | 500-2000ms | 5 req/s | ~$0.001/req |
| Local | < 10ms | Unlimited | $0 |

## Security Considerations

### API Key Protection

- **Never log API keys**: Redact from logs and errors
- **Environment variables**: Store keys in environment, not config files
- **Validation**: Check key format before API calls
- **Isolation**: Provider instances don't share keys

### Credential Validation

```go
// Example: Anthropic key validation
if !strings.HasPrefix(apiKey, "sk-ant-") {
    return nil, fmt.Errorf("invalid Anthropic API key format")
}
```

### Input Sanitization

- **Query validation**: Check for injection patterns
- **Candidate validation**: Verify paths and metadata
- **Configuration validation**: Type-check all config values

## Testing Strategy

### Unit Tests (40+ tests)

- Provider initialization tests
- Ranking logic tests
- Configuration parsing tests
- Error handling tests

### Integration Tests (15+ tests)

- End-to-end ranking with real providers
- Auto-detection validation
- Fallback behavior testing
- Capabilities reporting tests

### Test Isolation

- **Credential-based skipping**: Skip API tests without credentials
- **Mock providers**: Test factory without real APIs
- **Environment restoration**: Clean up env vars after tests

**Example Test**:
```go
func TestAllProviders_Integration(t *testing.T) {
    t.Run("Anthropic_Provider", func(t *testing.T) {
        if os.Getenv("ANTHROPIC_API_KEY") == "" {
            t.Skip("Skipping: ANTHROPIC_API_KEY not set")
        }

        factory, _ := ranking.NewFactory(nil)
        provider, _ := factory.GetProvider("anthropic")
        results, err := provider.Rank(ctx, query, candidates)

        require.NoError(t, err)
        assert.NotEmpty(t, results)
        assert.Equal(t, "Go Error Handling", results[0].Candidate.Name)
    })
}
```

## Extensibility

### Adding a New Provider

1. **Implement Provider interface**:
```go
type OpenAIProvider struct {
    client *openai.Client
    model  string
}

func (p *OpenAIProvider) Name() string { return "openai" }
func (p *OpenAIProvider) Model() string { return p.model }
func (p *OpenAIProvider) Rank(...) ([]RankedResult, error) { ... }
func (p *OpenAIProvider) Capabilities() Capabilities { ... }
```

2. **Register in factory**:
```go
func (f *Factory) registerProviders() {
    // ... existing providers

    if openaiKey := os.Getenv("OPENAI_API_KEY"); openaiKey != "" {
        provider, err := NewOpenAIProvider(openaiKey, f.config)
        if err == nil {
            f.providers["openai"] = provider
        }
    }
}
```

3. **Add tests**:
```go
func TestOpenAIProvider(t *testing.T) {
    // Unit tests
}

func TestAllProviders_Integration(t *testing.T) {
    t.Run("OpenAI_Provider", func(t *testing.T) {
        // Integration test
    })
}
```

## Dependencies

- `github.com/anthropics/anthropic-sdk-go` v0.2.0+ (Anthropic)
- Google Cloud SDK (Vertex AI)
- `gopkg.in/yaml.v3` (configuration)

## Future Enhancements

1. **Additional Providers**: OpenAI, Cohere, local LLMs
2. **Ranking Cache**: Cache rankings to reduce API calls
3. **Parallel Ranking**: Query multiple providers simultaneously
4. **Provider Health Checks**: Monitor API availability
5. **Dynamic Provider Selection**: Choose based on query characteristics
6. **Cost Optimization**: Select cheapest provider per request
7. **A/B Testing**: Compare provider quality
