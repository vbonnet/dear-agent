# pkg/llm - Unified LLM Agent Execution Library

**Status**: Phase 6 Complete (All Phases)
**Version**: 0.4.0
**Last Updated**: 2026-03-20

## Overview

The `pkg/llm` package provides a unified authentication hierarchy and configuration system for LLM agent execution across engram tools. It eliminates code duplication, supports multiple authentication methods, and enables per-tool model preferences.

## Phase 1 Features

### 1. Authentication Hierarchy (`pkg/llm/auth`)

**Goal**: Detect and select the appropriate authentication method based on provider and environment.

**Precedence Order**:
1. **Vertex AI** - Google Cloud Platform authentication (when `GOOGLE_CLOUD_PROJECT` is set)
2. **API Key** - Direct API key authentication (provider-specific environment variables)
3. **None** - No authentication available

**Supported Providers**:
- **Anthropic/Claude**: `ANTHROPIC_API_KEY` or Vertex AI
- **Gemini/Google**: `GEMINI_API_KEY`, `GOOGLE_API_KEY` (legacy), or Vertex AI
- **OpenRouter**: `OPENROUTER_API_KEY` only (no Vertex AI support)

**API**:
```go
// DetectAuthMethod determines the best authentication method for a provider
func DetectAuthMethod(providerFamily string) AuthMethod

// GetAPIKey retrieves the API key for a provider from environment
func GetAPIKey(provider string) (string, error)

// ValidateAPIKey checks if an API key has the correct format
func ValidateAPIKey(provider, key string) error

// SanitizeKey redacts an API key for safe logging
func SanitizeKey(key string) string
```

**Examples**:
```go
// Anthropic with Vertex AI
os.Setenv("GOOGLE_CLOUD_PROJECT", "my-project")
method := auth.DetectAuthMethod("anthropic")
// Returns: AuthVertexAI

// Gemini with API key
os.Setenv("GEMINI_API_KEY", "AIza...")
method := auth.DetectAuthMethod("gemini")
// Returns: AuthAPIKey

key, _ := auth.GetAPIKey("gemini")
// Returns: "AIza..."
```

### 2. Per-Tool Configuration System (`pkg/llm/config`)

**Goal**: Enable tools to use different models based on cost/accuracy tradeoffs.

**Configuration File**: `~/.engram/llm-config.yaml`

**Structure**:
```yaml
tools:
  ecphory:
    gemini:
      model: gemini-2.0-flash-exp
      max_tokens: 8192
    anthropic:
      model: claude-3-5-sonnet-20241022
      max_tokens: 4096
    default_family: gemini  # Cost-optimized

  multi-persona-review:
    anthropic:
      model: claude-opus-4-6
      max_tokens: 8192
    gemini:
      model: gemini-2.5-pro-exp
      max_tokens: 8192
    default_family: anthropic  # Quality-optimized

defaults:
  anthropic:
    model: claude-3-5-sonnet-20241022
  gemini:
    model: gemini-2.0-flash-exp
```

**API**:
```go
// LoadConfig loads configuration from a YAML file
func LoadConfig(path string) (*Config, error)

// SelectModel chooses the appropriate model based on tool and provider
func SelectModel(config *Config, toolName, providerFamily string) string

// GetMaxTokens retrieves the max_tokens setting for a tool/provider
func GetMaxTokens(config *Config, toolName, providerFamily string) int
```

**Fallback Hierarchy**:
1. Tool-specific configuration for provider
2. Global defaults for provider
3. Hardcoded defaults

**Examples**:
```go
config, _ := config.LoadConfig("~/.engram/llm-config.yaml")

// Cost-optimized tool
model := config.SelectModel(config, "ecphory", "gemini")
// Returns: "gemini-2.0-flash-exp"

// High-stakes tool
model = config.SelectModel(config, "multi-persona-review", "anthropic")
// Returns: "claude-opus-4-6"

// Provider override
model = config.SelectModel(config, "ecphory", "anthropic")
// Returns: "claude-3-5-sonnet-20241022"
```

## Architecture

### Package Structure

```
pkg/llm/
├── auth/                    # Authentication hierarchy
│   ├── hierarchy.go         # Auth method detection
│   ├── apikey.go            # API key management
│   ├── hierarchy_test.go    # 100% coverage (26 subtests)
│   ├── apikey_test.go       # 100% coverage (39 subtests)
│   ├── example_test.go      # Usage examples
│   └── README.md            # Package documentation
├── config/                  # Per-tool configuration
│   ├── schema.go            # YAML configuration structures
│   ├── loader.go            # Configuration loading and selection
│   ├── loader_test.go       # 82.8% coverage (11 tests)
│   ├── example_test.go      # Usage examples
│   ├── doc.go               # Package documentation
│   ├── example-config.yaml  # Configuration template
│   └── README.md            # Package documentation
└── SPEC.md                  # This file
```

