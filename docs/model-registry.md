# Model registry

- **Status:** authoritative
- **Last updated:** 2026-09-02

Where a model identifier has to be registered before the fleet can select it,
and how to verify an identifier before wiring it. Adding a model to one of
these surfaces and not the others is the recurring defect this page exists to
prevent: a model that resolves but prices at zero, or prices correctly but is
rejected by the harness allowlist.

## Verify before wiring

Never copy a model string from an announcement, a blog post, or recall. Ask the
provider what it actually serves:

- Anthropic: `GET /v1/models` lists the ids; `GET /v1/models/{id}` returns
  `max_input_tokens`, `max_tokens`, and the capability matrix.
- Antigravity (`agy`): `agy models` prints `slug<TAB>label` for the installed
  public catalog.

The two differ in an important way. Anthropic ids are the wire strings
(`claude-fable-5-1`). AGY's `--model` takes the **display label**, not the slug
(`Gemini 3.8 Flash (Medium)`), which is what `AGP-20` pins and why the AGY rows
in `HarnessModels` store labels containing spaces and parentheses.

## Registration surfaces

| Surface | File | What it controls |
| --- | --- | --- |
| Harness model allowlist and aliases | `agm/internal/agent/models.go` | Which strings `ValidateModel` accepts and what `ResolveModelFullName` emits per harness |
| Cross-harness tier aliases | `agm/internal/agent/models.go` (`CrossHarnessAliases`) | Translating a tier alias when a spawn crosses harnesses |
| Canonical rate card | `pkg/costtrack/pricing.go` | Input, output, cache-read, and cache-write rates keyed by exact model id |
| Budget rate table | `internal/pricing/pricing.go` | Alias and substring pricing for cost reports |
| Transcript cost estimator | `agm/internal/usage/usage.go` | Substring dispatch from a transcript's model field to a rate tier |
| Context windows | `agm/internal/session/context_detector.go` | Per-model context window for Pi direct and OpenRouter routes |
| Context threshold catalog | `pkg/context/models.yaml` | Context pressure thresholds and provenance notes |
| Research effort tiers | `engram/internal/harnesseffort/` | Which model each effort tier resolves to for Gemini and OpenCode |
| Role routing | `config/roles.yaml` | Primary, secondary, and tertiary model per role. Validation is lenient: this file needs no registration, only real ids |

## Two traps in the substring matchers

`internal/pricing.Lookup` and `agm/internal/usage.PriceFor` both fall back to
substring matching, so a point release silently inherits its predecessor's rate
unless a more specific entry precedes the generic one.

- `claude-fable-5-1` contains `fable`. Fable 5.1 prices cache reads at 0.025x
  input where Fable 5 uses 0.1x, so the specific case must be matched first or
  cache reads are overcharged fourfold.
- `gemini-3.8-flash` does not contain `3-flash` or `2.5-flash`, so it does not
  collide with the older Gemini entries. It also does not match anything, which
  is why it needs its own row rather than inheriting one.

## Currently wired frontier entries

| Model | Identifier | Context | Rate (per Mtok) |
| --- | --- | --- | --- |
| Claude Fable 5.1 | `claude-fable-5-1` | 1M in / 128K out | $10 in, $50 out, $0.25 cache read |
| Claude Fable 5 | `claude-fable-5` | 1M in / 128K out | $10 in, $50 out, $1.00 cache read |
| Gemini 3.8 Flash | `gemini-3.8-flash`, labels `Gemini 3.8 Flash (Low\|Medium\|High)` | 1M in / 65K out | $0.75 in, $3.75 out through 2026-12-31 |
| Gemini 3.5 Flash | `gemini-3.5-flash` | 1M in / 65K out | $1.50 in, $9.00 out |

Gemini 3.8 Flash introductory pricing ends 2026-12-31 and doubles to
$1.50 / $7.50 on 2027-01-01. The rate rows in `pkg/costtrack/pricing.go` and
`internal/pricing/pricing.go` carry that reminder.

## Constraints

- First-party integrations only. Do not add a middleman router such as LiteLLM
  or OpenRouter as a new access path for a model that the official CLI or API
  already serves.
- The Fable 5 free window for Pro, Max, and Team ended 2026-06-23. Both Fable
  generations bill at list price now; no free-window logic remains.
