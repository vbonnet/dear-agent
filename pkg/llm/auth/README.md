# pkg/llm/auth

Authentication hierarchy detection for LLM providers.

## Overview

This package implements a multi-tiered authentication strategy that automatically detects the appropriate authentication method for each LLM provider based on available credentials and environment configuration.

## Authentication Hierarchy

The package prioritizes authentication methods in the following order:

1. **Vertex AI ADC** (Application Default Credentials) - Preferred for GCP environments
2. **API Keys** - Direct provider authentication
3. **None** - No authentication available

## Supported Providers

### Anthropic/Claude
- **Vertex AI**: Requires `GOOGLE_CLOUD_PROJECT` environment variable
- **API Key**: Requires `ANTHROPIC_API_KEY` environment variable

### Gemini/Google
- **Vertex AI**: Requires `GOOGLE_CLOUD_PROJECT` environment variable
- **API Key**: Requires `GEMINI_API_KEY` or `GOOGLE_API_KEY` environment variable

### OpenRouter
- **API Key Only**: Requires `OPENROUTER_API_KEY` environment variable
- No Vertex AI support

## Usage

```go
import "github.com/vbonnet/engram/core/pkg/llm/auth"

// Detect authentication method for a provider
authMethod := auth.DetectAuthMethod("anthropic")

switch authMethod {
case auth.AuthVertexAI:
    // Use Vertex AI Claude with ADC
    fmt.Println("Using Vertex AI authentication")

case auth.AuthAPIKey:
    // Use Anthropic API with key from ANTHROPIC_API_KEY
    fmt.Println("Using API key authentication")

case auth.AuthNone:
    // No authentication available
    return fmt.Errorf("no authentication configured for provider")
}
```

## Environment Variables

### Vertex AI (GCP)
- `GOOGLE_CLOUD_PROJECT` - GCP project ID for Vertex AI access

### Anthropic
- `ANTHROPIC_API_KEY` - API key for direct Anthropic API access

### Gemini/Google
- `GEMINI_API_KEY` - Preferred API key for Gemini API access
- `GOOGLE_API_KEY` - Legacy API key for Google AI access

### OpenRouter
- `OPENROUTER_API_KEY` - API key for OpenRouter access

## Design Principles

1. **Security First**: Prioritizes managed authentication (Vertex AI ADC) over API keys
2. **Cloud Native**: Seamless integration with GCP Vertex AI when available
3. **Graceful Degradation**: Falls back to API keys when cloud services unavailable
4. **Zero Configuration**: Automatic detection based on environment
5. **Provider Flexibility**: Supports multiple provider aliases (e.g., "anthropic"/"claude")

## Testing

The package includes comprehensive test coverage:

```bash
go test ./pkg/llm/auth/...
```

Test coverage includes:
- Auth method precedence for each provider
- Environment variable detection
- Provider name aliases
- Environment isolation between tests
- Unknown provider handling

## Future Enhancements

Planned additions (see PLAN.md):
- OAuth device flow support (via sub-agents/headless mode only)
- Keychain integration for secure API key storage
- Token refresh handling
- Multi-region Vertex AI support