### Design Principles

1. **Provider-agnostic**: Support multiple LLM providers with consistent interface
2. **Environment-based**: Detect authentication from environment variables (12-factor app)
3. **Graceful degradation**: Fall back to defaults when configuration is missing
4. **Security-first**: Validate API keys, sanitize for logging, never extract OAuth tokens
5. **ToS compliance**: No OAuth token extraction (use sub-agents or headless mode instead)

### Provider Aliases

Both full and short names are supported:
- `"anthropic"` ↔ `"claude"`
- `"gemini"` ↔ `"google"`
- `"openrouter"` (no alias)

## Testing

### Coverage Summary

- **pkg/llm/auth**: 100% coverage (65 test cases across 5 test functions)
- **pkg/llm/config**: 82.8% coverage (11 test functions)
- **Total**: 76 test cases, all passing

### Test Categories

**Authentication Tests**:
- Environment variable detection (GOOGLE_CLOUD_PROJECT, API keys)
- Provider precedence (Vertex AI > API Key > None)
- Provider aliases (anthropic/claude, gemini/google)
- API key validation (format checking, prefix validation)
- Key sanitization (safe logging)
- Environment isolation between tests

**Configuration Tests**:
- YAML parsing and validation
- Tilde expansion (`~/.engram/...`)
- Three-tier fallback hierarchy
- Provider alias normalization
- Missing file graceful handling
- Max tokens retrieval

### Running Tests

```bash
# From engram/core directory
go test ./pkg/llm/... -v -cover

# Coverage report
go test ./pkg/llm/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## Security Considerations

### API Key Management

1. **Validation**: All API keys validated for correct format before use
2. **Sanitization**: Keys sanitized when logged (shows first 8 + last 4 chars)
3. **No storage**: Keys read from environment only, never persisted to disk
4. **Format enforcement**:
   - Anthropic: Must start with `sk-ant-`
   - Gemini: Must start with `AIza`
   - OpenRouter: Must start with `sk-or-`

### ToS Compliance

**CRITICAL**: This package does NOT extract OAuth tokens from harnesses (Claude Code, Gemini CLI). OAuth is only used via:
1. **Sub-agent delegation** - Harness manages OAuth automatically
2. **Headless CLI invocation** - `gemini -p`, `claude -p` (OAuth preserved by harness)
3. **External API** - Uses API keys or Vertex AI ADC only

Extracting OAuth tokens violates terms of service for all providers.

## Phase 2: Headless Mode & Delegation Strategies (Complete)

**Status**: ✅ Complete

### Delegation Strategy Interface (`pkg/llm/delegation`)

Three execution strategies for seamless cross-provider support:

1. **SubAgent** - Use harness's Agent tool (preserves OAuth automatically)
   - Detects: `CLAUDE_SESSION_ID`, `GEMINI_SESSION_ID`
   - Benefits: Zero config, no API keys needed, ToS compliant

2. **Headless** - Invoke CLI in non-interactive mode
   - Commands: `gemini -p "prompt"`, `claude -p "prompt"`, `codex exec "prompt"`
   - Benefits: Cross-provider execution, OAuth inheritance when available

3. **ExternalAPI** - Direct SDK calls
   - Uses: API keys or Vertex AI ADC
   - Benefits: Works everywhere, no dependencies

**Priority**: SubAgent → Headless → ExternalAPI (automatic fallback)

**API**:
```go
type DelegationStrategy interface {
    Execute(ctx context.Context, input AgentInput) (AgentOutput, error)
}

