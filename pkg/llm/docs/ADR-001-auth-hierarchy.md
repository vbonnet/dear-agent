# ADR-001: Use supported provider credentials without OAuth extraction

Status: Accepted

## Context

LLM providers expose different supported credential mechanisms. Reusing tokens
by scraping another harness is fragile, insecure, and may violate provider
terms.

## Decision

Direct provider clients obtain credentials through `pkg/llm/auth` and accept
only mechanisms implemented by that provider path, such as an API key or
supported cloud identity. Harness OAuth remains inside the harness; work that
must use it is delegated through an explicit harness integration rather than
token extraction.

Missing or invalid credentials fail provider construction and may trigger a
configured router fallback. They are never treated as anonymous success.

## Consequences

- Direct API and harness-delegated execution remain distinct trust boundaries.
- Provider fallback is observable and testable.
- Configuration cannot silently promote scraped credentials.

## Evidence

- `../auth/`
- `../provider/` and provider authentication tests
