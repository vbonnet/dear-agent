# Ranking - Specification

## Overview

The ranking package implements a multi-provider abstraction for semantic relevance ranking in the Ecphory retrieval system. It supports multiple LLM backends (Anthropic, Vertex AI Gemini, Vertex AI Claude) with automatic provider detection and graceful fallback to local Jaccard similarity ranking.

## Purpose

**Primary Goal**: Provide vendor-independent semantic ranking of engram candidates using the best available AI provider, with zero dependency on any single vendor.

**Key Capabilities**:
- Multi-provider support (4 providers: Anthropic, Vertex Gemini, Vertex Claude, Local)
- Automatic provider detection based on environment variables
- Factory pattern for provider lifecycle management
- Graceful degradation to local ranking when APIs unavailable
- Provider capabilities reporting (caching, structured output, token limits)
- Cost tracking integration via CostSink abstraction

## Functional Requirements

### FR-1: Provider Interface

The system SHALL define a provider interface for semantic ranking:

- **FR-1.1**: Provider SHALL have Name() method returning provider identifier
- **FR-1.2**: Provider SHALL have Model() method returning model identifier
- **FR-1.3**: Provider SHALL have Rank() method for scoring candidates
- **FR-1.4**: Provider SHALL have Capabilities() method reporting features
- **FR-1.5**: Rank() SHALL accept context, query string, and candidate list
- **FR-1.6**: Rank() SHALL return ranked results with scores and reasoning

### FR-2: Provider Implementations

The system SHALL support multiple provider implementations:

- **FR-2.1**: Anthropic provider using anthropic-sdk-go
  - Model: claude-3-5-haiku-20241022
  - API key from ANTHROPIC_API_KEY environment variable
  - Supports prompt caching and structured output
  - Max tokens: 200,000 (context window)
  - Max concurrency: 5 concurrent requests

- **FR-2.2**: Vertex AI Gemini provider using Google Cloud
  - Model: gemini-2.0-flash-exp
  - Project ID from GOOGLE_CLOUD_PROJECT
  - Location from VERTEX_LOCATION (default: us-central1)
  - Supports structured output (JSON mode)
  - Max tokens: 1,000,000 (context window)
  - Max concurrency: 5 concurrent requests

- **FR-2.3**: Vertex AI Claude provider using Google Cloud + Anthropic
  - Model: claude-sonnet-4-5@20250929
  - Project ID from GOOGLE_CLOUD_PROJECT
  - Location: us-east5 (only supported region)
  - Supports prompt caching and structured output
  - Max tokens: 200,000 (context window)
  - Max concurrency: 5 concurrent requests

- **FR-2.4**: Local provider using Jaccard similarity
  - No API dependency (always available)
  - Tag-based similarity calculation
  - Case-insensitive keyword matching
  - No cost, unlimited requests
  - Deterministic scoring

### FR-3: Provider Factory

The system SHALL provide factory for provider management:

- **FR-3.1**: Factory SHALL register providers during initialization
- **FR-3.2**: Factory SHALL auto-detect best available provider
- **FR-3.3**: Factory SHALL support manual provider selection by name
- **FR-3.4**: Factory SHALL list all registered providers
- **FR-3.5**: Factory SHALL allow runtime provider registration
- **FR-3.6**: Factory SHALL report provider availability and selection reasoning

### FR-4: Auto-Detection

The system SHALL auto-detect providers with precedence order:

- **FR-4.1**: Precedence: Anthropic → Vertex Claude → Vertex Gemini → Local
- **FR-4.2**: Check ANTHROPIC_API_KEY for Anthropic provider
- **FR-4.3**: Check GOOGLE_CLOUD_PROJECT + location=us-east5 for Vertex Claude
- **FR-4.4**: Check GOOGLE_CLOUD_PROJECT + USE_VERTEX_GEMINI for Vertex Gemini
- **FR-4.5**: Fall back to Local provider if no credentials available
- **FR-4.6**: Provide Detect() method showing available providers and selection reason

### FR-5: Ranking Interface

The system SHALL provide consistent ranking interface across providers:

- **FR-5.1**: Accept query string and candidate list
- **FR-5.2**: Return ranked results sorted by relevance (descending)
- **FR-5.3**: Scores SHALL be normalized to [0.0, 1.0] range
- **FR-5.4**: Include reasoning for each ranking decision (API providers only)
- **FR-5.5**: Handle empty candidate lists gracefully
- **FR-5.6**: Support context cancellation

### FR-6: Configuration

The system SHALL support configuration via YAML and environment variables:

- **FR-6.1**: Load config from ~/.engram/config.yaml
- **FR-6.2**: Override with environment variables
- **FR-6.3**: Provide default configuration values
- **FR-6.4**: Support model selection per provider
- **FR-6.5**: Support location configuration for Vertex AI

### FR-7: Capabilities Reporting

The system SHALL report provider capabilities:

- **FR-7.1**: SupportsCaching (prompt caching support)
- **FR-7.2**: SupportsStructuredOutput (JSON mode support)
- **FR-7.3**: MaxTokensPerRequest (context window size)
- **FR-7.4**: MaxConcurrentRequests (rate limit)
- **FR-7.5**: Capabilities accessible via Capabilities() method