func NewDelegationStrategy(providerOverride string) DelegationStrategy
func DetectHarnessProvider() string
func NormalizeProvider(provider string) string
```

**Test Coverage**: 54.1% (12 tests passing)

## Phase 3: Provider Consolidation (Complete)

**Status**: ✅ Complete

### Provider Interface

Unified provider abstraction migrated from `ecphory/ranking`:

```go
type Provider interface {
    Name() string
    Generate(ctx context.Context, req GenerateRequest) (*GenerateResponse, error)
    Capabilities() Capabilities
}
```

**Providers Implemented**:
- **Anthropic** - Direct API or Vertex AI
- **Vertex AI Claude** - Claude via Google Cloud
- **Vertex AI Gemini** - Gemini via Google Cloud
- **OpenRouter** - Multi-model proxy (optional)

**Factory Pattern**:
```go
func AutoDetect(config *Config) (Provider, error)
func Register(provider Provider)
func GetProvider(name string) (Provider, bool)
```

**Test Coverage**: 5.4% (8 tests, integration tests skip without credentials)

## Phase 4: Consumer Migration (Complete)

**Status**: ✅ Complete

### Updated Commands

**engram review**:
- Added `--provider` flag for explicit provider selection
- Loads per-tool config from `~/.engram/llm-config.yaml`
- Example: `engram review --file=SPEC.md --provider=gemini`

**engram subagent**:
- Integrated delegation strategy display
- Shows: SubAgent/Headless/ExternalAPI strategy in use
- Passes provider override to delegation layer

**engram explain**:
- Added `--provider` flag
- Uses cost-optimized models by default (gemini-flash)
- Per-tool config integration

### ecphory/ranking Migration

Migrated to use `pkg/llm/auth` for API key management:
- `factory.go` - Updated to use auth hierarchy
- `anthropic.go` - Delegates to `pkg/llm/auth.GetAPIKey()`
- `vertexai_*.go` - Uses auth detection

**Backward Compatibility**: 100% (all 52 existing tests passing)
**Test Coverage**: 61.3%

## Phase 5: SKILLs & Agent Definitions (Complete)

**Status**: ✅ Complete

### Claude Code Skills

Created 3 SKILL definitions in `~/.claude/skills/`:

1. **multi-persona-review.md** - Multi-persona code review
   - Default model: `claude-opus-4-6` (quality-optimized)
   - Natural language detection: "using Gemini", "with Claude"

2. **review-spec.md** - SPEC.md validation
   - Default model: `gemini-2.0-flash-exp` (cost-optimized)
   - LLM-as-judge with 0-10 scoring

3. **ecphory-explain.md** - Semantic search
   - Default model: `gemini-2.0-flash-exp` (ultra-cheap)
   - Shows tier-by-tier retrieval and cost breakdown

### Gemini CLI Agents

Created 3 agent definitions in `~/.gemini/agents/`:

Parallel implementations of Claude Code skills:
- `multi-persona-review.md`
- `review-spec.md`
- `ecphory-explain.md`

All support provider override via `--provider` flag.

### Cost Optimization Strategy

**Documented in SKILLs**:
- Frequent tasks (ecphory): Flash models ($0.0001/query)
- Critical tasks (review): Premium models ($0.010/query)
- 30x cost difference = $109/year savings for ecphory alone

## Phase 6: Testing & Merge (Complete)

**Status**: ✅ Complete

### Test Results

**Unit Tests**: 137 tests, 100% passing
- `pkg/llm/auth`: 100.0% coverage (53 tests)
- `pkg/llm/config`: 82.8% coverage (12 tests)
- `pkg/llm/delegation`: 54.1% coverage (12 tests)
- `pkg/llm/provider`: 5.4% coverage (8 tests, integration tests skipped)
- `pkg/ecphory/ranking`: 61.3% coverage (52 tests)

**Integration Points Validated**:
- ✅ Sub-agent delegation (harness detection working)
- ✅ Headless mode (CLI availability detection)
- ✅ Per-tool config (YAML loading and model selection)
- ✅ Provider override (--provider flag working)
- ✅ Backward compatibility (ecphory/ranking tests passing)

**No Regressions**: All existing functionality preserved

### Deployment

- ✅ Code merged to main branch
- ✅ Binary rebuilt from main: `~/go/bin/engram`
- ✅ Documentation updated: SPEC v0.4.0, ARCHITECTURE v0.4.0
- ✅ ADRs created: ADR-001, ADR-002

## Usage Examples

### Basic Authentication Detection

```go
import "github.com/vbonnet/engram/core/pkg/llm/auth"

// Detect best auth method for Anthropic
method := auth.DetectAuthMethod("anthropic")
switch method {
case auth.AuthVertexAI:
    // Use Vertex AI with Google Cloud SDK
case auth.AuthAPIKey:
    key, err := auth.GetAPIKey("anthropic")
    if err != nil {
        return err
    }
    // Use Anthropic SDK with API key
case auth.AuthNone:
    return errors.New("no authentication available")
}
```

### Per-Tool Model Selection

```go
import "github.com/vbonnet/engram/core/pkg/llm/config"

// Load configuration
cfg, err := config.LoadConfig("~/.engram/llm-config.yaml")
if err != nil {
    // Uses default config if file missing
}

// Select model for cost-optimized tool
model := config.SelectModel(cfg, "ecphory", "gemini")
maxTokens := config.GetMaxTokens(cfg, "ecphory", "gemini")

