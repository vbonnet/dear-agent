# pkg/llm - Architecture

**Version**: 0.4.0
**Last Updated**: 2026-03-20
**Status**: Production

## Overview

The `pkg/llm` package provides a unified authentication hierarchy and delegation system for LLM agent execution across engram tools. It eliminates code duplication, enables cross-provider execution, and supports cost-aware model selection.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                         engram CLI                           │
│  ┌─────────────┐  ┌──────────────┐  ┌──────────────────┐   │
│  │   review    │  │   subagent   │  │     explain      │   │
│  └──────┬──────┘  └──────┬───────┘  └────────┬─────────┘   │
│         │                 │                    │             │
│         └─────────────────┼────────────────────┘             │
│                           │                                  │
│                     ┌─────▼──────┐                          │
│                     │  pkg/llm   │                          │
│                     └─────┬──────┘                          │
│                           │                                  │
│        ┌──────────────────┼──────────────────┐              │
│        │                  │                  │              │
│   ┌────▼─────┐     ┌─────▼──────┐    ┌─────▼──────┐       │
│   │   auth   │     │  delegation│    │   config   │       │
│   └──────────┘     └─────┬──────┘    └────────────┘       │
│                           │                                  │
│        ┌──────────────────┼──────────────────┐              │
│        │                  │                  │              │
│  ┌─────▼──────┐   ┌──────▼───────┐  ┌──────▼────────┐     │
│  │  SubAgent  │   │   Headless   │  │  ExternalAPI  │     │
│  └────────────┘   └──────────────┘  └───────┬───────┘     │
│                                              │              │
│                                      ┌───────▼────────┐     │
│                                      │    provider    │     │
│                                      └────────────────┘     │
└─────────────────────────────────────────────────────────────┘
```

## Core Components

### 1. Authentication Hierarchy (`pkg/llm/auth`)

**Purpose**: Detect and select the appropriate authentication method based on provider and environment.

**Design Pattern**: Strategy pattern with environment-based detection

**Precedence Order**:
```
Vertex AI (GOOGLE_CLOUD_PROJECT) > API Key (PROVIDER_API_KEY) > None
```

**Key Functions**:
- `DetectAuthMethod(providerFamily string) AuthMethod` - Determines auth method
- `GetAPIKey(provider string) (string, error)` - Retrieves API key from environment
- `ValidateAPIKey(provider, key string) error` - Validates API key format
- `SanitizeKey(key string) string` - Redacts key for safe logging

**Security Features**:
- No OAuth token extraction (ToS violation)
- Format validation (sk-ant-, AIza, sk-or- prefixes)
- Sanitized logging (first 8 + last 4 chars only)
- Environment-only storage (never persisted)

**Test Coverage**: 100% (53 tests)

### 2. Per-Tool Configuration (`pkg/llm/config`)

**Purpose**: Enable tools to use different models based on cost/accuracy tradeoffs.

**Design Pattern**: YAML-based configuration with three-tier fallback

**Configuration File**: `~/.engram/llm-config.yaml`

**Fallback Hierarchy**:
```
Tool-specific config → Global defaults → Hardcoded defaults
```

**Example**:
```yaml
tools:
  ecphory:  # Cost-optimized (runs 100x/day)
    gemini:
      model: gemini-2.0-flash-exp
      max_tokens: 8192
    default_family: gemini

  multi-persona-review:  # Quality-optimized (critical reviews)
    anthropic:
      model: claude-opus-4-6
      max_tokens: 8192
    default_family: anthropic
