# Model Registry

<!-- Last audited at: 2026-09-23 -->

How a new model gets wired into dear-agent, and what the current registrations
assume. A model is "registered" when every table below names it. Missing one
does not raise an error: each table has a fallback that returns a plausible
number, so a partial registration reports confident wrong values rather than
failing.

## Where a model has to be named

| Table | File | What a miss costs |
| --- | --- | --- |
| Harness alias registry | `agm/internal/agent/models.go` | The alias is passed through to the harness verbatim and rejected at launch |
| Context window registry | `pkg/context/models.yaml` | Falls back to the 200K `default` row; context percentages and compaction thresholds are wrong |
| Pi and OpenRouter windows | `agm/internal/session/context_detector.go` | Returns `(0, false)`; the caller substitutes a conservative default |
| Cost-report rate card | `pkg/costtrack/pricing.go` | Absent key yields the zero `Pricing`, so the model appears free |
| Shared rate lookup | `internal/pricing/pricing.go` | Substring scan resolves to a less specific family row at that family's price |
| Transcript cost estimator | `agm/internal/usage/usage.go` | Substring switch matches an earlier, less specific arm |
| Effort tiers | `engram/internal/harnesseffort/harness-effort-defaults.yaml` | The model cannot be named by an effort tier |

Two of these fail by returning a *wrong* number rather than nothing:
`internal/pricing.Lookup` and `usage.PriceFor` both scan for substrings, so a
point release is swallowed by the generation it extends. `claude-opus-5-5`
contains both `opus` and `opus-5`, and before this registration it priced at the
Claude 4.x Opus rate of $15/$75 — 3.75x its real input price, with nothing
downstream to flag it.

## Claude Opus 5.5

**Model id: `claude-opus-5-5`.** Not `claude-opus-5.5`; the dotted spelling is
not an accepted identifier, and AGM hands `FullName` to the harness unchanged.

| Property | Value |
| --- | --- |
| Context window | 1,000,000 tokens (same as Opus 5) |
| Max output | 128K tokens |
| Standard rate | $4.00 in / $20.00 out per MTok |
| Cache read | $0.20 per MTok (0.05x input, not the usual 0.1x) |
| Fast mode | $8.00 / $40.00 per MTok, Claude API only |
| Default reasoning effort | `medium` (Opus 5 defaults to `high`) |

Source: Anthropic published pricing, read 2026-09-23.

Opus 5.5 is **cheaper than Opus 5** ($4/$20 against $5/$25) despite being the
newer model, so a rate row copied from Opus 5 overstates cost by 25%. Both
rate tables carry the standard band only; fast mode is a second band that a
single-rate table cannot express.

Aliases: `opus5.5` and `opus55` under `claude-code`, `pi-cli`, and `openrouter`.
The two aggregators address it as `anthropic/claude-opus-5-5`, which needs its
own `models.yaml` row because `GetModel` normalizes only case and underscores
and cannot reach the unqualified row from a provider-qualified spelling.

### Behavioral differences that are not wired here

These change how a caller must *use* the model and are recorded so the next
change does not rediscover them. Nothing in this repository depends on them yet:

- Thinking cannot be disabled. `{type: "disabled"}` and `budget_tokens` both
  return 400 at every effort level; effort is the only depth control.
- Forced tool choice (`tool_choice` `any` or `tool`) returns 400. Use `auto`
  with `strict: true`, or structured outputs.
- Computer use is only available through `computer_toolset_20260801`.

### Selecting it

Opus 5.5 is registered as an available option, not as anyone's default. No
harness default, effort tier, or cross-harness abstract tier was repointed at
it. `latest-opus` still resolves to `claude-opus-4` and the `deep` tier still
names it; repointing those is a routing decision with its own cost profile and
belongs in its own change.

### Runtime verification status

Registration was made from the published identifier and rate card, not from a
live call. Claude authentication for detached agents is currently broken (the
keychain-versus-file credential split), so `agm`- and CLI-driven verification of
`claude-opus-5-5` cannot run and is not evidence either way. The locally
installed Claude Code build (2.1.273) predates the model and does not carry the
id, so it will reject the alias until it is updated; that is a client-version
floor, not a registration defect.
