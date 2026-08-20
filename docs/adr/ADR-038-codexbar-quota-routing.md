# ADR-038: CodexBar-backed quota-aware routing

Status: Proposed (2026-08-10)

## Context

DEAR needs model routing that can preserve Claude, Codex/OpenAI, and Gemini
capacity over a day instead of blindly exhausting the first configured provider.
CodexBar already aggregates local provider usage and exposes a redacted
programmatic view, so DEAR should consume that view before duplicating every
provider-specific parser.

The CodexBar security gate in Engram Research concluded:

- install only CodexBar `v0.49.0` or newer, preferably `v0.49.1` or newer;
- do not install `0.48.x` or earlier on machines with large Claude/Codex logs;
- start with Codex, Claude, and Gemini providers only;
- prefer local log, OAuth/API, or CLI-backed usage collection;
- leave browser-cookie imports, dashboard scraping extras, iCloud sync, and any
  optional telemetry-style integrations disabled until separately reviewed.

CodexBar's usable data interface is the redacted dashboard snapshot:
`codexbar dashboard --identity redacted`. The same schema is also available from
the optional loopback server under dashboard v1 endpoints. The CLI path is the
preferred DEAR dependency because it avoids introducing a long-running local
HTTP dependency and keeps identity redaction explicit at the call site.

## Decision

`pkg/llm/router` owns the provider-neutral quota interface:

- `QuotaReader` returns a `QuotaSnapshot` with provider families and quota
  windows.
- `CodexBarQuotaReader` shells out to
  `codexbar dashboard --identity redacted` with a short timeout.
- `EvaluateProviderQuota` converts the snapshot into an avoid/deprioritize
  decision using configurable thresholds.

The first implementation is default-off scaffolding. A follow-up wiring change
can apply the decision before each role candidate is attempted:

- avoid a provider when remaining quota is at or below the hard floor and another
  candidate remains viable;
- deprioritize a provider below the soft floor by trying healthier candidates
  first;
- ignore stale or unavailable quota data for availability, but record the reason
  in router metadata once policy is wired in;
- evaluate each provider family, not a hard-coded vendor list, so operators can
  add Claude, OpenAI/Codex, Gemini, OpenRouter, or local providers by config.

Example operational policy:

```yaml
quota_routing:
  enabled: true
  source:
    type: codexbar
    command: codexbar
    timeout: 2s
  thresholds:
    deprioritize_below_remaining_percent: 25
    avoid_below_remaining_percent: 10
    max_snapshot_age: 5m
  providers:
    anthropic:
      aliases: ["claude"]
    openai:
      aliases: ["codex"]
    gemini:
      aliases: ["google"]
```

## Alternatives

Parsing provider logs directly is the fallback if CodexBar is not installed or
its interface regresses. The minimum parser set would read Claude project logs
under `~/.claude/projects`, Codex session history under `~/.codex/sessions`, and
Gemini CLI/provider logs where present. Direct parsing should be isolated behind
the same `QuotaReader` interface so the router policy does not learn provider
storage formats.

Using CodexBar's loopback server would remove process spawn overhead, but it
adds an always-on local network dependency and requires server lifecycle
management. It remains a valid later optimization if polling overhead becomes
visible.

Hard-coding provider weights in the router would be simpler, but it cannot react
to resets, manual usage outside DEAR, or per-account quota differences.

## Consequences

Quota routing remains generic and configurable. CodexBar is treated as an
optional local data source, not as an authority for credentials or provider
execution. The router can spread load across Claude, Codex/OpenAI, and Gemini
without making any Claude-only mechanism mandatory.

The integration must not install or auto-start CodexBar. Operators make that
security decision separately, using the Engram Research audit as the gate.
