# Model registry

- **Status:** authoritative
- **Last updated:** 2026-09-09

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
- OpenAI through Codex: `~/.codex/models_cache.json` is the catalog the CLI
  fetches from the provider. It carries `slug`, `context_window`,
  `max_context_window`, and `supported_reasoning_levels` per model, plus the
  `fetched_at` and `etag` that prove it came from the provider rather than from
  a human. Confirm with `codex exec -m <slug> 'reply OK'`.
- Antigravity (`agy`): `agy models` prints `slug<TAB>label` for the installed
  public catalog.

These differ in an important way. Anthropic and OpenAI ids are the wire strings
(`claude-opus-5`, `gpt-6-astra`). AGY's `--model` takes the **display label**,
not the slug (`Gemini 3.5 Flash (Medium)`), which is what `AGP-20` pins and why
the AGY rows in `HarnessModels` store labels containing spaces and parentheses.

Run the negative control too. Registering an id that does not exist is the
expensive failure, and the provider will tell you plainly:

```
$ codex exec -m gpt-6-astra-ultra 'reply OK'
ERROR: {"type":"error","status":400,"error":{"type":"invalid_request_error",
  "message":"The 'gpt-6-astra-ultra' model is not supported when using Codex
  with a ChatGPT account."}}
```

## An effort level is not a model id

This is the trap GPT-6 Astra introduces. Astra's catalog entry lists
`supported_reasoning_levels` of low, medium, high, xhigh, max, and **ultra**,
where ultra is described as "Maximum reasoning with automatic task delegation".

"Astra Ultra" is therefore a way of running `gpt-6-astra`, not a second model.
There is exactly one Astra slug. Effort is selected per launch through
`model_reasoning_effort`, which is a Codex config key, not part of the id.

Gemini encodes its effort in the label the CLI accepts
(`Gemini 3.5 Flash (Medium)` is a distinct `--model` argument), so AGY genuinely
needs one registry row per effort. Codex does not. Do not mirror the AGY shape
onto codex rows.

### Known gap: AGM cannot pin codex effort on a local launch

`agm/internal/harnessexec` sets `model_reasoning_effort` in exactly one place,
`remoteCodexReasoningEffort`, and only for the cold-remote-resume path
(`HEXEC-21`). `session_create_test.go` asserts that a fresh local launch does
**not** receive that override. A locally spawned codex session therefore
inherits whatever `~/.codex/config.toml` sets, and AGM has no flag to change it.

Consequence: an operator who asks for Astra at `ultra` through
`agm session new --model=gpt-6-astra` gets Astra at the config's effort instead,
silently. Closing this needs a real effort flag plumbed through `codexRequest`;
`ConfigOverrides` is reserved for hook-trust and must not be overloaded for it.

## Registration surfaces

| Surface | File | What it controls |
| --- | --- | --- |
| Harness model allowlist and aliases | `agm/internal/agent/models.go` | Which strings `ValidateModel` accepts and what `ResolveModelFullName` emits per harness |
| Cross-harness tier aliases | `agm/internal/agent/models.go` (`CrossHarnessAliases`) | Translating a tier alias when a spawn crosses harnesses |
| Canonical rate card | `pkg/costtrack/pricing.go` | Input, output, cache-read, and cache-write rates keyed by exact model id |
| Budget rate table | `internal/pricing/pricing.go` | Alias and substring pricing for cost reports |
| Context windows | `agm/internal/session/context_detector.go` | Per-model context window for Pi direct and OpenRouter routes |
| Context threshold catalog | `pkg/context/models.yaml` | Context pressure thresholds and provenance notes |
| Research effort tiers | `engram/internal/harnesseffort/` | Which model each effort tier resolves to, and the OpenCode `provider/model` spelling |
| Role routing | `config/roles.yaml` | Primary, secondary, and tertiary model per role. Validation is lenient: this file needs no registration, only real ids |

`agm/internal/usage/usage.go` is **not** a surface for OpenAI or Gemini models.
It parses Claude Code's own JSONL transcripts under `~/.claude/projects`, so
only Claude tiers ever reach its dispatch. Adding a GPT row there would be dead
code that implies coverage the package does not have.

## Two traps in the substring matchers

`internal/pricing.Lookup` falls back to substring matching, so a point release
silently inherits its predecessor's rate unless a more specific entry precedes
the generic one.

- `gpt-6-astra` shares no substring with any `gpt-5.x` row, so before it was
  registered it matched nothing and priced at **zero**. A zero on the most
  expensive tier we route to understates spend exactly where it hurts most.
- `ValidateModel` is deliberately forward-compatible: an unknown but
  syntactically safe id passes through with a warning on stderr rather than an
  error. That is why an unregistered model still launches, and why "it worked"
  is not evidence that a model was registered.

## Currently wired frontier entries

| Model | Identifier | Context | Rate (per Mtok) |
| --- | --- | --- | --- |
| GPT-6 Astra | `gpt-6-astra`, alias `astra` | 272K default / 872K max | $10 in, $50 out, $1.00 cache read |
| GPT-5.6 Sol | `gpt-5.6-sol` | 272K | $4 in, $20 out |
| GPT-5.6 Terra | `gpt-5.6-terra` | 272K | $2 in, $12 out |
| GPT-5.6 Luna | `gpt-5.6-luna` | 272K | $0.20 in, $1.20 out |
| Claude Opus 5 | `claude-opus-5` | 1M | $5 in, $25 out |

Astra's rates above are the **standard** service tier in the **short-context**
band. Two bands reprice it and neither is modelled in the single-rate tables:

- Long context (beyond the 272K default window): $20 in, $2 cache read, $75 out.
- The `fast` speed tier: $20 in, $2 cache read, $100 out.

This is why `context_detector.go` wires Astra at 272000 rather than at its
872000 maximum. The larger window is not free headroom; it is a different price.

## Known drift

The `gpt-5.6` rate comment in `agm/internal/agent/models.go` still reads
"sol $5/$30, terra $2.50/$15, luna $1/$6". The provider's published rates are
now $4/$20, $2/$12, and $0.20/$1.20. That correction is deliberately not
bundled into the Astra change; it is a separate rate-card fix.

`harness-effort-defaults.yaml` still names retired o-series models (`o3`,
`o4-mini`) in its lookup, operational, and analysis tiers. Only the `deep` tier
moved to Astra here, because that is the tier whose purpose matches Astra's
`ultra` level. The rest need the same verify-then-wire treatment.

## Constraints

- First-party integrations only. Do not add a middleman router such as LiteLLM
  or OpenRouter as a new access path for a model that the official CLI or API
  already serves.
- One rate row per model per table. If a model has banded pricing, wire the band
  the fleet actually uses and document the others here rather than averaging.
