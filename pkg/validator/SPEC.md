# Content Validator Specification

<!-- Last audited at: 2026-07-19 -->

## EARS Requirements

**VALIDATOR-01** When Engram content is validated, the system shall check frontmatter, type, title, description, context references, examples, and constraints.

**VALIDATOR-02** When content validation runs with auto-fix enabled, the system shall update supported metadata fixes and record every applied fix.

**VALIDATOR-03** When token budgets or progressive disclosure rules are exceeded, the system shall report errors or warnings with file context.

**VALIDATOR-04** When links are checked, the system shall distinguish valid, missing, and external targets according to configured policy.

**VALIDATOR-05** When the validator package is built, the system shall expose only maintained Engram, content, link, and YAML token contracts; retired numeric-phase artifact and retrospective schemas shall remain absent.

**VALIDATOR-06** When YAML frontmatter token counting is offline or exact tokenizers are unavailable, the system shall use the shared simple or heuristic fallback without selecting a model family.

**VALIDATOR-07** When frontmatter-only counting is disabled, the system shall report both frontmatter and total token counts and their percentage.

**VALIDATOR-08** While validation runs for any supported harness and model family, the system shall preserve identical content rules and fallback order.

## BDD Traceability

- Feature: `agm/test/bdd/features/validation_workspace_parity.feature`

## Test Traceability

- Unit package: `pkg/validator`