```

**Key Functions**:
- `LoadConfig(path string) (*Config, error)` - Loads YAML configuration
- `SelectModel(config, toolName, providerFamily string) string` - Chooses model
- `GetMaxTokens(config, toolName, providerFamily string) int` - Gets token limit

**Cost Impact**: 30x difference (Flash vs Opus) = $109/year savings for ecphory

**Test Coverage**: 82.8% (12 tests)

### 3. Delegation Strategies (`pkg/llm/delegation`)

**Purpose**: Enable seamless cross-provider execution while maintaining ToS compliance.

**Design Pattern**: Strategy pattern with automatic detection and fallback

**Three Strategies**:

#### SubAgent Strategy
- **When**: Running inside harness (Claude Code, Gemini CLI)
- **Detection**: `CLAUDE_SESSION_ID` or `GEMINI_SESSION_ID` environment variables
- **Auth**: OAuth preserved automatically by harness
- **Benefits**: Zero config, ToS compliant, no API keys needed
- **Limitation**: Single-level only (sub-agents can't spawn sub-agents)

#### Headless Strategy
- **When**: CLI available but wrong provider (cross-provider execution)
- **Commands**: `gemini -p "prompt"`, `claude -p "prompt"`, `codex exec "prompt"`
- **Auth**: Inherits OAuth from harness if available, else uses API key
- **Benefits**: Cross-provider support, no nesting limit
- **Example**: User in Claude Code says "using Gemini" → runs `gemini -p`

#### ExternalAPI Strategy
- **When**: No harness detected or headless unavailable
- **Auth**: API keys or Vertex AI ADC
- **Benefits**: Works everywhere, no dependencies
- **Example**: Running `engram review` from terminal

**Priority Chain**:
```
SubAgent → Headless → ExternalAPI
```

**Provider Override Behavior**:
```
Provider matches harness  → SubAgent
Provider differs + CLI    → Headless
Provider differs + no CLI → ExternalAPI
No harness                → ExternalAPI
```

**Key Functions**:
- `NewDelegationStrategy(providerOverride string) DelegationStrategy` - Creates strategy
- `DetectHarnessProvider() string` - Detects current harness
- `NormalizeProvider(provider string) string` - Handles aliases

**Test Coverage**: 54.1% (12 tests)

### 4. Provider Interface (`pkg/llm/provider`)

**Purpose**: Unified interface for multiple LLM providers.

**Design Pattern**: Factory pattern with auto-detection

**Providers Implemented**:
- **Anthropic** - Direct API or Vertex AI
- **Vertex AI Claude** - Claude via Google Cloud
- **Vertex AI Gemini** - Gemini via Google Cloud
- **OpenRouter** - Multi-model proxy (optional)

**Interface**:
```go
type Provider interface {
    Name() string
    Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
    Capabilities() Capabilities
}
```

**Factory**:
- `AutoDetect(config) (Provider, error)` - Auto-detects based on credentials
- `Register(provider)` - Adds custom provider
- `GetProvider(name) (Provider, bool)` - Retrieves by name

**Test Coverage**: 5.4% (8 tests, integration tests require credentials)

## Data Flow

### Typical Request Flow

```
1. User runs: engram review --file=SPEC.md --provider=gemini

2. CLI parses flags:
   - tool_name = "review"
   - provider_override = "gemini"

3. Load config:
   config = LoadConfig("~/.engram/llm-config.yaml")
   model = SelectModel(config, "review", "gemini")
   // Returns: "gemini-2.0-flash-exp"

4. Detect auth:
   method = DetectAuthMethod("gemini")
   // Returns: AuthVertexAI (if GOOGLE_CLOUD_PROJECT set)

5. Create delegation strategy:
   strategy = NewDelegationStrategy("gemini")
   // In Claude Code: Returns HeadlessStrategy (cross-provider)
   // In terminal: Returns ExternalAPIStrategy

6. Execute:
   output = strategy.Execute(ctx, AgentInput{
       Prompt: "Review SPEC.md...",
       Model: model,
   })

7. Return result to user
```

### Cross-Provider Flow (User in Claude Code requests Gemini)

```
1. User in Claude Code: "run review using Gemini"

2. SKILL detects: provider_override = "gemini"