### FR-8: Error Handling

The system SHALL handle errors gracefully:

- **FR-8.1**: Return error for missing required configuration
- **FR-8.2**: Return error for invalid provider names
- **FR-8.3**: Return error for API failures with descriptive messages
- **FR-8.4**: Allow factory to fall back to next provider on auto-detect failure
- **FR-8.5**: Support graceful degradation to local provider

## Non-Functional Requirements

### NFR-1: Performance

- **NFR-1.1**: Local provider SHALL rank candidates in < 10ms
- **NFR-1.2**: API provider calls SHALL timeout after 30 seconds
- **NFR-1.3**: Factory auto-detection SHALL complete in < 100ms
- **NFR-1.4**: Provider registration SHALL be O(1) complexity

### NFR-2: Reliability

- **NFR-2.1**: Local provider SHALL always be available (no dependencies)
- **NFR-2.2**: API failures SHALL not crash the system
- **NFR-2.3**: Invalid configuration SHALL return clear error messages
- **NFR-2.4**: Provider selection SHALL be deterministic given same environment

### NFR-3: Maintainability

- **NFR-3.1**: Adding new providers SHALL require < 200 lines of code
- **NFR-3.2**: Provider interface SHALL be version-stable
- **NFR-3.3**: Configuration SHALL be backward-compatible
- **NFR-3.4**: Documentation SHALL include provider addition guide

### NFR-4: Security

- **NFR-4.1**: API keys SHALL never be logged or exposed
- **NFR-4.2**: Credential validation SHALL prevent injection attacks
- **NFR-4.3**: Provider isolation SHALL prevent cross-contamination
- **NFR-4.4**: Configuration SHALL validate all inputs

### NFR-5: Testability

- **NFR-5.1**: All providers SHALL have unit tests with 80%+ coverage
- **NFR-5.2**: Integration tests SHALL use credential-based skipping
- **NFR-5.3**: Mock providers SHALL be available for testing
- **NFR-5.4**: Factory SHALL support test isolation

## Constraints

### Technical Constraints

- **C-1**: Anthropic provider requires anthropic-sdk-go v0.2.0+
- **C-2**: Vertex AI providers require Google Cloud authentication
- **C-3**: Vertex Claude only available in us-east5 region
- **C-4**: Configuration file must be valid YAML

### Operational Constraints

- **C-5**: API keys must be valid and not expired
- **C-6**: Network connectivity required for API providers
- **C-7**: Local provider limited to keyword/tag matching
- **C-8**: Vertex AI requires gcloud CLI or service account

## Configuration Schema

```yaml
ecphory:
  ranking:
    provider: auto              # auto | anthropic | vertexai-gemini | vertexai-claude | local
    fallback: local             # Fallback provider if primary fails

  providers:
    anthropic:
      api_key_env: ANTHROPIC_API_KEY
      model: claude-3-5-haiku-20241022

    vertexai:
      project_id_env: GOOGLE_CLOUD_PROJECT
      location: us-central1
      model: gemini-2.0-flash-exp

    vertexai-claude:
      project_id_env: GOOGLE_CLOUD_PROJECT
      location: us-east5
      model: claude-sonnet-4-5@20250929
```

## Provider Precedence Logic

```
1. If ANTHROPIC_API_KEY is set:
   → Use Anthropic provider

2. Else if GOOGLE_CLOUD_PROJECT is set and location is us-east5:
   → Use Vertex Claude provider

3. Else if GOOGLE_CLOUD_PROJECT is set and USE_VERTEX_GEMINI is true:
   → Use Vertex Gemini provider

4. Else:
   → Use Local provider (always available)
```

## Ranking Output Format

```go
type RankedResult struct {
    Candidate Candidate  // Original candidate
    Score     float64    // Relevance score [0.0, 1.0]
    Reasoning string     // Explanation (API providers only)
}
```

## Usage Examples

### Auto-Detection

```go
factory, err := ranking.NewFactory(nil)
provider, err := factory.AutoDetect()
results, err := provider.Rank(ctx, query, candidates)
```

### Manual Selection

```go
factory, err := ranking.NewFactory(nil)
provider, err := factory.GetProvider("anthropic")
results, err := provider.Rank(ctx, query, candidates)
```

### Introspection

```go
factory, err := ranking.NewFactory(nil)
detection := factory.Detect()
fmt.Printf("Using: %s\n", detection.Provider)
fmt.Printf("Reason: %s\n", detection.Reason)
fmt.Printf("Available: %v\n", detection.Available)
```

## Dependencies

- `github.com/anthropics/anthropic-sdk-go` v0.2.0+ (Anthropic provider)
- Google Cloud authentication (Vertex AI providers)
- `gopkg.in/yaml.v3` (configuration parsing)

## Version History

- **v1.0.0** (2026-03-17): Initial implementation
  - 4 providers (Anthropic, Vertex Gemini, Vertex Claude, Local)
  - Factory pattern with auto-detection
  - Configuration via YAML and environment variables
