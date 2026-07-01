# Model Router Specification

## Purpose

The model router assigns an AGM session to a cost tier and returns the
harness-native model alias for that tier. It does not invoke harnesses directly;
callers resolve the returned alias through `agent.ResolveModelFullName`.

## EARS Requirements

**MR-01** When a caller provides a valid explicit tier, the router shall return that tier's configured model alias for the selected harness.

**MR-02** When a caller provides an invalid tier, the router shall reject it instead of silently falling back to another tier.

**MR-03** When no explicit tier is provided, the router shall classify the prompt using task-complexity heuristics and select the corresponding tier model.

**MR-04** When a harness has no configured tier map, the router shall return no model rather than inventing one.

**MR-05** When `codex-cli` uses implicit tier routing, the router shall resolve mid and expensive tier aliases to a Codex model supported by ChatGPT-account Codex auth.

**MR-06** When a Codex model is not universally account-supported, the router shall not select that model implicitly while it remains available for explicit model selection.
