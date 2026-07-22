# ADR-001: Use supported provider credentials without OAuth extraction

Status: Accepted

## Context

LLM providers expose different supported credential mechanisms. Reusing tokens
by scraping another harness is fragile, insecure, and may violate provider
terms.

## Decision

Direct provider clients obtain credentials through `pkg/llm/auth` and accept
only mechanisms implemented by that provider path, such as an API key or
supported cloud identity. They do not scrape harness OAuth as a substitute for
a provider credential.

Claude harness launches have a separate, explicit extractor:
`pkg/llm/auth/oauth.go` may read the harness-owned credentials file and inject
`CLAUDE_CODE_OAUTH_TOKEN` only into the launched Claude process. This path
crosses the harness storage boundary deliberately; it is not exposed as a
general direct-client credential source.

Missing or invalid credentials fail provider construction and may trigger a
configured router fallback. They are never treated as anonymous success.

## Consequences

- Direct API and harness-delegated execution remain distinct trust boundaries.
- The Claude launcher must keep OAuth extraction scoped to its child process.
- Provider fallback is observable and testable.
- Configuration cannot silently promote scraped credentials.

## Evidence

- `../auth/`
- `../provider/` and provider authentication tests