3. Delegation factory:
   - Detects: CLAUDE_SESSION_ID (in Claude harness)
   - Requested: gemini (doesn't match claude)
   - Check: gemini CLI available? YES
   - Strategy: HeadlessStrategy

4. Headless execution:
   - Runs: gemini -p "Review SPEC.md..." --output-format=json
   - Parses JSON output
   - Returns result

5. OAuth preserved: gemini CLI inherits OAuth from Claude Code harness
```

## Design Decisions

### Why Three-Tier Delegation?

**Problem**: Users want cross-provider execution (e.g., "use Gemini" while in Claude Code) but must maintain ToS compliance (no OAuth extraction).

**Solution**: Three strategies with automatic fallback:
1. **SubAgent**: Best UX, preserves OAuth, ToS compliant (when provider matches)
2. **Headless**: Good UX, enables cross-provider, preserves OAuth when available
3. **ExternalAPI**: Universal fallback, requires API keys

**Trade-offs**:
- SubAgent limited to single-level nesting
- Headless requires CLI installation
- ExternalAPI requires credentials

**Decision**: Use all three with automatic selection based on context.

**Reference**: See ADR-001 for full rationale.

### Why Per-Tool Configuration?

**Problem**: Different tools have different cost/accuracy tradeoffs:
- ecphory: Runs 100x/day, simple queries → needs cheap model
- review: Runs 1x/day, critical analysis → needs premium model

**Solution**: YAML configuration with tool-specific model selection:
```yaml
tools:
  ecphory:
    default_family: gemini  # Flash model: $0.0001/query
  review:
    default_family: anthropic  # Opus model: $0.010/query
```

**Impact**: 30x cost difference = $109/year savings for ecphory alone

**Decision**: Config-driven, not code-driven, for easy updates.

**Reference**: See ADR-002 for full rationale.

### Why Provider Aliases?

**Problem**: Users think in terms of "Claude" and "Gemini", not "Anthropic" and "Google".

**Solution**: Support both forms:
- "anthropic" ↔ "claude"
- "gemini" ↔ "google"

**Implementation**: Simple map in `NormalizeProvider()`:
```go
var providerAliases = map[string]string{
    "claude": "anthropic",
    "google": "gemini",
}
```

**UX Impact**: Reduces confusion, matches mental models.

## Security Considerations

### ToS Compliance

**CRITICAL**: This package does NOT extract OAuth tokens from harnesses.

**OAuth Usage**:
- ✅ Via sub-agent delegation (harness manages automatically)
- ✅ Via headless CLI (inherits from harness)
- ❌ NEVER extracted or stored

**Violations**:
- Extracting OAuth tokens from Claude Code
- Extracting OAuth tokens from Gemini CLI
- Storing OAuth tokens on disk
- Passing OAuth tokens between processes

**Enforcement**: Code reviews, security audits, no token storage code paths.

### API Key Security

**Validation**:
- Format checking (sk-ant-, AIza, sk-or- prefixes)
- Length validation
- No storage (environment only)

**Sanitization**:
```go
SanitizeKey("sk-ant-api03-abc...xyz9")
// Returns: "sk-ant-a***...***xyz9"
```

Shows first 8 + last 4 chars for debugging while preventing leaks.

## Testing Strategy

### Unit Tests

**Coverage Targets**:
- Authentication: 100% (security-critical)
- Configuration: >80%
- Delegation: >50%
- Providers: Best-effort (require credentials)

**Categories**:
- Environment detection
- Fallback hierarchy
- Error handling
- Edge cases (empty keys, missing files)
- Environment isolation

### Integration Tests

**Skipped When**:
- No API keys available
- No Vertex AI project configured
- Running in CI without credentials

**Pattern**:
```go
if os.Getenv("ANTHROPIC_API_KEY") == "" {
    t.Skip("Skipping integration test: ANTHROPIC_API_KEY not set")
}
```

### Manual Testing

**Scenarios**:
- OAuth flow in Claude Code
- Cross-provider in Claude Code (using Gemini)
- Per-tool config validation
- Provider override
- Headless mode execution

## Performance

### Benchmarks

**Auth Detection**: <1ms
- Environment variable lookup: O(1)
- No network calls
- No file I/O

**Config Loading**: <10ms
- YAML parse: ~5ms for typical config
- Cached in memory
- Tilde expansion: <1ms

**Delegation Strategy Selection**: <1ms
- Environment variable checks: O(1)
- No subprocess execution (only in actual Execute())

### Optimization Opportunities

**Config Caching**: Load once, reuse across requests
**Provider Pooling**: Reuse SDK clients
**Headless Batching**: Batch multiple prompts in single CLI invocation

## References

- **SPEC.md**: Complete specification with usage examples
- **ADR-001**: Authentication hierarchy design decisions
- **ADR-002**: Per-tool configuration rationale
- **ROADMAP.md**: Implementation timeline and phases

## Glossary

**Harness**: Host application (Claude Code, Gemini CLI) that manages OAuth
**Sub-agent**: Agent spawned by harness using Agent tool
**Headless mode**: CLI invoked non-interactively with `-p` flag
**Provider family**: Top-level provider name (anthropic, gemini, openrouter)
**Provider alias**: Alternative name (claude for anthropic, google for gemini)
**ToS**: Terms of Service (no OAuth extraction)
**ADC**: Application Default Credentials (Google Cloud)
**Vertex AI**: Google Cloud's LLM platform
