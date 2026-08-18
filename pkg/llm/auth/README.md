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

## Claude Code OAuth (Max-plan) tokens

`OAuthResolver` / `ResolveOAuthToken()` resolve the Claude Code OAuth access
token used by agm-spawned workers and the VROOM supervisor mesh. They prefer
the live token in `~/.claude/.credentials.json` over the
`CLAUDE_CODE_OAUTH_TOKEN` environment variable (which goes stale once Claude
Code refreshes the file), and **auto-refresh** when the on-disk token is
expired:

- The whole read → freshness-check → refresh-exchange → write cycle runs under
  a **cross-process advisory file lock** (`~/.claude/.credentials.lock`), so the
  separate tmux-pane processes that share one credentials file never each spend
  the single-use refresh token (the rotation race that poisons the token
  family). The freshness check is re-evaluated under the lock.
- The credentials file is backed up to `.credentials.json.bak` before a
  refresh, then written atomically (temp file + rename, mode `0600`).
- Errors are typed: `ErrTokenFamilyDead` (a `400 invalid_grant` — re-auth
  required) and `ErrRefreshNotPersisted` (rotated token could not be written —
  critical). Neither is ever silently swallowed.
- The token endpoint, client ID, and request User-Agent are overridable via
  `CLAUDE_OAUTH_TOKEN_ENDPOINT`, `CLAUDE_OAUTH_CLIENT_ID`, and
  `CLAUDE_OAUTH_USER_AGENT` (the endpoint and client ID have migrated before).

The standalone `cmd/token-refresher` binary drives this on a schedule (the
launchd idle backstop). See its package doc for modes and exit codes.

It must **not** be wired in as a Claude Code `apiKeyHelper`. That wiring was
retired on 2026-07-10: since claude-code 2.1.205 a configured helper is treated
as an external API key that shadows a healthy claude.ai OAuth login and refuses
to fall back to it (anthropics/claude-code#11587). See
[`cmd/token-refresher/README.md`](../../../cmd/token-refresher/README.md)
("Retired wiring").

## Future Enhancements

Planned additions (see PLAN.md):
- OAuth device flow support (via sub-agents/headless mode only)
- Keychain integration for secure API key storage
- Multi-region Vertex AI support
