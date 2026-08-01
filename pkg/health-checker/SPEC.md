# Health Checker Specification

<!-- Last audited at: 2026-08-01 -->

**Version:** 1.1
**Status:** Baseline
**Scope:** `pkg/health-checker`.

**Parity scope:** Claude Code, Codex CLI, Antigravity, and OpenCode across the Anthropic, OpenAI, Gemini, GLM, DeepSeek, Nemotron, and Qwen model families.

## EARS Requirements

**HEALTH-01** When health results are classified, the system shall treat OK and informational states as healthy and warning and error states as issues.

**HEALTH-02** When sequential checks run, the system shall preserve declaration order and stop with partial results on context cancellation.

**HEALTH-03** When parallel checks run, the system shall preserve result indexes and convert panics into error results.

**HEALTH-04** When results are summarized, the system shall count passed, warning, error, and fixable outcomes consistently.

**HEALTH-05** When summary status is converted to an exit code, the system shall prioritize errors over warnings and warnings over success.

**HEALTH-06** When dry-run fixing is enabled, the system shall report fixable results without invoking fix functions.

**HEALTH-07** When a fix succeeds, the system shall mark the result healthy and remove stale fix metadata.

**HEALTH-08** When a fix fails, the system shall retain issue status and shall append failure context to the result.

**HEALTH-09** While health operations are called from any supported harness and model family, the system shall preserve identical execution, summary, and fixing semantics.

**HEALTH-10** When a health status is validated, the system shall accept exactly OK, informational, warning, and error and return a stable invalid-status error identity for every other value.

**HEALTH-11** When a malformed health result is canonicalized, the system shall preserve its diagnostic, identify the invalid value, convert it to an error, clear executable fix metadata, and make repeated canonicalization value-idempotent.

**HEALTH-12** When a check returns a malformed result, the runner shall preserve its declaration index, replace its identity from the owning check, continue other checks, and return a canonical error without creating a new runner error.

**HEALTH-13** When malformed results are classified, summarized, or filtered, the system shall treat them as non-fixable critical errors that cannot leave aggregate health green or disappear from issue output.

**HEALTH-14** When preview or single-result fixing receives malformed input, the system shall suppress its callback and return no executable malformed metadata before dry-run or eligibility handling.

**HEALTH-15** When batch fixing contains a malformed result, the system shall validate the complete batch before dry-run handling or callbacks, apply zero fixes, preserve caller-owned input, and return only a safely canonicalized copy with an error.

**HEALTH-16** While all health statuses are valid, the system shall preserve public representation, status strings, ordering, aggregation, fix eligibility, dry-run slice identity, callback and cancellation behavior, reports, and empty-summary semantics.

**HEALTH-17** When parallel cancellation leaves an unexecuted result slot, the runner shall preserve the context error and return that slot as a canonical health error with owning check identity.

**HEALTH-18** When report-based fixing receives malformed input, the system shall return an empty non-nil report with non-nil result lists before counts, dry-run handling, or callbacks.

**HEALTH-19** When valid nil or empty batches are fixed, the system shall preserve original-slice identity in dry-run mode and preserve existing non-nil empty result and report-list behavior outside dry-run mode.

## BDD Traceability

- Feature: `agm/test/bdd/features/shared_runtime_policy_guardrails.feature`

## Test Traceability

- Unit package: `pkg/health-checker`
- Command compatibility: `cmd/flywheel-drift`
