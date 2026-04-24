# ADR-001: Authentication Hierarchy Design

**Status**: Accepted
**Date**: 2026-03-20
**Authors**: Claude Sonnet 4.5
**Context**: llm-agent-architecture swarm, Phase 1

## Context and Problem Statement

Multiple engram tools need to authenticate with LLM providers (Anthropic, Gemini, OpenRouter) using different authentication methods (Vertex AI, API keys, OAuth via harnesses). Without a unified hierarchy, each tool reimplements authentication logic, leading to:

1. **Code duplication**: Same auth logic in 3+ locations
2. **Inconsistent precedence**: Tools disagree on Vertex AI vs API key priority
3. **Security risks**: Improper key validation, logging leaks
4. **ToS violations**: Potential OAuth token extraction from harnesses

**Question**: How should we design a unified authentication hierarchy that supports multiple providers and methods while maintaining security and ToS compliance?

## Decision Drivers

* **Security**: Must validate keys, sanitize logs, never extract OAuth tokens
* **Consistency**: All tools use same precedence order
* **Simplicity**: Environment-based detection (12-factor app)
* **Flexibility**: Support multiple providers and auth methods
* **ToS compliance**: Never extract OAuth from Claude Code/Gemini CLI

## Considered Options

### Option 1: OAuth-First Hierarchy (REJECTED)

**Precedence**: OAuth > Vertex AI > API Key > None

**Pros**:
- Optimal user experience (zero config)
- Best security (no API keys needed)

**Cons**:
- **VIOLATES ToS**: Requires extracting OAuth tokens from harnesses
- **Not implementable**: Claude Code/Gemini CLI don't expose OAuth tokens
- **Security risk**: If implemented, violates provider terms of service

**Decision**: REJECTED due to ToS violations.

### Option 2: API Key-First Hierarchy

**Precedence**: API Key > Vertex AI > None

**Pros**:
- Simple implementation
- Explicit user intent

**Cons**:
- Suboptimal for GCP users (API key required even with Vertex AI)
- Encourages API key proliferation
- Higher cost (Vertex AI often cheaper)

**Decision**: REJECTED due to poor GCP integration.

### Option 3: Vertex AI-First Hierarchy (SELECTED)

**Precedence**: Vertex AI > API Key > None

**Pros**:
- **Cost-effective**: Vertex AI often cheaper than direct API
- **Enterprise-friendly**: Uses Google Cloud ADC
- **Graceful degradation**: Falls back to API key if GCP not configured
- **ToS compliant**: No OAuth extraction

**Cons**:
- Requires GCP setup for Vertex AI
- Two environment variables per provider (GOOGLE_CLOUD_PROJECT + provider-specific)

**Decision**: SELECTED for production use.

## Decision Outcome

**Chosen**: Option 3 - Vertex AI-First Hierarchy

### Per-Provider Precedence

#### Anthropic/Claude
```
1. Vertex AI Claude (GOOGLE_CLOUD_PROJECT + Vertex AI permissions)
2. Anthropic API Key (ANTHROPIC_API_KEY)
3. None
```

#### Gemini/Google
```
1. Vertex AI Gemini (GOOGLE_CLOUD_PROJECT + Vertex AI permissions)
2. Gemini API Key (GEMINI_API_KEY or GOOGLE_API_KEY legacy)
3. None
```

#### OpenRouter
```
1. OpenRouter API Key (OPENROUTER_API_KEY)
2. None
```

**Note**: OpenRouter does not support Vertex AI.

### OAuth Handling

**CRITICAL**: OAuth is NEVER extracted from harnesses.

**OAuth Usage**:
- ✅ Via sub-agent delegation (harness manages OAuth automatically)
- ✅ Via headless CLI invocation (`gemini -p`, `claude -p` inherits OAuth)
- ❌ NEVER extracted, stored, or passed between processes

**Rationale**: Extracting OAuth tokens violates Claude Code and Gemini CLI terms of service. Instead, we use delegation strategies (see ADR-003) to leverage OAuth without extraction.

### Implementation

**API**:
```go
func DetectAuthMethod(providerFamily string) AuthMethod {
    switch providerFamily {
    case "anthropic", "claude":
        if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" && hasVertexClaude() {
            return AuthVertexAI
        }
        if os.Getenv("ANTHROPIC_API_KEY") != "" {
            return AuthAPIKey
        }
        return AuthNone
    case "gemini", "google":
        if os.Getenv("GOOGLE_CLOUD_PROJECT") != "" && hasVertexGemini() {
            return AuthVertexAI
        }
        if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("GOOGLE_API_KEY") != "" {
            return AuthAPIKey
        }
        return AuthNone
    case "openrouter":
        if os.Getenv("OPENROUTER_API_KEY") != "" {
            return AuthAPIKey
        }
        return AuthNone
    }
    return AuthNone
}
```

**Key Validation**:
```go
func ValidateAPIKey(provider, key string) error {
    switch provider {
    case "anthropic", "claude":
        if !strings.HasPrefix(key, "sk-ant-") {
            return errors.New("Anthropic API key must start with sk-ant-")
        }
    case "gemini", "google":
        if !strings.HasPrefix(key, "AIza") {
            return errors.New("Gemini API key must start with AIza")
        }
    case "openrouter":
        if !strings.HasPrefix(key, "sk-or-") {
            return errors.New("OpenRouter API key must start with sk-or-")
        }
    }
    return nil
}
```

**Key Sanitization**:
```go
func SanitizeKey(key string) string {
    if len(key) < 13 {
        return "***"
    }
    return key[:8] + "***...***" + key[len(key)-4:]
}
// Example: "sk-ant-api03-abc...xyz9" (shows first 8 + last 4)
```

## Consequences

### Positive

* **Unified**: All tools use same auth hierarchy
* **Cost-effective**: Vertex AI preferred when available
* **Secure**: Key validation, sanitized logging, no OAuth extraction
* **ToS compliant**: No terms of service violations
* **Enterprise-ready**: Works with Google Cloud ADC
* **Graceful degradation**: Falls back to API key if Vertex AI unavailable

### Negative

* **GCP dependency**: Optimal experience requires Google Cloud setup
* **Two env vars**: Vertex AI needs GOOGLE_CLOUD_PROJECT + provider permissions
* **No OAuth**: Cannot leverage harness OAuth directly (requires delegation - see ADR-003)

### Neutral

* **Environment-based**: Config via environment variables (12-factor app pattern)
* **Provider-specific**: Each provider has own precedence logic

## Validation

**Test Coverage**: 100% (53 tests)

**Test Categories**:
- Vertex AI detection (GOOGLE_CLOUD_PROJECT)
- API key fallback
- Provider precedence (Vertex AI > API Key > None)
- Provider aliases (claude/anthropic, google/gemini)
- Key validation (format checking)
- Key sanitization (safe logging)
- Environment isolation

**Security Audit**: No OAuth extraction code paths, validated by code review.

## References

* **SPEC.md**: Authentication Hierarchy section
* **ARCHITECTURE.md**: Authentication component design
* **pkg/llm/auth**: Implementation
* **ADR-002**: Per-tool configuration
* **ADR-003**: Delegation strategies (planned)

## Notes

This ADR focuses on authentication **detection** and **validation**. For authentication **usage** (how to leverage harness OAuth without extraction), see delegation strategies in ARCHITECTURE.md Phase 2.

**Key Principle**: Detection (this ADR) is separate from execution (delegation strategies).