// Use with LLM client...
```

### API Key Validation

```go
import "github.com/vbonnet/engram/core/pkg/llm/auth"

key := os.Getenv("ANTHROPIC_API_KEY")
if err := auth.ValidateAPIKey("anthropic", key); err != nil {
    return fmt.Errorf("invalid API key: %w", err)
}

// Safe logging
log.Printf("Using key: %s", auth.SanitizeKey(key))
// Output: "Using key: sk-ant-a***...***xyz9"
```

## Migration Guide

### From ecphory/ranking to pkg/llm

**Before**:
```go
import "github.com/vbonnet/engram/core/pkg/ecphory/ranking"

// Hardcoded provider selection
provider := ranking.NewAnthropicProvider(apiKey)
```

**After**:
```go
import (
    "github.com/vbonnet/engram/core/pkg/llm/auth"
    "github.com/vbonnet/engram/core/pkg/llm/config"
)

// Automatic detection with fallback
cfg, _ := config.LoadConfig("~/.engram/llm-config.yaml")
model := config.SelectModel(cfg, "ecphory", "anthropic")

method := auth.DetectAuthMethod("anthropic")
if method == auth.AuthAPIKey {
    key, _ := auth.GetAPIKey("anthropic")
    // Use key...
}
```

## Contributing

### Adding a New Provider

1. Update `auth/hierarchy.go`:
   - Add case in `DetectAuthMethod()`
   - Document environment variables

2. Update `auth/apikey.go`:
   - Add case in `GetAPIKey()`
   - Add validation in `ValidateAPIKey()`

3. Update `config/loader.go`:
   - Add hardcoded default in `SelectModel()`

4. Add tests:
   - Test auth detection
   - Test API key retrieval
   - Test validation

### Adding a New Tool Configuration

1. Update `~/.engram/llm-config.yaml`:
   - Add tool section with provider configs
   - Set `default_family`

2. Document in `config/example-config.yaml`

3. Add test case in `config/loader_test.go`

## References

- **PLAN.md**: Complete implementation plan with all 6 phases
- **ROADMAP.md**: Project timeline and task breakdown
- **ADR-001**: Authentication hierarchy design decisions
- **ADR-002**: Per-tool configuration rationale

## Changelog

### v0.4.0 (2026-03-20) - All Phases Complete

**Added**:
- Phase 4: Consumer migration (`engram review`, `engram subagent`, `engram explain` with `--provider` flags)
- Phase 5: 6 SKILL definitions (3 Claude Code + 3 Gemini CLI)
- Phase 6: Comprehensive testing and validation (137 tests, 100% passing)

**Migration**:
- ecphory/ranking migrated to use pkg/llm/auth (backward compatible)
- All tools support per-tool model preferences
- Natural language provider detection in SKILLs

**Documentation**:
- SPEC.md updated to v0.4.0
- ARCHITECTURE.md created
- ADR-001: Authentication hierarchy decisions
- ADR-002: Per-tool configuration rationale

### v0.3.0 (2026-03-20) - Phase 3 Complete

**Added**:
- `pkg/llm/provider` - Unified provider interface
- Anthropic, Vertex AI Claude, Vertex AI Gemini providers
- OpenRouter provider (optional fallback)
- Provider factory with auto-detection
- Cost tracking integration

**Integration**:
- Migrated Provider interface from ecphory/ranking
- 8 provider tests (5.4% coverage due to credential requirements)

### v0.2.0 (2026-03-20) - Phase 2 Complete

**Added**:
- `pkg/llm/delegation` - Three-tier delegation strategy
- SubAgent strategy (CLAUDE_SESSION_ID, GEMINI_SESSION_ID detection)
- Headless strategy (`gemini -p`, `claude -p`, `codex exec`)
- ExternalAPI strategy (SDK calls with API keys/Vertex AI)
- Delegation factory with automatic fallback

**Test Coverage**: 54.1% (12 tests)

### v0.1.0 (2026-03-20) - Phase 1 Complete

**Added**:
- `pkg/llm/auth` - Authentication hierarchy with Vertex AI > API Key > None precedence
- `pkg/llm/config` - Per-tool configuration system with YAML support
- Support for Anthropic, Gemini, OpenRouter providers
- Provider aliases (anthropic/claude, gemini/google)
- API key validation and sanitization
- Comprehensive test suite (100% coverage for auth, 82.8% for config)
- Example configurations and usage documentation

**Security**:
- ToS-compliant (no OAuth extraction)
- API key format validation
- Secure key sanitization for logging
